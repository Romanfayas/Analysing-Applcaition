"""Upcoming-breakout (candidate) tracking, reasons, and per-stock signal tests (P1.9.29).

One pass of the unchanged BreakoutEngine over the NSE exchange series produces everything the Breakouts pages need:

* every day a stock was a BREAKOUT_CANDIDATE ("upcoming"), with the engine's reasons and levels frozen as of that day;
* what happened next: did the engine's own CONFIRMED_BREAKOUT follow within 10 sessions (BROKE_OUT), did the price close under the
  stop first (INVALIDATED), or did 10 sessions pass (EXPIRED)? plus the 5- and 10-session price move after the flag;
* the reasons / levels for already-confirmed breakouts that were recorded before reasons were stored;
* a per-stock test: how this stock's confirmed breakouts and candidate flags did, with sample-size honesty.

Nothing here tunes a rule. The label ("a CONFIRMED_BREAKOUT follows") is the engine's own definition, so a high hit rate means the
engine foresees its OWN label, not that the move pays after costs. Net results are reported separately from the cost ledger.
"""
from __future__ import annotations

import json
import math
from dataclasses import dataclass
from datetime import date
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
from psycopg2.extras import Json

from src.engines.research import breakout_evidence as be
from src.engines.research.cost_model import CostEngine, CostModelConfig
from src.engines.research.performance import bca_interval
from src.engines.technical.breakout import BreakoutEngine, BreakoutState

HORIZON_SESSIONS = 10
CANDIDATE = BreakoutState.BREAKOUT_CANDIDATE
CONFIRMED = BreakoutState.CONFIRMED_BREAKOUT
MIN_TRADES_FOR_VERDICT = 10
LOOKBACK = 65
NOTIONAL = 100_000.0


@dataclass
class SymbolSeries:
    symbol: str
    symbol_id: int
    dates: np.ndarray
    o: np.ndarray
    h: np.ndarray
    l: np.ndarray
    c: np.ndarray
    v: np.ndarray
    stage: List[Optional[str]]                    # stage on each bar t (None before 40 bars of history)
    detail: Dict[int, Dict[str, Any]]            # t -> engine detail, kept for CANDIDATE / CONFIRMED days and WATCHLIST days near resistance


def _detail(res) -> Dict[str, Any]:
    ez = list(res.entry_zone) if res.entry_zone else [None, None]
    return {"stage": res.stage, "stop": float(res.invalidation_level), "t1": float(res.target_1), "t2": float(res.target_2),
            "entry_low": None if ez[0] is None else float(ez[0]), "entry_high": None if ez[1] is None else float(ez[1]),
            "resistance": float(res.resistance_level), "atr": float(res.atr), "rvol": float(res.rvol), "dist": float(res.distance_to_resistance_pct),
            "score": None if res.structure_score is None else float(res.structure_score),
            "breakdown": {k: float(v) for k, v in (res.score_breakdown or {}).items()},
            "reasons": list(res.reasons or []), "invalidation": list(res.invalidation_conditions or [])}


def analyze_symbol(sym: str, symbol_id: int, g: pd.DataFrame) -> SymbolSeries:
    g = g.sort_values("timestamp").reset_index(drop=True)
    o, h, l, c = [g[k].to_numpy(float) for k in ("open", "high", "low", "close")]
    v = g["volume"].to_numpy(int)
    dates = pd.to_datetime(g["timestamp"]).dt.strftime("%Y-%m-%d").to_numpy()
    stage: List[Optional[str]] = [None] * len(g)
    detail: Dict[int, Dict[str, Any]] = {}
    for t in range(40, len(g)):
        lb = max(0, t - LOOKBACK)
        res = BreakoutEngine(symbol=str(sym), open_prices=o[lb:t + 1].tolist(), high_prices=h[lb:t + 1].tolist(), low_prices=l[lb:t + 1].tolist(),
                             close_prices=c[lb:t + 1].tolist(), volumes=v[lb:t + 1].tolist(), shariah_status="PASS").analyze()
        stage[t] = res.stage
        # WATCHLIST days within 6% under resistance are the "closest to breaking out" readings; pattern_replay paper-tests them
        if res.stage in (CANDIDATE, CONFIRMED) or (res.stage == BreakoutState.WATCHLIST and 0 < res.distance_to_resistance_pct <= 6.0):
            detail[t] = _detail(res)
    return SymbolSeries(str(sym), int(symbol_id), dates, o, h, l, c, v, stage, detail)


