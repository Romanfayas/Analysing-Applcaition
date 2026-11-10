"""P1.9.28 governed learning loop: pure logic, then PostgreSQL behaviour on a scratch database built from the real migrations."""
import os
import uuid
from datetime import date, datetime, timedelta

import numpy as np
import psycopg2
import pytest

from src.engines.research import learning as L
from src.engines.research.learning import CONTROL, Cand

X = Cand("TRAIL_2ATR_20D", "ANY_OPEN", "UPTREND")      # the candidate the synthetic world makes genuinely better
Y = Cand("HOLD_40D", "WITHIN_1PCT", "ANY_MARKET")


def trade(day: date, net: float, sym="S0", bars=3):
    return {"entry_date": day.isoformat(), "exit_date": (day + timedelta(days=bars)).isoformat(), "net_return_pct": float(net), "entry": 100.0,
            "exit": 100.0 * (1 + net / 100), "bars": bars, "symbol": sym, "gross": net, "why": "TARGET", "period": "HOLDOUT"}


def world(better=X, start=date(2023, 1, 2), end=date(2026, 10, 30), per_q=20, mu_better=1.5, mu_other=-0.5, seed=7, worse_after=None):
    """Every candidate gets `per_q` trades per quarter. `better` truly earns mu_better; all others (control included) mu_other.
    worse_after: (date, mean) makes `better` earn `mean` from that date on (to exercise probation)."""
    rng = np.random.default_rng(seed)
    out = {}
    for c in L.space():
        rows, d = [], start
        qs = L.quarter_starts(date(start.year, 1, 1), end)
        for i, q in enumerate(qs):
            nxt = qs[i + 1] if i + 1 < len(qs) else end + timedelta(days=1)
            span = max(1, (nxt - q).days - 6)
            for k in range(per_q):
                day = q + timedelta(days=int(k * span / per_q))
                if day < start or day > end:
                    continue
                mu = mu_better if c == better else mu_other
                if c == better and worse_after and day >= worse_after[0]:
                    mu = worse_after[1]
                rows.append(trade(day, rng.normal(mu, 1.0), sym=f"S{k % 5}"))
        rows.sort(key=lambda r: (r["entry_date"], r["symbol"]))
        out[c] = rows
    return out


# ---------------------------------------------------------------------------------------------- the menu
def test_menu_is_finite_unique_and_contains_control():
    s = L.space()
    assert len(s) == 24 and CONTROL in s
    assert len({c.name for c in s}) == 24 and len({c.sha256 for c in s}) == 24


def test_quarter_starts():
    assert L.quarter_starts(date(2024, 1, 1), date(2024, 11, 6)) == [date(2024, 1, 1), date(2024, 4, 1), date(2024, 7, 1), date(2024, 10, 1)]


# ---------------------------------------------------------------------------------------------- no future leakage
def test_selection_ignores_trades_that_finish_after_the_cutoff_and_needs_enough_trades():
    rows = {c: [trade(date(2024, 1, 1) + timedelta(days=i), -0.5) for i in range(40)] for c in L.space()}
    # X looks superb, but every one of its trades finishes on/after the cutoff: the learner must not see them
    rows[X] = [trade(date(2024, 4, 1) - timedelta(days=1) + timedelta(days=i), 9.0, bars=3) for i in range(40)]
    chosen, basis, _ = L.select(rows, date(2024, 4, 1))
    assert chosen != X
    # fewer than MIN_SELECTION_TRADES finished trades -> nobody eligible -> control, and it says so
    few = {c: [trade(date(2024, 1, 1) + timedelta(days=i), 1.0) for i in range(5)] for c in L.space()}
    assert L.select(few, date(2024, 6, 1))[:2] == (CONTROL, "NO_ELIGIBLE_CANDIDATE")


def test_a_future_only_winner_is_never_picked_in_advance():
    """X is poor until 2025-07-01 and superb after. At every origin before then the procedure must NOT have chosen it."""
    rows = world(better=X, mu_better=-0.8, mu_other=-0.5)
    boom = date(2025, 7, 1)
    rows[X] = [r for r in rows[X] if r["entry_date"] < boom.isoformat()] + [trade(boom + timedelta(days=i), 5.0, bars=2) for i in range(60)]
    wf = L.walk_forward(rows, date(2026, 10, 30))
    early = [b for b in wf["blocks"] if b["origin"] < "2025-07-01"]
    assert early and all(b["chosen"] != X.name for b in early)


