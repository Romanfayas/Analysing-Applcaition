"""P1.9.31: paper-test of each breakout stage on the stock's own past.

For every past day the engine called a stock "closest to breaking out" (a WATCHLIST reading within 6% under resistance), "upcoming" (BREAKOUT_CANDIDATE)
or "confirmed" (CONFIRMED_BREAKOUT) -- each found using only data up to that day -- a simulated trade is opened and followed on the bars that came
after. The question it answers: when this pattern showed up before, did the price go through resistance as predicted, and did the trade make money
after costs?

Trade rules (written down before looking at results, never tuned):
* closest / upcoming: a buy-stop at the engine's entry level (the resistance). It stays valid for 5 sessions; it fills at the first bar whose high
  reaches it (at the open if the stock gaps above). A close at or under the stop before it fills cancels it. A bar that fills and also trades down
  to the stop is counted as stopped out (worst case).
* confirmed: bought at the next session's open, the same control plan the nightly study uses.
* exits (all three): a stop and Target 1 placed around the price actually paid -- the engine's own risk distance R below it for the stop and 1.5 R above
  it for the target, which is the engine's 1.5:1 plan carried to the real fill. (The engine anchors its published stop and target at the resistance
  level; a confirmed breakout is bought above that level, so the published levels leave a reward of well under half the risk.) Whichever the bars reach
  first, with the stop winning a tie; otherwise the close of the 10th session after entry. Gap-through fills are at the open.
* a pending order is cancelled if the price closes one risk unit R below where it was flagged, or under the engine's stop when that is lower. (A stock still
  several percent under resistance can have an engine stop above its price, which would cancel every order at once.)
* costs come from the same ledger as the nightly study; a stock re-flagged while its previous trade is still live is a repeat and is not traded again.
* Net results are compared with holding the Nifty 50 over the same days.

Honest limits: daily bars decide the entry and exit day; the time of day comes from the 1-minute bars and exists only since they were first stored.
Past results are not a forecast.
"""
from __future__ import annotations

import math
from datetime import date
from typing import Any, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
from psycopg2.extras import Json

from src.engines.research import breakout_evidence as be
from src.engines.research.breakout_watch import CANDIDATE, CONFIRMED, SymbolSeries
from src.engines.research.cost_model import CostEngine, CostModelConfig
from src.engines.research.performance import bca_interval
from src.engines.technical.breakout import BreakoutState

SETUPS = ("CLOSEST", "UPCOMING", "CONFIRMED")
CLOSEST_MAX_DIST_PCT = 6.0
TRIGGER_SESSIONS = 5
HOLD_SESSIONS = 10
NOTIONAL = 100_000.0
MIN_TRADES_FOR_VERDICT = 10
RECENT_SESSIONS = 126
KEEP_RUNS = 3

SETUP_TEXT = {
    "CLOSEST": "Closest to breaking out: price within 6% under resistance, not yet an upcoming flag. Buy only if it trades up through resistance within 5 sessions.",
    "UPCOMING": "Upcoming breakout: the engine's BREAKOUT_CANDIDATE (tight, quiet, near resistance). Buy only if it trades up through resistance within 5 sessions.",
    "CONFIRMED": "Confirmed breakout: the engine's CONFIRMED_BREAKOUT. Bought at the next session's open.",
}


def is_signal(s: SymbolSeries, t: int, setup: str) -> bool:
    d = s.detail.get(t)
    if not d:
        return False
    if setup == "CLOSEST":
        dist = d.get("dist")
        return d["stage"] == BreakoutState.WATCHLIST and dist is not None and 0 < dist <= CLOSEST_MAX_DIST_PCT
    return d["stage"] == (CANDIDATE if setup == "UPCOMING" else CONFIRMED)


def _pct(a: float, b: float) -> float:
    return (b / a - 1) * 100