def analyze_universe(frame: pd.DataFrame) -> Dict[str, SymbolSeries]:
    ids = dict(frame[["symbol", "symbol_id"]].drop_duplicates().itertuples(index=False, name=None))
    return {str(sym): analyze_symbol(str(sym), ids[sym], g) for sym, g in frame.groupby("symbol")}


def signals_from(analysis: Dict[str, SymbolSeries]) -> Dict[str, Dict[str, Any]]:
    """Same structure as breakout_evidence.collect_signals, so the learner's simulator can reuse this single engine pass."""
    out: Dict[str, Dict[str, Any]] = {}
    for sym, s in analysis.items():
        sig = []
        for t in range(40, len(s.dates) - 1):
            d = s.detail.get(t)
            if d and d["stage"] == CONFIRMED:
                sig.append({"t": t, "date": s.dates[t], "stop": d["stop"], "t1": d["t1"], "t2": d["t2"], "entry_high": d["entry_high"],
                            "entry_low": d["entry_low"], "atr": d["atr"], "close": float(s.c[t]), "score": d["score"], "rvol": d["rvol"]})
        out[sym] = {"o": s.o, "h": s.h, "l": s.l, "c": s.c, "dates": s.dates, "signals": sig}
    return out


# --------------------------------------------------------------------------------------------- candidate outcomes

def candidate_outcome(s: SymbolSeries, t: int, horizon: int = HORIZON_SESSIONS) -> Dict[str, Any]:
    """What happened after the candidate flag on bar t, using only bars after t. Sessions are counted in bars of the series."""
    d = s.detail[t]
    stop, base = d["stop"], float(s.c[t])
    last = len(s.dates) - 1
    status, broke, to_break = "WATCHING", None, None
    elapsed = 0
    hi, lo = base, base
    for k in range(1, horizon + 1):
        i = t + k
        if i > last:
            break
        elapsed = k
        hi, lo = max(hi, float(s.h[i])), min(lo, float(s.l[i]))
        if s.stage[i] == CONFIRMED:
            status, broke, to_break = "BROKE_OUT", s.dates[i], k
            break
        if float(s.c[i]) <= stop:
            status = "INVALIDATED"
            break
    if status == "WATCHING" and elapsed >= horizon:
        status = "EXPIRED"
    r5 = (float(s.c[t + 5]) / base - 1) * 100 if t + 5 <= last else None
    r10 = (float(s.c[t + 10]) / base - 1) * 100 if t + 10 <= last else None
    return {"status": status, "sessions_elapsed": elapsed, "last_session": s.dates[min(t + elapsed, last)] if elapsed else None,
            "last_close": float(s.c[min(t + elapsed, last)]) if elapsed else None, "broke_out_session": broke, "sessions_to_breakout": to_break,
            "ret_5d_pct": r5, "ret_10d_pct": r10, "max_gain_pct": (hi / base - 1) * 100, "max_drawdown_pct": (lo / base - 1) * 100}


def candidates(analysis: Dict[str, SymbolSeries]) -> List[Dict[str, Any]]:
    """Every candidate flag in history with its outcome. A flag raised while an earlier one for the same stock is still being
    watched (within the horizon) is marked as a repeat so statistics count each setup once."""
    rows: List[Dict[str, Any]] = []
    for sym, s in analysis.items():
        last_counted_t = -10**9
        for t in sorted(k for k, d in s.detail.items() if d["stage"] == CANDIDATE):
            repeat = t - last_counted_t <= HORIZON_SESSIONS
            if not repeat:
                last_counted_t = t
            d = s.detail[t]
            out = candidate_outcome(s, t)
            rows.append({"symbol": sym, "symbol_id": s.symbol_id, "flagged_date": s.dates[t], "flagged_close": float(s.c[t]), "resistance": d["resistance"],
                         "stop": d["stop"], "entry_low": d["entry_low"], "entry_high": d["entry_high"], "t1": d["t1"], "t2": d["t2"], "score": d["score"],
                         "rvol": d["rvol"], "atr": d["atr"], "reasons": d["reasons"], "breakdown": d["breakdown"], "counts": not repeat, **out})
    return rows