def test_only_completed_quarters_feed_the_pooled_results():
    rows = world()
    wf = L.walk_forward(rows, date(2026, 11, 6))
    assert wf["blocks"][-1]["completed"] is False and wf["blocks"][-1]["origin"] == "2026-10-01"
    assert all(b["completed"] for b in wf["blocks"][:-1])
    pooled_max = max(r["entry_date"] for r in wf["policy_rows"])
    assert pooled_max < "2026-10-01"


# ---------------------------------------------------------------------------------------------- gates
def test_gates_pass_when_the_procedure_really_beats_control():
    rows = world()
    wf = L.walk_forward(rows, date(2026, 11, 6))
    cur, _, _ = L.select(rows, date(2026, 10, 1))
    g = L.evaluate_gates(wf, cur)
    assert cur == X and g["passed"], g["gates"]
    assert g["policy_mean_net_pct"] > g["control_mean_net_pct"] and g["diff_ci"]["low"] > 0 and g["policy_ci"]["low"] > 0


def test_gates_fail_when_nothing_beats_control_and_when_there_is_too_little_data():
    flat = world(better=X, mu_better=-0.5, mu_other=-0.5)
    wf = L.walk_forward(flat, date(2026, 11, 6))
    cur, _, _ = L.select(flat, date(2026, 10, 1))
    assert not L.evaluate_gates(wf, cur)["passed"]
    tiny = world(per_q=3)
    wf2 = L.walk_forward(tiny, date(2026, 11, 6))
    g2 = L.evaluate_gates(wf2, L.select(tiny, date(2026, 10, 1))[0])
    assert not g2["gates"]["G1_sample"]["pass"] and not g2["passed"]


def test_unstable_selection_fails_g4():
    rows = world()
    wf = L.walk_forward(rows, date(2026, 11, 6))
    for b in wf["blocks"][-3:-1]:
        b["chosen"] = Y.name           # the choice flipped recently
    assert not L.evaluate_gates(wf, X)["gates"]["G4_stable_selection"]["pass"]


def test_diff_interval_needs_samples_and_separates_clear_differences():
    assert L.diff_interval([1, 2], [1, 2, 3, 4, 5, 6]) is None
    ci = L.diff_interval(list(np.random.default_rng(1).normal(2, 1, 60)), list(np.random.default_rng(2).normal(0, 1, 60)))
    assert ci["low"] > 0
    ci0 = L.diff_interval(list(np.random.default_rng(3).normal(0, 1, 60)), list(np.random.default_rng(4).normal(0, 1, 60)))
    assert ci0["low"] < 0 < ci0["high"]


def test_probation_rolls_back_only_after_enough_forward_trades():
    rows = world(worse_after=(date(2026, 1, 1), -3.0))
    since = date(2025, 12, 31)
    p = L.probation(rows, X, since)
    assert p["judged"] and p["failed"] and p["active_mean_net_pct"] < p["control_mean_net_pct"]
    early = L.probation(rows, X, date(2026, 10, 15))
    assert not early["judged"] and not early["failed"]


# ---------------------------------------------------------------------------------------------- P&L audit
def test_pnl_audit_reconciles_to_the_cost_ledger():
    rows = [trade(date(2025, 1, 6) + timedelta(days=5 * i), 1.0 if i % 2 else -1.0, sym=f"S{i}", bars=3) for i in range(20)]
    a = L.pnl_audit(rows, "t")
    assert a["trades"] == 20 and a["reconciled_to_ledger"] and abs(a["reconciliation_difference_inr"]) < 0.01
    assert a["net_pnl_inr"] == pytest.approx(a["gross_pnl_inr"] - a["total_costs_inr"], abs=0.05)
    assert a["ending_equity_inr"] == pytest.approx(L.CAPITAL + a["net_pnl_inr"], abs=0.05)


def test_pnl_audit_flags_more_overlap_than_the_account_can_afford():
    rows = [trade(date(2025, 1, 6), 1.0, sym=f"S{i}", bars=5) for i in range(15)]
    assert "overlap" in L.pnl_audit(rows, "t")["capital_warning"]
    assert L.pnl_audit([], "t") == {"label": "t", "trades": 0}