def simulate_signal(s: SymbolSeries, t: int, setup: str, cost, rebase: bool = True) -> Tuple[Dict[str, Any], int]:
    """One paper trade for the signal on bar t. Returns the trade row and the last bar it used (the next trade must start after it)."""
    d = s.detail[t]
    o, h, l, c = s.o, s.h, s.l, s.c
    last = len(s.dates) - 1
    stop, target, trig = float(d["stop"]), float(d["t1"]), float(d["entry_low"])
    risk = max(trig - stop, 0.5 * float(d.get("atr") or 0.0))       # the engine's risk unit R (entry level minus invalidation, at least half an ATR)
    cancel = min(stop, float(c[t]) - risk)
    row: Dict[str, Any] = {
        "symbol": s.symbol, "symbol_id": s.symbol_id, "setup": setup, "signal_date": str(s.dates[t]), "signal_close": float(c[t]), "distance_pct": d.get("dist"),
        "resistance": float(d["resistance"]), "stop_level": stop, "target_1": target, "outcome": "PENDING", "triggered": False, "entry_date": None, "entry_time": None,
        "entry_price": None, "exit_date": None, "exit_time": None, "exit_price": None, "bars_held": None, "gross_pct": None, "net_pct": None, "excess_vs_nifty_pct": None,
        "mfe_pct": None, "mae_pct": None, "move_10d_pct": _pct(float(c[t]), float(c[t + 10])) if t + 10 <= last else None, "note": None,
    }
    fill: Optional[int] = None
    entry = 0.0
    if setup == "CONFIRMED":
        if t + 1 > last:
            return row, last
        fill, entry = t + 1, float(o[t + 1])
    else:
        for i in range(t + 1, min(t + TRIGGER_SESSIONS, last) + 1):
            if float(c[i]) <= cancel:
                row.update(outcome="FAILED_BEFORE_ENTRY", note="Fell a full risk unit below the flag price before reaching resistance; order cancelled")
                return row, i
            if float(h[i]) >= trig:
                fill, entry = i, max(float(o[i]), trig)
                break
        if fill is None:
            if t + TRIGGER_SESSIONS <= last:
                row.update(outcome="NOT_TRIGGERED", note="Did not reach resistance within 5 sessions")
                return row, t + TRIGGER_SESSIONS
            return row, last
    if rebase and risk > 0:
        stop, target = entry - risk, entry + 1.5 * risk
    if entry <= stop or target <= entry:
        row.update(outcome="SKIPPED", note="Opened above the engine's Target 1, so its plan no longer applies; no trade")
        return row, fill
    row.update(stop_level=stop, target_1=target)
    exit_i, px, why = None, 0.0, ""
    if setup != "CONFIRMED" and float(l[fill]) <= stop:
        exit_i, px, why = fill, stop, "STOPPED"            # filled and fell to the stop on the same bar: worst case
    else:
        for i in range(fill + 1, min(fill + HOLD_SESSIONS, last) + 1):
            if float(l[i]) <= stop:
                exit_i, px, why = i, min(float(o[i]), stop), "STOPPED"
                break
            if float(h[i]) >= target:
                exit_i, px, why = i, max(float(o[i]), target), "HIT_TARGET"
                break
        if exit_i is None:
            if fill + HOLD_SESSIONS <= last:
                exit_i, px, why = fill + HOLD_SESSIONS, float(c[fill + HOLD_SESSIONS]), "TIME_EXIT"
            else:
                exit_i, px, why = last, float(c[last]), "OPEN"
    bars = exit_i - fill
    hi, lo = float(np.max(h[fill:exit_i + 1])), float(np.min(l[fill:exit_i + 1]))
    row.update(outcome=why, triggered=True, entry_date=str(s.dates[fill]), entry_price=entry, exit_date=str(s.dates[exit_i]), exit_price=px, bars_held=bars,
               gross_pct=_pct(entry, px), net_pct=cost(entry, px, bars), mfe_pct=_pct(entry, hi), mae_pct=_pct(entry, lo))
    if why == "OPEN":
        row["note"] = "Still running at the latest bar; marked at its close and left out of the statistics"
    return row, exit_i