# --------------------------------------------------------------------------------------------- persistence

def store_history(conn, analysis: Dict[str, SymbolSeries], sessions: int = 120, shariah: Optional[Dict[str, str]] = None) -> int:
    """Insert recent candidate flags as HISTORY_REPLAY (each was found using only data up to its own day). Never overwrites: a live
    flag for the same day wins. Outcomes of rows still WATCHING are finished by resolve()."""
    shariah = shariah or {}
    all_dates = sorted({d for s in analysis.values() for d in s.dates})
    if not all_dates:
        return 0
    cutoff = all_dates[-sessions] if len(all_dates) >= sessions else all_dates[0]
    n = 0
    with conn.cursor() as cur:
        for r in candidates(analysis):
            if r["flagged_date"] < cutoff:
                continue
            cur.execute("""INSERT INTO breakout_watch (symbol_id, flagged_date, origin, flagged_close, resistance_level, stop_level, entry_low, entry_high, target_1, target_2,
                               structure_score, rvol, atr, reasons, score_breakdown, shariah_status, status, sessions_elapsed, last_session, last_close,
                               broke_out_session, sessions_to_breakout, ret_5d_pct, ret_10d_pct, max_gain_pct, max_drawdown_pct, resolved_at, counts_in_stats)
                           VALUES (%s,%s,'HISTORY_REPLAY',%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s, CASE WHEN %s <> 'WATCHING' THEN now() END, %s)
                           ON CONFLICT (symbol_id, flagged_date) DO NOTHING""",
                        (r["symbol_id"], r["flagged_date"], r["flagged_close"], r["resistance"], r["stop"], r["entry_low"], r["entry_high"], r["t1"], r["t2"], r["score"],
                         r["rvol"], r["atr"], Json(r["reasons"]), Json(r["breakdown"]), shariah.get(r["symbol"], "UNKNOWN"), r["status"], r["sessions_elapsed"],
                         r["last_session"], r["last_close"], r["broke_out_session"], r["sessions_to_breakout"], r["ret_5d_pct"], r["ret_10d_pct"], r["max_gain_pct"],
                         r["max_drawdown_pct"], r["status"], r["counts"]))
            n += cur.rowcount
    conn.commit()
    return n


def resolve(conn, analysis: Dict[str, SymbolSeries]) -> Dict[str, int]:
    """Finish every WATCHING flag (live or replayed) from the exchange series. Idempotent; a flag that already resolved is untouched."""
    out = {"checked": 0, "resolved": 0, "still_watching": 0, "no_data": 0}
    with conn.cursor() as cur:
        cur.execute("""SELECT w.id, s.symbol, w.flagged_date::text FROM breakout_watch w JOIN symbols s ON s.id = w.symbol_id WHERE w.status = 'WATCHING'""")
        rows = cur.fetchall()
        for wid, sym, fdate in rows:
            out["checked"] += 1
            s = analysis.get(sym)
            if s is None:
                out["no_data"] += 1
                continue
            idx = np.where(s.dates == fdate)[0]
            if len(idx) == 0:
                out["no_data"] += 1
                continue
            t = int(idx[0])
            if t not in s.detail or s.detail[t]["stage"] != CANDIDATE:      # the series (adjusted/exchange) does not call it a candidate that day: leave it, never rewrite the claim
                out["no_data"] += 1
                continue
            o = candidate_outcome(s, t)
            cur.execute("""UPDATE breakout_watch SET status=%s, sessions_elapsed=%s, last_session=%s, last_close=%s, broke_out_session=%s, sessions_to_breakout=%s,
                               ret_5d_pct=%s, ret_10d_pct=%s, max_gain_pct=%s, max_drawdown_pct=%s, resolved_at = CASE WHEN %s <> 'WATCHING' THEN now() ELSE NULL END, updated_at=now()
                           WHERE id=%s AND status='WATCHING'""",
                        (o["status"], o["sessions_elapsed"], o["last_session"], o["last_close"], o["broke_out_session"], o["sessions_to_breakout"], o["ret_5d_pct"],
                         o["ret_10d_pct"], o["max_gain_pct"], o["max_drawdown_pct"], o["status"], wid))
            out["resolved" if o["status"] != "WATCHING" else "still_watching"] += 1
    conn.commit()
    return out