# ---------------------------------------------------------------------------------------------- lessons
def tr(**kw):
    base = {"id": 1, "origin": "LIVE", "verdict": "WORKED", "sessions": 2, "return_pct": 1.5, "best_pct": 3.2, "worst_pct": -0.4, "rvol": 2.0,
            "structure_score": 75.0, "nifty_return_pct": 0.2, "entry_price": 100.0, "entry_high": 101.0}
    base.update(kw)
    return base


def test_lessons_tag_the_reason_for_each_outcome():
    t, s = L.lesson_for(tr())
    assert "FAST_WIN" in t and "HIGH_STRUCTURE_SCORE" in t and "MARKET_FLAT" in t and "Target 1" in s
    t, s = L.lesson_for(tr(verdict="FAILED", sessions=1, best_pct=0.3, return_pct=-3.0, rvol=1.1, nifty_return_pct=-1.8))
    assert {"FAST_FAILURE", "NEVER_FOLLOWED_THROUGH", "LOW_VOLUME_CONFIRMATION", "MARKET_HEADWIND"} <= set(t) and "stopped out" in s
    t, _ = L.lesson_for(tr(verdict="FAILED", sessions=6, best_pct=4.0))
    assert "GAVE_BACK_GAINS" in t
    t, s = L.lesson_for(tr(verdict="NOT_TRADEABLE", entry_price=108.0))
    assert "GAPPED_PAST_TARGET" in t and "+6.9%" in s
    t, _ = L.lesson_for(tr(verdict="NO_FOLLOW_THROUGH", sessions=10))
    assert "TIME_EXPIRED" in t


# ---------------------------------------------------------------------------------------------- PostgreSQL
MIG = os.getenv("P17_MIGRATIONS_DIR", os.path.join(os.path.dirname(__file__), "..", "..", "backend", "migrations"))


@pytest.fixture()
def scratch():
    url = os.getenv("ADMIN_DATABASE_URL") or os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    try:
        admin = psycopg2.connect(url)
        admin.autocommit = True
        name = "p1928_test_" + uuid.uuid4().hex[:8]
        admin.cursor().execute(f"CREATE DATABASE {name}")
    except Exception as e:
        pytest.skip(f"cannot create a scratch database ({type(e).__name__}); needs a role with CREATEDB")
    conn = psycopg2.connect(url.rsplit("/", 1)[0].split("?")[0] + "/" + name)
    cur = conn.cursor()
    cur.execute("SELECT 1 FROM pg_roles WHERE rolname = 'halal_app_user'")
    if not cur.fetchone():
        cur.execute("CREATE ROLE halal_app_user")
    cur.execute("CREATE TABLE symbols (id serial PRIMARY KEY, symbol text)")
    cur.execute("INSERT INTO symbols (symbol) SELECT 'S' || g FROM generate_series(1, 5) g")
    for f in ("44-p1927-breakout-tracking.sql", "45-p1928-learning-loop.sql"):
        cur.execute(open(os.path.join(MIG, f)).read())
    conn.commit()
    yield conn
    conn.close()
    admin.cursor().execute(f"DROP DATABASE {name} WITH (FORCE)")
    admin.close()


def q1(conn, sql, args=None):
    cur = conn.cursor()
    cur.execute(sql, args)
    r = cur.fetchone()
    conn.rollback()
    return r[0] if r else None


def test_db_space_is_created_once_with_control_first(scratch):
    ids = L.ensure_space(scratch)
    assert len(ids) == 24
    L.ensure_space(scratch)
    assert q1(scratch, "SELECT count(*) FROM strategy_versions") == 24 and q1(scratch, "SELECT count(*) FROM strategy_status_events") == 24
    assert q1(scratch, "SELECT name FROM strategy_current WHERE status = 'CONTROL'") == CONTROL.name
    assert q1(scratch, "SELECT count(*) FROM strategy_current WHERE status = 'CANDIDATE'") == 23


def test_db_append_only_blocks_edits_even_for_the_owner(scratch):
    L.ensure_space(scratch)
    cur = scratch.cursor()
    cur.execute("INSERT INTO learning_runs (space_version, data_through, candidates_tried, result, decision) VALUES ('T', '2026-01-01', 24, '{}', 'NO_CHANGE_INSUFFICIENT_DATA')")
    scratch.commit()
    for sql in ("UPDATE strategy_versions SET rationale = 'x'", "DELETE FROM strategy_status_events", "UPDATE learning_runs SET decision = 'PROMOTED'"):
        with pytest.raises(psycopg2.errors.RaiseException):
            cur.execute(sql)
        scratch.rollback()


