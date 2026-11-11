"""Governed learning loop for breakout trade plans (P1.9.28). Spec: docs/LEARNING_LOOP_SPEC.md

What "learning from mistakes" means here, and what it does NOT mean
-------------------------------------------------------------------
* The learner can only choose from a finite, pre-registered menu (LEARNER_SPACE_V1: 6 exit plans x 2 entry filters x 2 market
  filters = 24 candidates). It cannot invent rules, and nothing it learns edits a strategy in place.
* The honest test of a learner is not "which candidate looked best" but "how does the *procedure of picking* do on data it never
  saw". So the procedure is replayed quarter by quarter: at each quarter start it picks the candidate that was best on trades
  FINISHED before that date, and is then scored on that quarter's trades. Pooled out-of-sample results are compared with the
  frozen control (the current plan, T1_10D / any open / any market).
* Promotion to ACTIVE_PAPER requires every gate (sample size, positive after costs, beats control, stable selection) using only
  COMPLETED quarters, so running nightly cannot "peek" at a half-finished quarter until it looks good.
* An active version stays on probation: if its own forward trades fall behind control it is rolled back automatically.
* Paper trading only. This module never touches orders, live flags or the frozen Strategy V2 parameters.
"""
from __future__ import annotations

import hashlib
import json
import math
from dataclasses import dataclass
from datetime import date, timedelta
from itertools import product
from typing import Any, Dict, List, Optional, Sequence, Tuple

import numpy as np
from psycopg2.extras import Json

from src.engines.research import breakout_evidence as be
from src.engines.research.cost_model import CostEngine, CostModelConfig
from src.engines.research.evaluation_stats import ClosedTrade as EqTrade, equity_curve, max_drawdown, reconcile_to_ledger
from src.engines.research.performance import bca_interval

SPACE_VERSION = "LEARNER_SPACE_V1"
SPEC_VERSION = "LEARNING_LOOP_SPEC_V1"
MIN_SELECTION_TRADES = be.MIN_GROUP_TRADES        # a candidate needs this many finished trades before it may be chosen
MIN_OOS_TRADES = 60                                 # pooled out-of-sample trades needed before any promotion
MIN_OOS_BLOCKS = 4                                  # completed quarters with out-of-sample trades
STABILITY_BLOCKS = 2                                # the choice must not have changed over the last N blocks
PROBATION_MIN_TRADES = 20                           # forward trades before an active version can be judged
FIRST_ORIGIN = date(2024, 1, 1)
BOOT = 5000
SEED = 20261110
CAPITAL = 1_000_000.0
NOTIONAL = 100_000.0


# ---------------------------------------------------------------------------------------------------- the menu

@dataclass(frozen=True)
class Cand:
    plan: str
    entry: str
    market: str

    @property
    def name(self) -> str:
        return f"BO::{self.plan}::{self.entry}::{self.market}"

    @property
    def params(self) -> Dict[str, str]:
        return {"plan": self.plan, "entry": self.entry, "market": self.market}

    @property
    def sha256(self) -> str:
        return hashlib.sha256(json.dumps({"space": SPACE_VERSION, **self.params}, sort_keys=True).encode()).hexdigest()


def space() -> List[Cand]:
    return [Cand(p["id"], e["id"], m["id"]) for p, e, m in product(be.PLANS, be.ENTRY_FILTERS, be.MARKET_FILTERS)]


CONTROL = Cand("T1_10D", "ANY_OPEN", "ANY_MARKET")   # the current plan: Target 1 or stop, 10-session limit, no filters


def _plan(c: Cand) -> Dict[str, Any]:
    return next(p for p in be.PLANS if p["id"] == c.plan)


def _entry(c: Cand) -> Optional[float]:
    return next(e["max_ext_pct"] for e in be.ENTRY_FILTERS if e["id"] == c.entry)


def _uptrend_only(c: Cand) -> bool:
    return next(m["uptrend"] for m in be.MARKET_FILTERS if m["id"] == c.market)


# --------------------------------------------------------------------------------------------- simulate candidates