def backfill_reasons(conn, analysis: Dict[str, SymbolSeries]) -> int:
    """Reasons, resistance, ATR and score breakdown for tracked confirmed breakouts that were recorded without them. The engine is
    re-run on the exchange series as of the signal day; the result is labelled by the stage it reproduces."""
    n = 0
    with conn.cursor() as cur:
        cur.execute("""SELECT t.id, s.symbol, t.signal_date::text FROM breakout_tracking t JOIN symbols s ON s.id = t.symbol_id WHERE t.reasons IS NULL""")
        for tid, sym, sdate in cur.fetchall():
            s = analysis.get(sym)
            if s is None:
                continue
            idx = np.where(s.dates == sdate)[0]
            if len(idx) == 0 or s.detail.get(int(idx[0]), {}).get("stage") not in (CANDIDATE, CONFIRMED):
                continue
            d = s.detail[int(idx[0])]
            cur.execute("""UPDATE breakout_tracking SET reasons=%s, stage=%s, resistance_level=%s, atr=%s, score_breakdown=%s WHERE id=%s AND reasons IS NULL""",
                        (Json(d["reasons"]), d["stage"], d["resistance"], d["atr"], Json(d["breakdown"]), tid))
            n += cur.rowcount
    conn.commit()
    return n


# --------------------------------------------------------------------------------------------- per-stock tests

def _verdict(n: int, mean_net: Optional[float], ci: Optional[Dict[str, float]]) -> Tuple[str, str]:
    if n < MIN_TRADES_FOR_VERDICT:
        return "INSUFFICIENT_DATA", f"Only {n} past confirmed breakout trade(s) for this stock; at least {MIN_TRADES_FOR_VERDICT} are needed before any figure means anything."
    if ci and ci["low"] > 0:
        return "POSITIVE_AFTER_COSTS", "Net return after costs was positive and its 95% interval is above zero on this stock's own history (a past result, not a forecast)."
    if ci and ci["high"] < 0:
        return "NEGATIVE_AFTER_COSTS", "Net return after costs was negative and its 95% interval is below zero on this stock's own history."
    return "NO_EDGE_DEMONSTRATED", "The net result after costs is not distinguishable from zero on this stock's own history."


