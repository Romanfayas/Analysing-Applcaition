"""Did confirmed breakouts actually follow through? Measured on NSE's own price series (P1.9.27).

Replays the unchanged BreakoutEngine bar by bar over the exchange research series and records what each confirmed
breakout did next: the entry is the next session's open, the stop is the invalidation level, the target is Target 1, and
a trade still open after 10 sessions is closed at that close. Results are net of the platform's delivery cost model.

Any rule that might improve results is judged the honest way: it is chosen on the discovery period and must also hold in the
later holdout, which it never saw. A rule that only looks good in hindsight is reported as rejected, not adopted.
"""
from __future__ import annotations

import json
import math
import os
from dataclasses import asdict
from datetime import date, timedelta
from typing import Any, Callable, Dict, List, Optional, Tuple

import numpy as np
import pandas as pd
import psycopg2
from psycopg2.extras import Json

from src.engines.research.exchange_series import SERIES_LIMITATIONS, SERIES_VERSION, build_series
from src.engines.research.performance import bca_interval, profit_factor
from src.engines.technical.breakout_backtest import BreakoutBacktester, CompletedTrade

RUN_KIND = "BREAKOUT_FOLLOW_THROUGH"
RUN_VERSION = "BREAKOUT_EVIDENCE_V1"
DISCOVERY_END = date(2024, 12, 31)
HOLDOUT_START = date(2025, 1, 1)
MIN_HISTORY_BARS = 500
MIN_GROUP_TRADES = 30


def _shariah(conn) -> Dict[str, str]:
    with conn.cursor() as cur:
        cur.execute("""SELECT s.symbol, x.status::text FROM (SELECT DISTINCT ON (symbol_id) symbol_id, status FROM shariah_screenings
                       ORDER BY symbol_id, screening_date DESC, id DESC) x JOIN symbols s ON s.id = x.symbol_id""")
        return dict(cur.fetchall())


def _nifty(conn) -> pd.Series:
    with conn.cursor() as cur:
        cur.execute("SELECT trade_date, close::float8 FROM index_daily_bars WHERE index_name = 'Nifty 50' ORDER BY trade_date")
        rows = cur.fetchall()
    if not rows:
        return pd.Series(dtype=float)
    s = pd.Series({pd.Timestamp(d): c for d, c in rows}).sort_index()
    return s


def _asof(series: pd.Series, day: str) -> Optional[float]:
    i = series.index.searchsorted(pd.Timestamp(day), side="right") - 1
    return float(series.iloc[i]) if i >= 0 else None


def summarize(rows: List[Dict[str, Any]]) -> Dict[str, Any]:
    n = len(rows)
    if n == 0:
        return {"trades": 0}
    net = np.array([r["net_return_pct"] for r in rows], float)
    gross = np.array([r["gross_return_pct"] for r in rows], float)
    reasons = pd.Series([r["exit_reason"] for r in rows]).value_counts().to_dict()
    pf = profit_factor(net)
    excess = [r["excess_vs_nifty_pct"] for r in rows if r.get("excess_vs_nifty_pct") is not None]
    out = {
        "trades": n, "win_rate_pct": float((net > 0).mean() * 100),
        "target_hit_pct": 100 * reasons.get("TARGET", 0) / n, "stopped_pct": 100 * reasons.get("STOP_LOSS", 0) / n,
        "time_exit_pct": 100 * reasons.get("TIME_LIMIT", 0) / n,
        "avg_net_return_pct": float(net.mean()), "median_net_return_pct": float(np.median(net)),
        "avg_gross_return_pct": float(gross.mean()),
        "profit_factor": None if not math.isfinite(pf) else float(pf),
        "avg_best_move_pct": float(np.mean([r["mfe_pct"] for r in rows])), "avg_worst_move_pct": float(np.mean([r["mae_pct"] for r in rows])),
        "avg_bars_held": float(np.mean([r["bars_held"] for r in rows])),
        "net_return_ci": bca_interval(net, np.mean, 1000),
        "avg_excess_vs_nifty_pct": float(np.mean(excess)) if excess else None,
        "excess_vs_nifty_ci": bca_interval(excess, np.mean, 1000) if len(excess) >= 5 else None,
    }
    return out