def make_cost():
    ce = CostEngine(CostModelConfig())

    def cost(entry: float, px: float, days: int) -> float:
        return float(ce.calculate_trade("X", entry, px, quantity=max(1, int(NOTIONAL // entry)), holding_days=max(1, days)).net_return_pct)
    return cost


def replay_symbol(s: SymbolSeries, cost, nifty: Optional[pd.Series] = None, rebase: bool = True) -> List[Dict[str, Any]]:
    rows: List[Dict[str, Any]] = []
    for setup in SETUPS:
        busy = -1
        for t in range(40, len(s.dates)):
            if t <= busy or not is_signal(s, t, setup):
                continue
            row, end = simulate_signal(s, t, setup, cost, rebase)
            busy = end
            if nifty is not None and row["entry_date"] and row["exit_date"] and row["net_pct"] is not None:
                a, b = be._asof(nifty, row["entry_date"]), be._asof(nifty, row["exit_date"])
                if a and b:
                    row["excess_vs_nifty_pct"] = row["net_pct"] - _pct(a, b)
            rows.append(row)
    return rows


def replay_universe(analysis: Dict[str, SymbolSeries], nifty: Optional[pd.Series] = None, rebase: bool = True) -> List[Dict[str, Any]]:
    cost = make_cost()
    rows: List[Dict[str, Any]] = []
    for sym in sorted(analysis):
        rows.extend(replay_symbol(analysis[sym], cost, nifty, rebase))
    return rows


# --------------------------------------------------------------------------------------------------- statistics

def _closed(rows: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    return [r for r in rows if r["outcome"] in ("HIT_TARGET", "STOPPED", "TIME_EXIT")]


def stats(rows: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Summary of one group of trades. `signals` counts settled signals (not pending ones); `triggered` is how often the price actually went through resistance."""
    settled = [r for r in rows if r["outcome"] not in ("PENDING", "OPEN", "SKIPPED")]
    closed = _closed(rows)
    net = [r["net_pct"] for r in closed]
    exc = [r["excess_vs_nifty_pct"] for r in closed if r["excess_vs_nifty_pct"] is not None]
    n_sig = len(settled)
    wins = sum(1 for x in net if x > 0)
    out: Dict[str, Any] = {
        "signals": n_sig, "pending": sum(1 for r in rows if r["outcome"] == "PENDING"),
        "triggered": len(closed), "triggered_pct": None if not n_sig else round(len(closed) / n_sig * 100, 1),
        "hit_target": sum(1 for r in closed if r["outcome"] == "HIT_TARGET"), "stopped": sum(1 for r in closed if r["outcome"] == "STOPPED"),
        "time_exit": sum(1 for r in closed if r["outcome"] == "TIME_EXIT"), "not_triggered": sum(1 for r in rows if r["outcome"] == "NOT_TRIGGERED"),
        "failed_before_entry": sum(1 for r in rows if r["outcome"] == "FAILED_BEFORE_ENTRY"), "skipped": sum(1 for r in rows if r["outcome"] == "SKIPPED"), "still_open": sum(1 for r in rows if r["outcome"] == "OPEN"),
        "wins": wins, "win_rate_pct": None if not net else round(wins / len(net) * 100, 1),
        "mean_net_pct": None if not net else round(float(np.mean(net)), 3), "median_net_pct": None if not net else round(float(np.median(net)), 3),
        "mean_excess_vs_nifty_pct": None if not exc else round(float(np.mean(exc)), 3),
        "total_net_inr_on_1L_each": round(sum(x / 100 * NOTIONAL for x in net), 0),
        "mean_days_held": None if not closed else round(float(np.mean([r["bars_held"] for r in closed])), 1),
        "mean_10d_move_pct": None,
        "net_ci95": None, "verdict": "INSUFFICIENT_DATA",
    }
    mv = [r["move_10d_pct"] for r in settled if r["move_10d_pct"] is not None]
    if mv:
        out["mean_10d_move_pct"] = round(float(np.mean(mv)), 3)
    ci = bca_interval(net, np.mean, 1000) if len(net) >= MIN_TRADES_FOR_VERDICT else None
    if ci:
        out["net_ci95"] = {"low": round(ci["low"], 3), "high": round(ci["high"], 3)}
    if len(net) >= MIN_TRADES_FOR_VERDICT:
        if ci and ci["low"] > 0:
            out["verdict"] = "POSITIVE_AFTER_COSTS"
        elif ci and ci["high"] < 0:
            out["verdict"] = "NEGATIVE_AFTER_COSTS"
        else:
            out["verdict"] = "NO_EDGE_DEMONSTRATED"
    return out


VERDICT_TEXT = {
    "INSUFFICIENT_DATA": f"Fewer than {MIN_TRADES_FOR_VERDICT} closed trades: too few to say anything.",
    "POSITIVE_AFTER_COSTS": "Net result after costs was positive and its 95% interval is above zero on this history (a past result, not a forecast).",
    "NEGATIVE_AFTER_COSTS": "Net result after costs was negative and its 95% interval is below zero on this history.",
    "NO_EDGE_DEMONSTRATED": "The net result after costs cannot be told apart from zero on this history.",
}


def build_summary(rows: List[Dict[str, Any]], as_of: str, all_dates: List[str], published: Optional[List[Dict[str, Any]]] = None) -> Dict[str, Any]:
    recent_from = all_dates[-RECENT_SESSIONS] if len(all_dates) >= RECENT_SESSIONS else (all_dates[0] if all_dates else "")
    out: Dict[str, Any] = {"as_of": as_of, "version": "PATTERN_REPLAY_V1", "recent_from": recent_from, "setups": {}, "rules": {
        "trigger_sessions": TRIGGER_SESSIONS, "hold_sessions": HOLD_SESSIONS, "closest_max_distance_pct": CLOSEST_MAX_DIST_PCT,
        "notional_per_trade_inr": NOTIONAL, "text": SETUP_TEXT}}
    for setup in SETUPS:
        mine = [r for r in rows if r["setup"] == setup]
        by_symbol: Dict[str, Any] = {}
        for sym in sorted({r["symbol"] for r in mine}):
            st = stats([r for r in mine if r["symbol"] == sym])
            by_symbol[sym] = {k: st[k] for k in ("signals", "triggered", "triggered_pct", "hit_target", "stopped", "time_exit", "win_rate_pct", "mean_net_pct", "total_net_inr_on_1L_each", "pending")}
        allst = stats(mine)
        rec = stats([r for r in mine if r["signal_date"] >= recent_from])
        out["setups"][setup] = {"all": allst, "recent": rec, "by_symbol": by_symbol, "verdict_text": VERDICT_TEXT[allst["verdict"]]}
        if published is not None:
            pst = stats([r for r in published if r["setup"] == setup])
            out["setups"][setup]["engine_levels_unchanged"] = {k: pst[k] for k in ("signals", "triggered", "win_rate_pct", "mean_net_pct", "skipped", "verdict")}
    return out


# --------------------------------------------------------------------------------------------------- minute times

def _minutes(cur, cache: Dict[Tuple[int, str], List[Tuple[str, float, float]]], symbol_id: int, day: str):
    k = (symbol_id, day)
    if k not in cache:
        cur.execute("""SELECT to_char(timestamp AT TIME ZONE 'Asia/Kolkata', 'HH24:MI'), high::float8, low::float8 FROM ohlcv_intraday
                       WHERE symbol_id = %s AND interval_minutes = 1 AND (timestamp AT TIME ZONE 'Asia/Kolkata')::date = %s ORDER BY timestamp""", (symbol_id, day))
        cache[k] = cur.fetchall()
    return cache[k]


def add_times(conn, rows: List[Dict[str, Any]], analysis: Dict[str, SymbolSeries]) -> int:
    """Fill the entry/exit time (IST) from 1-minute bars for trades whose days have them. Informational: the minute data is a different vendor from the
    exchange daily series, so a level the daily bar reached may not show in the minutes; then the time is left empty."""
    cache: Dict[Tuple[int, str], List[Tuple[str, float, float]]] = {}
    n = 0
    with conn.cursor() as cur:
        for r in rows:
            if r["outcome"] not in ("HIT_TARGET", "STOPPED", "TIME_EXIT", "OPEN") or not r["entry_date"]:
                continue
            m = _minutes(cur, cache, r["symbol_id"], r["entry_date"])
            if len(m) < 100:
                continue
            s = analysis[r["symbol"]]
            i = int(np.where(s.dates == r["entry_date"])[0][0])
            entry_idx = 0
            if r["setup"] != "CONFIRMED" and float(s.o[i]) < r["resistance"]:
                hit = next((k for k, (_, hi, _lo) in enumerate(m) if hi >= r["resistance"]), None)
                entry_idx = hit if hit is not None else -1
            if entry_idx >= 0:
                r["entry_time"] = m[entry_idx][0]
                n += 1
            if r["exit_date"] == r["entry_date"]:
                xm, start = m, max(entry_idx, 0)
            else:
                xm, start = _minutes(cur, cache, r["symbol_id"], r["exit_date"]), 0
            if len(xm) < 100:
                continue
            if r["outcome"] in ("TIME_EXIT", "OPEN"):
                r["exit_time"] = xm[-1][0]
            else:
                lvl = r["stop_level"] if r["outcome"] == "STOPPED" else r["target_1"]
                j = next((k for k in range(start, len(xm)) if (xm[k][2] <= lvl if r["outcome"] == "STOPPED" else xm[k][1] >= lvl)), None)
                if j is not None:
                    r["exit_time"] = xm[j][0]
    return n


# --------------------------------------------------------------------------------------------------- storage

def store(conn, rows: List[Dict[str, Any]], summary: Dict[str, Any]) -> int:
    with conn.cursor() as cur:
        cur.execute("INSERT INTO pattern_replay_runs (as_of, summary) VALUES (%s, %s) RETURNING id", (summary["as_of"], Json(summary)))
        run_id = cur.fetchone()[0]
        for r in rows:
            cur.execute("""INSERT INTO pattern_replay_trades (run_id, symbol_id, setup, signal_date, signal_close, distance_pct, resistance, stop_level, target_1, outcome, triggered,
                               entry_date, entry_time, entry_price, exit_date, exit_time, exit_price, bars_held, gross_pct, net_pct, excess_vs_nifty_pct, mfe_pct, mae_pct, move_10d_pct, note)
                           VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)""",
                        (run_id, r["symbol_id"], r["setup"], r["signal_date"], r["signal_close"], r["distance_pct"], r["resistance"], r["stop_level"], r["target_1"], r["outcome"],
                         r["triggered"], r["entry_date"], r["entry_time"], r["entry_price"], r["exit_date"], r["exit_time"], r["exit_price"], r["bars_held"], r["gross_pct"],
                         r["net_pct"], r["excess_vs_nifty_pct"], r["mfe_pct"], r["mae_pct"], r["move_10d_pct"], r["note"]))
        conn.commit()
        try:    # derived data: keep only the newest runs
            cur.execute("DELETE FROM pattern_replay_runs WHERE id <= %s", (run_id - KEEP_RUNS,))
            conn.commit()
        except Exception:
            conn.rollback()
    return run_id


def run_from_analysis(conn, analysis: Dict[str, SymbolSeries], nifty: Optional[pd.Series], through: date) -> Dict[str, Any]:
    rows = replay_universe(analysis, nifty)
    published = replay_universe(analysis, nifty, rebase=False)       # for comparison only: the engine's levels exactly as published
    add_times(conn, rows, analysis)
    all_dates = sorted({str(d) for s in analysis.values() for d in s.dates})
    summary = build_summary(rows, through.isoformat(), all_dates, published)
    run_id = store(conn, rows, summary)
    return {"run_id": run_id, "trades": len(rows), "by_setup": {k: v["all"]["signals"] for k, v in summary["setups"].items()}}