def test_db_promotion_then_idempotent_rerun_then_probation_rollback_then_no_repromotion(scratch):
    ids = L.ensure_space(scratch)
    through = date(2026, 11, 6)
    r1 = L.decide_and_record(scratch, world(), through, ids)
    assert r1["decision"] == "PROMOTED" and r1["current_selection"] == X.name, r1
    assert q1(scratch, "SELECT name FROM strategy_current WHERE status = 'ACTIVE_PAPER'") == X.name
    # same data again: nothing new is promoted, the active version is kept, and a probation row is written
    r2 = L.decide_and_record(scratch, world(), through, ids)
    assert r2["decision"] == "KEPT_ACTIVE"
    assert q1(scratch, "SELECT count(*) FROM strategy_status_events WHERE status = 'ACTIVE_PAPER'") == 1
    assert q1(scratch, "SELECT count(*) FROM learning_runs") == 2
    # the active version then performs badly on forward data -> automatic rollback
    later = date(2027, 2, 15)
    bad = world(end=later, worse_after=(through + timedelta(days=1), -3.0))
    r3 = L.decide_and_record(scratch, bad, later, ids)
    assert r3["decision"] == "ROLLED_BACK_PROBATION" and r3["probation"]["failed"], r3
    assert q1(scratch, "SELECT count(*) FROM strategy_current WHERE status = 'ACTIVE_PAPER'") == 0
    assert q1(scratch, "SELECT status FROM strategy_current WHERE name = %s", (X.name,)) == "PROBATION_FAILED"
    # even if the gates were to pass again, that version is never promoted a second time
    r4 = L.decide_and_record(scratch, world(end=later), later, ids)
    assert r4["decision"] != "PROMOTED"
    assert q1(scratch, "SELECT count(*) FROM strategy_status_events WHERE status = 'ACTIVE_PAPER'") == 1


def test_db_no_promotion_when_gates_fail_and_the_reason_is_recorded(scratch):
    ids = L.ensure_space(scratch)
    r = L.decide_and_record(scratch, world(mu_better=-0.5), date(2026, 11, 6), ids)
    assert r["decision"] == "NO_CHANGE_GATES_NOT_MET" and not r["gates_passed"]
    assert q1(scratch, "SELECT count(*) FROM strategy_current WHERE status = 'ACTIVE_PAPER'") == 0
    res = q1(scratch, "SELECT result FROM learning_runs")
    assert res["candidates_tried"] == 24 and "multiple_testing_note" in res and res["pnl_audit"][0]["reconciled_to_ledger"]


def test_db_lessons_are_created_once_per_resolved_signal(scratch):
    cur = scratch.cursor()
    rows = [("WORKED", 2, 2.5, 3.0, -0.2), ("FAILED", 1, -3.1, 0.2, -3.1), ("PENDING", 0, 0.1, 0.4, -0.1), ("NOT_TRADEABLE", 1, 0.0, 0.0, 0.0)]
    for i, (v, s, ret, best, worst) in enumerate(rows, start=1):
        cur.execute("""INSERT INTO breakout_tracking (symbol_id, signal_date, origin, signal_close, entry_low, entry_high, stop_level, target_1, target_2, verdict, sessions,
                       return_pct, best_pct, worst_pct, rvol, structure_score, entry_price)
                       VALUES (%s, %s, 'LIVE', 100, 99, 101, 95, 110, 120, %s, %s, %s, %s, %s, 2.0, 72, 100)""",
                    (i, date(2026, 11, 2) + timedelta(days=i), v, s, ret, best, worst))
    scratch.commit()
    assert L.build_lessons(scratch) == 3                 # PENDING is skipped
    assert L.build_lessons(scratch) == 0                 # idempotent
    assert q1(scratch, "SELECT count(*) FROM breakout_lessons WHERE 'FAST_FAILURE' = ANY(tags)") == 1
    cur.execute("UPDATE breakout_tracking SET verdict = 'WORKED' WHERE verdict = 'PENDING'")
    scratch.commit()
    assert L.build_lessons(scratch) == 1                 # lessoned once it resolves