def _regime(nifty: pd.Series) -> Callable[[str], Optional[bool]]:
    if nifty.empty:
        return lambda d: None
    sma = nifty.rolling(50).mean()

    def up(day: str) -> Optional[bool]:
        i = nifty.index.searchsorted(pd.Timestamp(day), side="right") - 1
        if i < 49:
            return None
        return bool(nifty.iloc[i] > sma.iloc[i])
    return up


def enrich(trades: List[CompletedTrade], nifty: pd.Series, shariah: Dict[str, str]) -> List[Dict[str, Any]]:
    regime = _regime(nifty)
    out = []
    for t in trades:
        a, b = _asof(nifty, t.entry_date), _asof(nifty, t.exit_date)
        n = (b / a - 1) * 100 if a and b else None
        net = t.net_return * 100
        atr_pct = float(t.atr / t.signal_close * 100) if t.atr and t.signal_close else None
        out.append({
            "symbol": t.symbol, "signal_date": t.signal_date, "entry_date": t.entry_date, "exit_date": t.exit_date,
            "entry_price": round(float(t.entry_price), 2), "exit_price": round(float(t.exit_price), 2),
            "stop": round(float(t.invalidation_level), 2), "target": round(float(t.target_price), 2),
            "exit_reason": t.exit_reason, "bars_held": t.bars_held, "shariah": shariah.get(t.symbol, "UNKNOWN"),
            "gross_return_pct": round(t.gross_return * 100, 3), "net_return_pct": round(net, 3),
            "mfe_pct": round(t.mfe * 100, 3), "mae_pct": round(t.mae * 100, 3),
            "nifty_return_pct": None if n is None else round(n, 3),
            "excess_vs_nifty_pct": None if n is None else round(net - n, 3),
            "structure_score": t.structure_score, "rvol": t.rvol, "atr_pct": atr_pct,
            "extension_pct": round(float(t.entry_price / t.entry_high - 1) * 100, 3) if t.entry_high else None,
            "market_uptrend": regime(t.signal_date),
            "period": "DISCOVERY" if date.fromisoformat(t.entry_date) <= DISCOVERY_END else "HOLDOUT",
        })
    return out


# Candidate rules: each is judged on discovery, then must also hold on the holdout.
CANDIDATES: List[Tuple[str, str, Callable[[Dict[str, Any]], Optional[bool]]]] = [
    ("NOT_EXTENDED", "Next open is at or below the top of the entry zone", lambda r: None if r["extension_pct"] is None else r["extension_pct"] <= 0),
    ("MILD_EXTENSION", "Next open is at most 1% above the entry zone", lambda r: None if r["extension_pct"] is None else r["extension_pct"] <= 1),
    ("RVOL_2X", "Breakout day volume is at least 2x normal", lambda r: None if r["rvol"] is None else r["rvol"] >= 2),
    ("SCORE_70", "Structure score is at least 70", lambda r: None if r["structure_score"] is None else r["structure_score"] >= 70),
    ("MARKET_UPTREND", "NIFTY 50 is above its 50-day average", lambda r: r["market_uptrend"]),
    ("LOW_ATR", "Daily range (ATR) is at most 2.5% of price", lambda r: None if r["atr_pct"] is None else r["atr_pct"] <= 2.5),
]


def _yes(v: Optional[bool]) -> bool:
    return v is not None and bool(v)