def simulate_all(signals: Dict[str, Dict[str, Any]], nifty) -> Dict[Cand, List[Dict[str, Any]]]:
    """Every candidate's finished trades over the whole series (same engine and costs as the breakout study)."""
    ce = CostEngine(CostModelConfig())

    def cost(entry: float, px: float, days: int) -> float:
        return float(ce.calculate_trade("X", entry, px, quantity=max(1, int(NOTIONAL // entry)), holding_days=max(1, days)).net_return_pct)

    up = be._regime(nifty)
    out: Dict[Cand, List[Dict[str, Any]]] = {}
    for c in space():
        allowed = (lambda dt: bool(up(dt))) if _uptrend_only(c) else (lambda dt: True)
        rows: List[Dict[str, Any]] = []
        for sym, d in signals.items():
            for r in be._simulate(d, _plan(c), _entry(c), allowed, cost):
                rows.append({**r, "symbol": sym})
        rows.sort(key=lambda r: (r["entry_date"], r["symbol"]))
        out[c] = rows
    return out


# ------------------------------------------------------------------------------------------------ walk-forward

def quarter_starts(first: date, last: date) -> List[date]:
    out, y, q = [], first.year, (first.month - 1) // 3
    d = date(y, q * 3 + 1, 1)
    while d <= last:
        if d >= first:
            out.append(d)
        q += 1
        if q == 4:
            q, y = 0, y + 1
        d = date(y, q * 3 + 1, 1)
    return out


def _mean(rows: Sequence[Dict[str, Any]]) -> Optional[float]:
    return float(np.mean([r["net_return_pct"] for r in rows])) if rows else None


def select(rows_by_cand: Dict[Cand, List[Dict[str, Any]]], before: date) -> Tuple[Cand, str, Dict[str, Any]]:
    """Best candidate on trades that FINISHED before `before` (trades still open at that date are unknown to the learner)."""
    cutoff = before.isoformat()
    table: Dict[str, Any] = {}
    best, best_mean = None, -math.inf
    for c in space():
        done = [r for r in rows_by_cand[c] if r["exit_date"] < cutoff]
        m = _mean(done)
        table[c.name] = {"trades": len(done), "mean_net_pct": None if m is None else round(m, 4)}
        if len(done) >= MIN_SELECTION_TRADES and m is not None and m > best_mean:
            best, best_mean = c, m
    if best is None:
        return CONTROL, "NO_ELIGIBLE_CANDIDATE", table
    return best, "BEST_MEAN_NET_ON_FINISHED_TRADES", table


def walk_forward(rows_by_cand: Dict[Cand, List[Dict[str, Any]]], data_through: date, first_origin: date = FIRST_ORIGIN) -> Dict[str, Any]:
    """Replays the picking procedure quarter by quarter. Completed blocks feed the gates; the in-progress quarter is reported only."""
    origins = quarter_starts(first_origin, data_through)
    blocks: List[Dict[str, Any]] = []
    pol, ctl = [], []
    for i, t in enumerate(origins):
        nxt = origins[i + 1] if i + 1 < len(origins) else None
        end_excl = nxt if nxt else data_through + timedelta(days=1)
        completed = nxt is not None and nxt <= data_through + timedelta(days=1)
        chosen, basis, _ = select(rows_by_cand, t)
        lo, hi = t.isoformat(), end_excl.isoformat()
        p = [r for r in rows_by_cand[chosen] if lo <= r["entry_date"] < hi]
        c = [r for r in rows_by_cand[CONTROL] if lo <= r["entry_date"] < hi]
        blocks.append({"origin": lo, "completed": completed, "chosen": chosen.name, "basis": basis, "policy_trades": len(p), "control_trades": len(c),
                       "policy_mean_net_pct": _round(_mean(p)), "control_mean_net_pct": _round(_mean(c))})
        if completed:
            pol += p
            ctl += c
    return {"origins": [o.isoformat() for o in origins], "blocks": blocks, "policy_rows": pol, "control_rows": ctl}


def _round(x: Optional[float], n: int = 4) -> Optional[float]:
    return None if x is None else round(float(x), n)


def diff_interval(a: Sequence[float], b: Sequence[float], confidence: float = 0.95, n_boot: int = BOOT, seed: int = SEED) -> Optional[Dict[str, float]]:
    """Percentile bootstrap of mean(a) - mean(b), groups resampled independently; None when either group has fewer than 5 trades."""
    a, b = np.asarray(a, float), np.asarray(b, float)
    if len(a) < 5 or len(b) < 5:
        return None
    rng = np.random.default_rng(seed)
    da = a[rng.integers(0, len(a), size=(n_boot, len(a)))].mean(axis=1)
    db = b[rng.integers(0, len(b), size=(n_boot, len(b)))].mean(axis=1)
    d = da - db
    lo, hi = np.quantile(d, [(1 - confidence) / 2, 1 - (1 - confidence) / 2])
    return {"estimate": float(a.mean() - b.mean()), "low": float(lo), "high": float(hi)}


def evaluate_gates(wf: Dict[str, Any], current: Cand) -> Dict[str, Any]:
    pol = [r["net_return_pct"] for r in wf["policy_rows"]]
    ctl = [r["net_return_pct"] for r in wf["control_rows"]]
    done = [b for b in wf["blocks"] if b["completed"]]
    blocks_with_trades = sum(1 for b in done if b["policy_trades"] > 0)
    abs_ci = bca_interval(pol, np.mean, 2000) if len(pol) >= 5 else None
    dif = diff_interval(pol, ctl)
    recent = [b["chosen"] for b in wf["blocks"]][-(STABILITY_BLOCKS + 1):]   # last completed blocks + the in-progress choice
    gates = {
        "G1_sample": {"pass": len(pol) >= MIN_OOS_TRADES and blocks_with_trades >= MIN_OOS_BLOCKS,
                      "detail": f"{len(pol)} out-of-sample trades (need {MIN_OOS_TRADES}) over {blocks_with_trades} completed quarters (need {MIN_OOS_BLOCKS})"},
        "G2_positive_after_costs": {"pass": bool(abs_ci and abs_ci["low"] > 0),
                                    "detail": "95% BCa interval of the out-of-sample mean net return must be entirely above zero",
                                    "interval": abs_ci},
        "G3_beats_control": {"pass": bool(dif and dif["low"] > 0),
                             "detail": "95% bootstrap interval of (policy mean - control mean) must be entirely above zero", "interval": dif},
        "G4_stable_selection": {"pass": len(recent) == STABILITY_BLOCKS + 1 and len(set(recent)) == 1 and recent[-1] == current.name and current != CONTROL,
                                "detail": f"the choice must be the same non-control candidate over the last {STABILITY_BLOCKS + 1} quarters", "recent_choices": recent},
    }
    return {"gates": gates, "passed": all(g["pass"] for g in gates.values()), "policy_oos_trades": len(pol), "control_oos_trades": len(ctl),
            "policy_mean_net_pct": _round(_mean(wf["policy_rows"])), "control_mean_net_pct": _round(_mean(wf["control_rows"])),
            "policy_ci": abs_ci, "diff_ci": dif}


def probation(rows_by_cand: Dict[Cand, List[Dict[str, Any]]], active: Cand, since: date) -> Dict[str, Any]:
    """Forward trades of the active version against control, using only entries AFTER the promotion's data cut-off."""
    lo = (since + timedelta(days=1)).isoformat()
    a = [r for r in rows_by_cand[active] if r["entry_date"] >= lo]
    c = [r for r in rows_by_cand[CONTROL] if r["entry_date"] >= lo]
    dif = diff_interval([r["net_return_pct"] for r in a], [r["net_return_pct"] for r in c])
    judged = len(a) >= PROBATION_MIN_TRADES
    m_a, m_c = _mean(a), _mean(c)
    failed = bool(judged and m_a is not None and m_c is not None and m_a < m_c)
    return {"forward_trades": len(a), "control_forward_trades": len(c), "judged": judged, "failed": failed, "active_mean_net_pct": _round(m_a),
            "control_mean_net_pct": _round(m_c), "diff_ci": dif,
            "rule": f"after {PROBATION_MIN_TRADES} forward trades, roll back if the active version's mean net return is below control's"}


# ---------------------------------------------------------------------------------------------------- P&L audit

def pnl_audit(rows: Sequence[Dict[str, Any]], label: str) -> Dict[str, Any]:
    """Every trade re-priced through the itemized cost ledger and replayed chronologically on a Rs 10 lakh account.
    The equity curve must end exactly at capital + the sum of the ledger's net P&L; concurrent positions above 10 are flagged."""
    ce = CostEngine(CostModelConfig())
    trades, total, gross, costs = [], 0.0, 0.0, 0.0
    for r in rows:
        qty = max(1, int(NOTIONAL // r["entry"]))
        c = ce.calculate_trade(r["symbol"], r["entry"], r["exit"], quantity=qty, holding_days=max(1, r["bars"]))
        total += c.net_pnl
        gross += c.gross_pnl_reference
        costs += c.total_costs
        trades.append(EqTrade(date.fromisoformat(r["entry_date"]), date.fromisoformat(r["exit_date"]), c.net_pnl, qty * r["entry"]))
    if not trades:
        return {"label": label, "trades": 0}
    curve = equity_curve(trades, CAPITAL, mtm=False)
    rec = reconcile_to_ledger(curve, trades, CAPITAL)
    days = sorted({t.entry for t in trades} | {t.exit for t in trades})
    peak = max(sum(1 for t in trades if t.entry <= d <= t.exit) for d in days)
    return {"label": label, "trades": len(trades), "capital_inr": CAPITAL, "notional_per_trade_inr": NOTIONAL,
            "gross_pnl_inr": round(gross, 2), "total_costs_inr": round(costs, 2), "net_pnl_inr": round(total, 2), "ending_equity_inr": round(curve[-1][1], 2),
            "reconciled_to_ledger": rec["reconciled"], "reconciliation_difference_inr": rec["difference_inr"],
            "max_drawdown": max_drawdown(curve), "peak_concurrent_positions": peak,
            "capital_warning": None if peak * NOTIONAL <= CAPITAL else f"{peak} positions overlap: Rs {peak * NOTIONAL:,.0f} would be needed, more than the Rs {CAPITAL:,.0f} account; the P&L is a per-trade ledger sum, not an affordable portfolio",
            "cost_model": CostModelConfig().version}


# ------------------------------------------------------------------------------------------------------ lessons

def lesson_for(r: Dict[str, Any]) -> Tuple[List[str], str]:
    """Tags and a plain sentence for a resolved tracked breakout. Descriptive only: a tag is not a rule until it passes the
    same walk-forward test as everything else. Returns are gross (before costs), as the tracker records them."""
    v, sess = r["verdict"], r.get("sessions") or 0
    ret, best, worst = r.get("return_pct"), r.get("best_pct"), r.get("worst_pct")
    tags = [f"ORIGIN_{r['origin']}", f"VERDICT_{v}"]
    f = lambda x: "n/a" if x is None else f"{float(x):+.1f}%"
    if v == "WORKED":
        text = f"Worked: reached Target 1 in {sess} session(s); the best move before the end was {f(best)}."
        tags.append("FAST_WIN" if sess <= 2 else "SLOW_WIN")
    elif v == "FAILED":
        text = f"Failed: stopped out after {sess} session(s), {f(ret)} gross; it had reached {f(best)} at its best."
        if sess <= 2:
            tags.append("FAST_FAILURE")
        if best is not None and float(best) >= 2.0:
            tags.append("GAVE_BACK_GAINS")
        if best is not None and float(best) < 1.0:
            tags.append("NEVER_FOLLOWED_THROUGH")
    elif v == "NO_FOLLOW_THROUGH":
        text = f"No follow-through: neither target nor stop in {sess} sessions; ended {f(ret)} gross (best {f(best)}, worst {f(worst)})."
        tags.append("TIME_EXPIRED")
    elif v == "NOT_TRADEABLE":
        ext = None
        if r.get("entry_price") and r.get("entry_high"):
            ext = (float(r["entry_price"]) / float(r["entry_high"]) - 1) * 100
        text = f"Not tradeable: the next open was already above Target 1{'' if ext is None else f' ({ext:+.1f}% above the entry zone)'}, so no trade was possible."
        tags.append("GAPPED_PAST_TARGET")
    else:
        text, tags = "Unresolved.", tags
    rv, sc, nf = r.get("rvol"), r.get("structure_score"), r.get("nifty_return_pct")
    if rv is not None:
        tags.append("LOW_VOLUME_CONFIRMATION" if float(rv) < 1.5 else "HIGH_VOLUME_CONFIRMATION" if float(rv) >= 2.5 else "NORMAL_VOLUME")
    if sc is not None:
        tags.append("HIGH_STRUCTURE_SCORE" if float(sc) >= 70 else "LOW_STRUCTURE_SCORE" if float(sc) < 50 else "MID_STRUCTURE_SCORE")
    if nf is not None and v in ("WORKED", "FAILED", "NO_FOLLOW_THROUGH"):
        tags.append("MARKET_HEADWIND" if float(nf) < -1 else "MARKET_TAILWIND" if float(nf) > 1 else "MARKET_FLAT")
    return tags, text


def build_lessons(conn) -> int:
    """One lesson per newly resolved tracked breakout (append-only; a PENDING one is lessoned when it resolves)."""
    with conn.cursor() as cur:
        cur.execute("""SELECT t.id, t.origin, t.symbol_id, t.signal_date, t.verdict, t.return_pct::float8, t.best_pct::float8, t.worst_pct::float8, t.sessions,
                              t.rvol::float8, t.structure_score::float8, t.nifty_return_pct::float8, t.entry_price::float8, t.entry_high::float8
                       FROM breakout_tracking t LEFT JOIN breakout_lessons l ON l.tracking_id = t.id
                       WHERE l.tracking_id IS NULL AND t.verdict <> 'PENDING' ORDER BY t.id""")
        cols = [d[0] for d in cur.description]
        rows = [dict(zip(cols, x)) for x in cur.fetchall()]
        for r in rows:
            tags, text = lesson_for(r)
            cur.execute("""INSERT INTO breakout_lessons (tracking_id, origin, symbol_id, signal_date, verdict, return_pct, best_pct, worst_pct, sessions, rvol,
                                                         structure_score, nifty_return_pct, tags, lesson)
                           VALUES (%(id)s, %(origin)s, %(symbol_id)s, %(signal_date)s, %(verdict)s, %(return_pct)s, %(best_pct)s, %(worst_pct)s, %(sessions)s, %(rvol)s,
                                   %(structure_score)s, %(nifty_return_pct)s, %(tags)s, %(lesson)s) ON CONFLICT (tracking_id) DO NOTHING""",
                        {**r, "tags": tags, "lesson": text})
    conn.commit()
    return len(rows)


# --------------------------------------------------------------------------------------------------- persistence

def ensure_space(conn) -> Dict[str, int]:
    """Creates the 24 pre-registered versions once (control first); idempotent."""
    ids: Dict[str, int] = {}
    with conn.cursor() as cur:
        for c in [CONTROL] + [x for x in space() if x != CONTROL]:
            cur.execute("SELECT id FROM strategy_versions WHERE params_sha256 = %s", (c.sha256,))
            row = cur.fetchone()
            if row:
                ids[c.name] = row[0]
                continue
            parent = ids.get(CONTROL.name) if c != CONTROL else None
            why = ("Frozen control: the current breakout plan (Target 1 or stop, 10-session limit, no filters). Never modified or removed."
                   if c == CONTROL else f"Pre-registered candidate in {SPACE_VERSION}: exit plan {c.plan}, entry filter {c.entry}, market filter {c.market}.")
            cur.execute("""INSERT INTO strategy_versions (name, space_version, params, params_sha256, parent_id, origin, rationale)
                           VALUES (%s, %s, %s, %s, %s, %s, %s) RETURNING id""",
                        (c.name, SPACE_VERSION, Json(c.params), c.sha256, parent, "FROZEN_CONTROL" if c == CONTROL else "LEARNER_SPACE", why))
            ids[c.name] = cur.fetchone()[0]
            cur.execute("INSERT INTO strategy_status_events (version_id, status, reason, actor) VALUES (%s, %s, %s, 'SEED')",
                        (ids[c.name], "CONTROL" if c == CONTROL else "CANDIDATE", "seeded from the pre-registered space"))
    conn.commit()
    return ids


def current_active(conn) -> Optional[Tuple[int, Cand, date]]:
    """(version id, candidate, data cut-off of the run that promoted it) for the active paper version, if any."""
    with conn.cursor() as cur:
        cur.execute("SELECT id, params FROM strategy_current WHERE status = 'ACTIVE_PAPER' ORDER BY status_at DESC LIMIT 1")
        row = cur.fetchone()
        if not row:
            return None
        cur.execute("""SELECT r.data_through FROM strategy_status_events ev JOIN strategy_evaluations se ON se.id = ev.evaluation_id
                       JOIN learning_runs r ON r.id = se.run_id WHERE ev.version_id = %s AND ev.status = 'ACTIVE_PAPER' ORDER BY ev.id DESC LIMIT 1""", (row[0],))
        since = cur.fetchone()
    p = row[1]
    return row[0], Cand(p["plan"], p["entry"], p["market"]), since[0] if since else date.today()


def blocked_version_ids(conn) -> set:
    with conn.cursor() as cur:
        cur.execute("SELECT DISTINCT version_id FROM strategy_status_events WHERE status IN ('PROBATION_FAILED', 'ROLLED_BACK')")
        return {r[0] for r in cur.fetchall()}


def _event(cur, vid: int, status: str, reason: str, actor: str, eval_id: Optional[int] = None) -> None:
    cur.execute("INSERT INTO strategy_status_events (version_id, status, reason, evaluation_id, actor) VALUES (%s, %s, %s, %s, %s)", (vid, status, reason, eval_id, actor))


def _eval(cur, run_id: int, vid: int, kind: str, g: Dict[str, Any]) -> int:
    dc = g.get("diff_ci") or {}
    pc = g.get("policy_ci") or {}
    cur.execute("""INSERT INTO strategy_evaluations (run_id, version_id, kind, oos_trades, oos_mean_net_pct, oos_ci_low, oos_ci_high, control_trades, control_mean_net_pct,
                                                     diff_mean_pct, diff_ci_low, diff_ci_high, gates, passed)
                   VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s) RETURNING id""",
                (run_id, vid, kind, g.get("policy_oos_trades", g.get("forward_trades", 0)), g.get("policy_mean_net_pct", g.get("active_mean_net_pct")),
                 pc.get("low"), pc.get("high"), g.get("control_oos_trades", g.get("control_forward_trades", 0)), g.get("control_mean_net_pct"),
                 dc.get("estimate"), dc.get("low"), dc.get("high"), Json(json.loads(json.dumps(g.get("gates", {"probation": g}), default=str))), bool(g.get("passed", not g.get("failed", False)))))
    return cur.fetchone()[0]


# ---------------------------------------------------------------------------------------------------- the run

def decide_and_record(conn, rows_by_cand: Dict[Cand, List[Dict[str, Any]]], data_through: date, ids: Optional[Dict[str, int]] = None) -> Dict[str, Any]:
    """Pure decision + persistence given every candidate's simulated trades. Used by run() and by the tests."""
    ids = ids or ensure_space(conn)
    wf = walk_forward(rows_by_cand, data_through)
    last_origin = date.fromisoformat(wf["blocks"][-1]["origin"]) if wf["blocks"] else FIRST_ORIGIN
    current, basis, sel_table = select(rows_by_cand, last_origin)
    g = evaluate_gates(wf, current)
    # G5: a version that was ever rolled back (probation or owner) is not promoted again within this menu: no flip-flopping.
    blocked = blocked_version_ids(conn)
    g["gates"]["G5_not_previously_rolled_back"] = {"pass": ids.get(current.name) not in blocked,
                                                    "detail": "a version that failed probation or was rolled back by the owner is never promoted again in this space version"}
    g["passed"] = all(x["pass"] for x in g["gates"].values())
    active = current_active(conn)
    prob = probation(rows_by_cand, active[1], active[2]) if active else None

    if active and prob and prob["failed"]:
        decision = "ROLLED_BACK_PROBATION"
    elif g["passed"] and current != CONTROL and not (active and active[1] == current):
        decision = "PROMOTED"
    elif active:
        decision = "KEPT_ACTIVE"
    elif g["gates"]["G1_sample"]["pass"]:
        decision = "NO_CHANGE_GATES_NOT_MET"
    else:
        decision = "NO_CHANGE_INSUFFICIENT_DATA"

    summary_rows = [{"candidate": c.name, **{k: v for k, v in sel_table[c.name].items()}} for c in space()]
    audit = [pnl_audit(rows_by_cand[CONTROL], "CONTROL " + CONTROL.name)]
    shown = (active[1] if active and decision != "ROLLED_BACK_PROBATION" else current)
    if shown != CONTROL:
        audit.append(pnl_audit(rows_by_cand[shown], ("ACTIVE " if active and shown == active[1] else "SELECTED ") + shown.name))
    result = {
        "spec": SPEC_VERSION, "space": SPACE_VERSION, "data_through": data_through.isoformat(), "series": be.SERIES_VERSION,
        "candidates_tried": len(space()), "current_selection": current.name, "selection_basis": basis, "selection_table_on_finished_trades": summary_rows,
        "walk_forward_blocks": wf["blocks"], "gates": g["gates"], "gates_passed": g["passed"],
        "policy_oos": {"trades": g["policy_oos_trades"], "mean_net_pct": g["policy_mean_net_pct"], "ci": g["policy_ci"]},
        "control_oos": {"trades": g["control_oos_trades"], "mean_net_pct": g["control_mean_net_pct"]}, "difference_ci": g["diff_ci"],
        "probation": prob, "pnl_audit": audit,
        "multiple_testing_note": (f"{len(space())} candidates exist. The gates are applied to the selection PROCEDURE's pooled out-of-sample trades, not to the best-looking "
                                  "candidate's in-sample score, and only completed quarters count, so re-running the learner cannot improve its odds."),
        "limitations": be.SERIES_LIMITATIONS,
    }
    with conn.cursor() as cur:
        cur.execute("INSERT INTO learning_runs (space_version, data_through, candidates_tried, result, decision) VALUES (%s,%s,%s,%s,%s) RETURNING id",
                    (SPACE_VERSION, data_through, len(space()), Json(json.loads(json.dumps(result, default=str))), decision))
        run_id = cur.fetchone()[0]
        ev_id = _eval(cur, run_id, ids[current.name], "SELECTION_POLICY_WALK_FORWARD", g)
        if active and prob is not None:
            _eval(cur, run_id, active[0], "FORWARD_PROBATION", {**prob, "passed": not prob["failed"]})
        if decision == "ROLLED_BACK_PROBATION":
            _event(cur, active[0], "PROBATION_FAILED", f"forward mean {prob['active_mean_net_pct']}% < control {prob['control_mean_net_pct']}% over {prob['forward_trades']} trades", "LEARNER", ev_id)
        elif decision == "PROMOTED":
            if active:
                _event(cur, active[0], "RETIRED", f"superseded by {current.name}", "LEARNER", ev_id)
            _event(cur, ids[current.name], "ACTIVE_PAPER", "passed every gate in " + SPEC_VERSION, "LEARNER", ev_id)
    conn.commit()
    return {"run_id": run_id, "decision": decision, "current_selection": current.name, "gates_passed": g["passed"], "gates": {k: v["pass"] for k, v in g["gates"].items()},
            "policy_oos_trades": g["policy_oos_trades"], "probation": prob}


def run(conn) -> Dict[str, Any]:
    """Full nightly pass. One engine pass over the NSE series feeds everything: reasons for tracked breakouts, upcoming-candidate flags and
    their outcomes, per-stock tests, the learner's menu simulation and its decision. Slow (about a minute)."""
    import pandas as pd
    from src.engines.research import breakout_watch as bw
    lessons = build_lessons(conn)
    df, _series = be.build_series(conn)
    df = df.rename(columns={"date": "timestamp"})
    keep = df.groupby("symbol").size()
    keep = keep[keep >= be.MIN_HISTORY_BARS].index
    frame = df[df["symbol"].isin(keep)].copy()
    frame["timestamp"] = pd.to_datetime(frame["timestamp"])
    analysis = bw.analyze_universe(frame)
    signals = bw.signals_from(analysis)
    nifty = be._nifty(conn)
    through = pd.Timestamp(frame["timestamp"].max()).date()

    watch = {"reasons_backfilled": bw.backfill_reasons(conn, analysis),
             "history_flags_added": bw.store_history(conn, analysis, shariah=be._shariah(conn)),
             "resolution": bw.resolve(conn, analysis)}
    stats = bw.stock_stats(analysis, signals, nifty)
    watch["stock_tests_stored"] = bw.store_stock_stats(conn, stats, through.isoformat())
    try:    # a failure here must never stop the learner's own decision below
        from src.engines.research import pattern_replay as pr
        watch["pattern_replay"] = pr.run_from_analysis(conn, analysis, nifty, through)
    except Exception as exc:
        conn.rollback()
        watch["pattern_replay"] = {"error": str(exc)[:300]}
    rates = bw.universe_rates(analysis)
    with conn.cursor() as cur:
        cur.execute("INSERT INTO research_runs (kind, version, series_version, result) VALUES (%s, %s, %s, %s)",
                    ("CANDIDATE_LIFT", "CANDIDATE_LIFT_V1", be.SERIES_VERSION, Json({**rates, "data_through": through.isoformat()})))
    conn.commit()

    rows = simulate_all(signals, nifty)
    out = decide_and_record(conn, rows, through)
    out["lessons_added"] = lessons
    out["data_through"] = through.isoformat()
    out["watch"] = watch
    out["candidate_lift"] = rates
    return out