def stock_stats(analysis: Dict[str, SymbolSeries], signals: Dict[str, Dict[str, Any]], nifty) -> Dict[str, Dict[str, Any]]:
    """Per-stock test of the signals on that stock alone, using the control plan (Target 1 or stop, 10 sessions) and the cost ledger."""
    ce = CostEngine(CostModelConfig())

    def cost(entry: float, px: float, days: int) -> float:
        return float(ce.calculate_trade("X", entry, px, quantity=max(1, int(NOTIONAL // entry)), holding_days=max(1, days)).net_return_pct)

    plan = next(p for p in be.PLANS if p["id"] == "T1_10D")
    base_up = be._regime(nifty)
    cands = candidates(analysis)
    out: Dict[str, Dict[str, Any]] = {}
    for sym, s in analysis.items():
        trades = be._simulate(signals[sym], plan, None, lambda dt: True, cost)
        net = [t["net_return_pct"] for t in trades]
        ci = bca_interval(net, np.mean, 1000) if len(net) >= MIN_TRADES_FOR_VERDICT else None
        mean_net = float(np.mean(net)) if net else None
        verdict, why = _verdict(len(net), mean_net, ci)
        mine = [c for c in cands if c["symbol"] == sym and c["counts"] and c["status"] != "WATCHING"]
        hit = sum(1 for c in mine if c["status"] == "BROKE_OUT")
        by_period = {}
        for per in ("DISCOVERY", "HOLDOUT"):
            pt = [t["net_return_pct"] for t in trades if t["period"] == per]
            by_period[per] = {"trades": len(pt), "mean_net_pct": None if not pt else round(float(np.mean(pt)), 3)}
        out[sym] = {
            "symbol": sym, "series_through": str(s.dates[-1]), "bars": int(len(s.dates)),
            "confirmed_breakouts": {"trades": len(net), "wins": sum(1 for x in net if x > 0), "win_rate_pct": None if not net else round(sum(1 for x in net if x > 0) / len(net) * 100, 1),
                                    "mean_net_pct": None if mean_net is None else round(mean_net, 3), "net_ci95": None if not ci else {"low": round(ci["low"], 3), "high": round(ci["high"], 3)},
                                    "total_net_inr_on_1L_each": round(sum(n / 100 * NOTIONAL for n in net), 0), "by_period": by_period,
                                    "last_trades": [{"entry": t["entry_date"], "exit": t["exit_date"], "why": t["why"], "net_pct": round(t["net_return_pct"], 2)} for t in trades[-5:]]},
            "upcoming_flags": {"resolved_flags": len(mine), "broke_out": hit, "hit_rate_pct": None if not mine else round(hit / len(mine) * 100, 1),
                               "mean_10d_move_pct": None if not [c["ret_10d_pct"] for c in mine if c["ret_10d_pct"] is not None] else round(float(np.mean([c["ret_10d_pct"] for c in mine if c["ret_10d_pct"] is not None])), 2),
                               "note": "Hit = the engine's own CONFIRMED_BREAKOUT followed within 10 sessions. It is not a profit measure."},
            "verdict": verdict, "verdict_text": why,
            "plan": "Target 1 or stop, 10-session limit; entry at the next open; costs and 10 bps assumed slippage included",
        }
    return out


def universe_rates(analysis: Dict[str, SymbolSeries]) -> Dict[str, Any]:
    """Universe-wide base rate vs candidate hit rate (all history), reported on the page so a per-stock number has context."""
    base_n = base_hit = 0
    cand = candidates(analysis)
    for s in analysis.values():
        conf = {t for t in range(len(s.dates)) if s.stage[t] == CONFIRMED}
        for t in range(40, len(s.dates) - HORIZON_SESSIONS):
            if t in conf:
                continue
            base_n += 1
            base_hit += any((t + k) in conf for k in range(1, 6))
    res = [c for c in cand if c["counts"] and c["status"] != "WATCHING"]
    hits5 = [c for c in res if c["status"] == "BROKE_OUT" and (c["sessions_to_breakout"] or 99) <= 5]
    return {"base_rate_next5_pct": round(base_hit / base_n * 100, 2) if base_n else None, "candidate_flags": len(res),
            "candidate_hit_next5_pct": round(len(hits5) / len(res) * 100, 2) if res else None,
            "candidate_hit_next10_pct": round(sum(1 for c in res if c["status"] == "BROKE_OUT") / len(res) * 100, 2) if res else None,
            "candidate_mean_10d_move_pct": round(float(np.mean([c["ret_10d_pct"] for c in res if c["ret_10d_pct"] is not None])), 3) if res else None,
            "note": "Counts each setup once (repeat flags within 10 sessions are excluded). The label is the engine's own CONFIRMED_BREAKOUT, not profit."}


def store_stock_stats(conn, stats: Dict[str, Dict[str, Any]], as_of: str) -> int:
    n = 0
    with conn.cursor() as cur:
        cur.execute("SELECT symbol, id FROM symbols")
        ids = dict(cur.fetchall())
        for sym, r in stats.items():
            if sym not in ids:
                continue
            cur.execute("INSERT INTO stock_signal_stats (symbol_id, as_of, result) VALUES (%s, %s, %s)", (ids[sym], as_of, Json(json.loads(json.dumps(r, default=str)))))
            n += 1
    conn.commit()
    return n