def evaluate_rules(rows: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    disc = [r for r in rows if r["period"] == "DISCOVERY"]
    hold = [r for r in rows if r["period"] == "HOLDOUT"]
    base_d = summarize(disc).get("avg_net_return_pct")
    base_h = summarize(hold).get("avg_net_return_pct")
    out = []
    for rule_id, text, fn in CANDIDATES:
        kept_d = [r for r in disc if _yes(fn(r))]
        kept_h = [r for r in hold if _yes(fn(r))]
        dropped_d = [r for r in disc if fn(r) is not None and not _yes(fn(r))]
        sd, sh = summarize(kept_d), summarize(kept_h)
        improves_d = base_d is not None and sd.get("trades", 0) >= MIN_GROUP_TRADES and sd["avg_net_return_pct"] > base_d
        improves_h = base_h is not None and sh.get("trades", 0) >= MIN_GROUP_TRADES and sh["avg_net_return_pct"] > base_h
        ci = sh.get("net_return_ci") if sh.get("trades") else None
        positive_h = bool(ci and ci["low"] > 0)
        verdict = "ADOPTED" if improves_d and improves_h and sh.get("avg_net_return_pct", -1) > 0 else "REJECTED"
        why = ("Better than all breakouts in both periods and positive in the holdout." if verdict == "ADOPTED" else
               "Too few trades to judge." if min(sd.get("trades", 0), sh.get("trades", 0)) < MIN_GROUP_TRADES else
               "No improvement on the discovery period." if not improves_d else
               "Looked better on discovery but did not hold in the holdout." if not improves_h else
               "Improved the holdout, but the result is still not positive after costs.")
        out.append({"rule": rule_id, "description": text, "verdict": verdict, "why": why, "discovery": sd, "holdout": sh,
                    "dropped_discovery_trades": len(dropped_d), "holdout_ci_above_zero": positive_h})
    return out


# ------------------------------------------------------------------------- alternative trade plans

PLANS: List[Dict[str, Any]] = [
    {"id": "T1_10D", "text": "Target 1 or stop, 10-session limit (the current plan)", "target": "T1", "limit": 10},
    {"id": "T2_20D", "text": "Target 2 or stop, 20-session limit", "target": "T2", "limit": 20},
    {"id": "TRAIL_2ATR_20D", "text": "Trailing stop 2 ATR below the high, 20-session limit", "trail_atr": 2.0, "limit": 20},
    {"id": "TRAIL_3ATR_40D", "text": "Trailing stop 3 ATR below the high, 40-session limit", "trail_atr": 3.0, "limit": 40},
    {"id": "HOLD_20D", "text": "Stop only, sell after 20 sessions", "limit": 20},
    {"id": "HOLD_40D", "text": "Stop only, sell after 40 sessions", "limit": 40},
]
ENTRY_FILTERS: List[Dict[str, Any]] = [
    {"id": "ANY_OPEN", "text": "any next-day open", "max_ext_pct": None},
    {"id": "WITHIN_1PCT", "text": "only if the next open is within 1% of the entry zone", "max_ext_pct": 1.0},
]
MARKET_FILTERS: List[Dict[str, Any]] = [
    {"id": "ANY_MARKET", "text": "any market", "uptrend": False},
    {"id": "UPTREND", "text": "only when NIFTY 50 is above its 50-day average", "uptrend": True},
]


def collect_signals(frame: pd.DataFrame) -> Dict[str, Dict[str, Any]]:
    """Every confirmed breakout, bar by bar, using only data up to that bar."""
    from src.engines.technical.breakout import BreakoutEngine, BreakoutState
    out: Dict[str, Dict[str, Any]] = {}
    for sym, g in frame.groupby("symbol"):
        g = g.sort_values("timestamp").reset_index(drop=True)
        o, h, l, c = [g[k].to_numpy(float) for k in ("open", "high", "low", "close")]
        v = g["volume"].to_numpy(int)
        dates = pd.to_datetime(g["timestamp"]).dt.strftime("%Y-%m-%d").to_numpy()
        sig: List[Dict[str, Any]] = []
        for t in range(40, len(g) - 1):
            lb = max(0, t - 65)
            res = BreakoutEngine(symbol=str(sym), open_prices=o[lb:t + 1].tolist(), high_prices=h[lb:t + 1].tolist(), low_prices=l[lb:t + 1].tolist(),
                                 close_prices=c[lb:t + 1].tolist(), volumes=v[lb:t + 1].tolist(), shariah_status="PASS").analyze()
            if res.stage == BreakoutState.CONFIRMED_BREAKOUT:
                sig.append({"t": t, "date": dates[t], "stop": float(res.invalidation_level), "t1": float(res.target_1), "t2": float(res.target_2),
                            "entry_high": float(res.entry_zone[1]), "entry_low": float(res.entry_zone[0]), "atr": float(res.atr), "close": float(c[t]),
                            "score": res.structure_score, "rvol": float(res.rvol)})
        out[str(sym)] = {"o": o, "h": h, "l": l, "c": c, "dates": dates, "signals": sig}
    return out


def _simulate(d: Dict[str, Any], plan: Dict[str, Any], max_ext: Optional[float], allowed: Callable[[str], bool], cost: Callable[[float, float, int], float]) -> List[Dict[str, Any]]:
    o, h, l, c, dates = d["o"], d["h"], d["l"], d["c"], d["dates"]
    rows, busy_until = [], -1
    for s in d["signals"]:
        e = s["t"] + 1
        if e <= busy_until or e >= len(o) or not allowed(s["date"]):
            continue
        entry = float(o[e])
        ext = (entry / s["entry_high"] - 1) * 100
        if entry <= s["stop"] or (max_ext is not None and ext > max_ext):
            continue
        target = {"T1": s["t1"], "T2": s["t2"]}.get(plan.get("target", ""), math.inf)
        if target <= entry:
            continue
        hwm, x, px, why = float(h[e]), None, None, "TIME_LIMIT"
        for i in range(e + 1, min(e + plan["limit"] + 1, len(o))):
            stop = s["stop"]
            if plan.get("trail_atr"):
                stop = max(stop, hwm - plan["trail_atr"] * s["atr"])
            if l[i] <= stop:
                x, px, why = i, min(float(o[i]), stop), "STOP_LOSS"
                break
            if h[i] >= target:
                x, px, why = i, max(float(o[i]), target), "TARGET"
                break
            hwm = max(hwm, float(h[i]))
        if x is None:
            x = min(e + plan["limit"], len(o) - 1)
            px = float(c[x])
        busy_until = x
        rows.append({"entry_date": dates[e], "gross": (px / entry - 1) * 100, "net_return_pct": cost(entry, px, x - e), "why": why,
                     "bars": x - e, "period": "DISCOVERY" if date.fromisoformat(dates[e]) <= DISCOVERY_END else "HOLDOUT",
                     "exit_date": dates[x], "entry": entry, "exit": px})
    return rows


def _plan_stats(rows: List[Dict[str, Any]], nifty: pd.Series) -> Dict[str, Any]:
    if not rows:
        return {"trades": 0}
    net = np.array([r["net_return_pct"] for r in rows], float)
    ex = []
    for r in rows:
        a, b = _asof(nifty, r["entry_date"]), _asof(nifty, r["exit_date"])
        if a and b:
            ex.append(r["net_return_pct"] - (b / a - 1) * 100)
    pf = profit_factor(net)
    return {"trades": len(rows), "win_rate_pct": float((net > 0).mean() * 100), "avg_net_return_pct": float(net.mean()),
            "net_return_ci": bca_interval(net, np.mean, 1000), "profit_factor": None if not math.isfinite(pf) else float(pf),
            "avg_excess_vs_nifty_pct": float(np.mean(ex)) if ex else None, "excess_ci": bca_interval(ex, np.mean, 1000) if len(ex) >= 5 else None,
            "avg_days": float(np.mean([r["bars"] for r in rows]))}


def search_plans(signals: Dict[str, Dict[str, Any]], nifty: pd.Series, only: Optional[set] = None) -> Dict[str, Any]:
    """Pick the best plan on discovery only, then report it on the holdout it never saw."""
    from src.engines.research.cost_model import CostEngine, CostModelConfig
    ce = CostEngine(CostModelConfig())

    def cost(entry: float, px: float, days: int) -> float:
        return float(ce.calculate_trade("X", entry, px, quantity=max(1, int(100000 // entry)), holding_days=max(1, days)).net_return_pct)

    up = _regime(nifty)
    table = []
    for plan in PLANS:
        for ef in ENTRY_FILTERS:
            for mf in MARKET_FILTERS:
                allowed = (lambda dt: bool(up(dt))) if mf["uptrend"] else (lambda dt: True)
                rows: List[Dict[str, Any]] = []
                for sym, d in signals.items():
                    if only is None or sym in only:
                        rows.extend(_simulate(d, plan, ef["max_ext_pct"], allowed, cost))
                table.append({"plan": plan["id"], "entry": ef["id"], "market": mf["id"], "text": f'{plan["text"]}; {ef["text"]}; {mf["text"]}',
                              "discovery": _plan_stats([r for r in rows if r["period"] == "DISCOVERY"], nifty),
                              "holdout": _plan_stats([r for r in rows if r["period"] == "HOLDOUT"], nifty)})
    usable = [x for x in table if x["discovery"].get("trades", 0) >= MIN_GROUP_TRADES]
    best = max(usable, key=lambda x: x["discovery"]["avg_net_return_pct"]) if usable else None
    verdict = "NONE_PASSED"
    if best:
        hci, xci = best["holdout"].get("net_return_ci"), best["holdout"].get("excess_ci")
        if hci and hci["low"] > 0 and xci and xci["low"] > 0:
            verdict = "EDGE_CONFIRMED"
        elif hci and hci["estimate"] > 0:
            verdict = "POSITIVE_NOT_PROVEN"
    return {"tested": len(table), "selected_on_discovery": best, "holdout_verdict": verdict, "table": sorted(table, key=lambda x: -(x["discovery"].get("avg_net_return_pct") or -99)),
            "multiple_testing_note": f"{len(table)} combinations were tried; the winner on discovery is expected to look better than it really is, which is why only the holdout counts."}


def run(conn, include_detail: bool = True) -> Dict[str, Any]:
    df, series = build_series(conn)
    df = df.rename(columns={"date": "timestamp"})
    lengths = df.groupby("symbol").size()
    keep = lengths[lengths >= MIN_HISTORY_BARS].index
    shariah = _shariah(conn)
    frame = df[df["symbol"].isin(keep)].copy()
    frame["timestamp"] = pd.to_datetime(frame["timestamp"])
    frame["shariah_status"] = frame["symbol"].map(shariah).fillna("UNKNOWN")
    bt = BreakoutBacktester(frame, cost_model="COST_MODEL_2026_Q4_V2")
    trades, _ = bt.run_backtest(train_end_idx=10**7, val_end_idx=10**7 + 1)
    nifty = _nifty(conn)
    rows = enrich(trades, nifty, shariah)
    halal = [r for r in rows if r["shariah"] == "PASS"]
    signals = collect_signals(frame)
    plans = search_plans(signals, nifty)
    halal_syms = {k for k, v in shariah.items() if v == "PASS"}
    plans_halal = search_plans(signals, nifty, halal_syms)
    for p_ in (plans, plans_halal):
        p_["table"] = p_["table"][:12]
    seeded = seed_tracking(conn, frame, signals, shariah)
    return {
        "_seeded": seeded,
        "kind": RUN_KIND, "version": RUN_VERSION, "series": {k: v for k, v in series.items() if k != "adjustments"},
        "adjustments": series["adjustments"], "symbols_tested": int(len(keep)),
        "excluded_short_history": sorted(set(lengths.index) - set(keep)),
        "rules_of_the_test": {"entry": "next session's open after a confirmed breakout", "stop": "invalidation level", "target": "Target 1 (1.5R)",
                              "time_limit_sessions": 10, "cooldown_sessions": 5, "costs": bt.cost_model_version,
                              "discovery_period_ends": str(DISCOVERY_END), "holdout_starts": str(HOLDOUT_START)},
        "overall": summarize(rows),
        "by_period": {"DISCOVERY": summarize([r for r in rows if r["period"] == "DISCOVERY"]), "HOLDOUT": summarize([r for r in rows if r["period"] == "HOLDOUT"])},
        "shariah_pass_only": {"overall": summarize(halal), "holdout": summarize([r for r in halal if r["period"] == "HOLDOUT"])},
        "rules": evaluate_rules(rows),
        "plans": plans, "plans_shariah_pass": plans_halal,
        "recent_trades": sorted(rows, key=lambda r: r["entry_date"], reverse=True)[:60] if include_detail else [],
        "limitations": SERIES_LIMITATIONS,
    }


def store(conn, result: Dict[str, Any]) -> int:
    with conn.cursor() as cur:
        cur.execute("INSERT INTO research_runs (kind, version, series_version, result) VALUES (%s, %s, %s, %s) RETURNING id",
                    (result["kind"], result["version"], SERIES_VERSION, Json(json.loads(json.dumps(result, default=str)))))
        rid = cur.fetchone()[0]
    conn.commit()
    return rid


SEED_SESSIONS = 45


def seed_tracking(conn, frame: pd.DataFrame, signals: Dict[str, Dict[str, Any]], shariah: Dict[str, str]) -> int:
    """Record recent confirmed breakouts so the monitor starts with history. Each was found using only data up to its own day,
    and is marked HISTORY_REPLAY so it is never confused with a signal the platform announced live."""
    dates = sorted(frame["timestamp"].unique())
    cutoff = pd.Timestamp(dates[-SEED_SESSIONS]).strftime("%Y-%m-%d")
    ids = dict(frame[["symbol", "symbol_id"]].drop_duplicates().itertuples(index=False, name=None))
    n = 0
    with conn.cursor() as cur:
        for sym, d in signals.items():
            for s in d["signals"]:
                if s["date"] < cutoff:
                    continue
                cur.execute("""INSERT INTO breakout_tracking (symbol_id, signal_date, origin, signal_close, entry_low, entry_high, stop_level, target_1, target_2, structure_score, rvol, shariah_status)
                               VALUES (%s, %s, 'HISTORY_REPLAY', %s, %s, %s, %s, %s, %s, %s, %s, %s) ON CONFLICT (symbol_id, signal_date) DO NOTHING""",
                            (int(ids[sym]), s["date"], s["close"], s["entry_low"], s["entry_high"], s["stop"], s["t1"], s["t2"], s["score"], s["rvol"], shariah.get(sym, "UNKNOWN")))
                n += cur.rowcount
    conn.commit()
    return n


def run_and_store(conn) -> Dict[str, Any]:
    r = run(conn)
    rid = store(conn, r)
    return {"run_id": rid, "seeded": r.pop("_seeded", 0), "overall": r["overall"]}


if __name__ == "__main__":
    c = psycopg2.connect(os.environ.get("DATABASE_URL"))
    r = run(c)
    for k, v in (("overall", r["overall"]), ("disc", r["by_period"]["DISCOVERY"]), ("hold", r["by_period"]["HOLDOUT"])):
        print(k, {a: (round(b, 3) if isinstance(b, float) else b) for a, b in v.items() if not isinstance(b, dict)})
    for x in r["rules"]:
        print(x["rule"], x["verdict"], "|", x["why"], "| disc", x["discovery"].get("trades"), x["discovery"].get("avg_net_return_pct"), "| hold", x["holdout"].get("trades"), x["holdout"].get("avg_net_return_pct"))
    for name in ("plans", "plans_shariah_pass"):
        p = r[name]; b = p["selected_on_discovery"]
        print(name, p["holdout_verdict"], "| best:", b["text"] if b else None)
        if b:
            for k in ("discovery", "holdout"):
                st = b[k]; print("   ", k, st.get("trades"), "win", round(st.get("win_rate_pct") or 0, 1), "net", round(st.get("avg_net_return_pct") or 0, 3), st.get("net_return_ci") and (round(st["net_return_ci"]["low"], 3), round(st["net_return_ci"]["high"], 3)), "vs nifty", st.get("avg_excess_vs_nifty_pct") and round(st["avg_excess_vs_nifty_pct"], 3), st.get("excess_ci") and (round(st["excess_ci"]["low"], 3), round(st["excess_ci"]["high"], 3)))
        for t in p["table"][:6]:
            print("   ", t["plan"], t["entry"], t["market"], "D", t["discovery"].get("trades"), round(t["discovery"].get("avg_net_return_pct") or 0, 3), "H", t["holdout"].get("trades"), round(t["holdout"].get("avg_net_return_pct") or 0, 3))
    print("seeded", r.pop("_seeded", 0), "stored", store(c, r))
