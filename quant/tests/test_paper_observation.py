"""P1.7 prospective-observation tests: pure engine semantics + real PostgreSQL behaviour (scratch database built from the real migrations)."""
import json
import os
import threading
import uuid
from dataclasses import replace
from datetime import date, datetime, timedelta, timezone

import psycopg2
import pytest

from src.engines.research.paper_observation import (
    Bar, PaperObservationEngine, PaperOutcome, Status, evaluate, independent_subset, snapshot_hash, summarize,
)
from src.engines.research.data_audit import expected_sessions

UTC = timezone.utc
RULE = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
DECISION = datetime(2026, 3, 4, 11, 0, tzinfo=UTC)          # Wed 2026-03-04 after close (IST 16:30)
DSESS = date(2026, 3, 4)
SESS = expected_sessions(date(2026, 3, 5), date(2026, 4, 30))[:12]   # skips holidays 3/26 3/31 etc.


def rec(entry=100.0, stop=95.0, target=110.0, setup="CONFIRMED_BREAKOUT", **kw):
    eng = PaperObservationEngine()
    return eng.record_candidate("TEST", setup, entry, stop, target, "PASS", DECISION, RULE,
                                candidate_timestamp=DECISION, **kw)


def bars(spec):
    """spec: list of (o,h,l,c) for consecutive expected sessions after the decision."""
    return [Bar(SESS[i], *t) for i, t in enumerate(spec)]


FLAT = (90, 92, 88, 90)   # never reaches 100


# ---------------- pure engine ----------------
def test_naive_timestamps_and_future_cutoff_rejected():
    eng = PaperObservationEngine()
    with pytest.raises(ValueError):
        eng.record_candidate("T", "X", 100, 95, 110, "PASS", datetime(2026, 3, 4, 11, 0), RULE, candidate_timestamp=DECISION)
    with pytest.raises(ValueError):
        eng.record_candidate("T", "X", 100, 95, 110, "PASS", DECISION + timedelta(hours=1), RULE, candidate_timestamp=DECISION)
    with pytest.raises(ValueError):
        eng.record_candidate("T", "X", 100, 95, 110, "FAIL", DECISION, RULE, candidate_timestamp=DECISION)


def test_entry_not_assumed_and_no_trade_when_trigger_never_hit():
    r = evaluate(rec(), bars([FLAT] * 10), SESS[9])
    assert (r.status, r.outcome) == (Status.COMPLETED, PaperOutcome.NOT_TRIGGERED)
    assert r.realized_gross_return is None and r.realized_net_return is None and r.itemized_costs is None
    assert all(h["status"] == "NOT_ENTERED" for h in r.horizons.values())


def test_gap_up_fills_at_open_and_target_hit_later():
    r = evaluate(rec(), bars([(102, 104, 101, 103), (104, 111, 103, 110)]), SESS[1])
    assert r.entry_fill_price == 102.0 and r.outcome == PaperOutcome.TARGET_HIT and r.exit_price == 110.0
    assert r.realized_gross_return == pytest.approx(110 / 102 - 1, abs=1e-6)


def test_intrabar_fill_at_trigger():
    r = evaluate(rec(), bars([(98, 101, 97, 100), (100, 111, 99, 110)]), SESS[1])
    assert r.entry_fill_price == 100.0 and r.outcome == PaperOutcome.TARGET_HIT


def test_same_session_stop_and_target_resolves_to_stop():
    r = evaluate(rec(), bars([(99, 112, 94, 100)]), SESS[0])      # entry session touches both
    assert r.outcome == PaperOutcome.STOPPED_OUT and r.exit_price == 95.0
    r2 = evaluate(rec(), bars([(99, 101, 98, 100), (100, 112, 94, 100)]), SESS[1])
    assert r2.outcome == PaperOutcome.STOPPED_OUT and r2.exit_price == 95.0


def test_gap_through_stop_and_target_fill_at_open():
    r = evaluate(rec(), bars([(99, 101, 98, 100), (92, 93, 90, 91)]), SESS[1])
    assert r.outcome == PaperOutcome.STOPPED_OUT and r.exit_price == 92.0
    r = evaluate(rec(), bars([(99, 101, 98, 100), (115, 118, 114, 116)]), SESS[1])
    assert r.outcome == PaperOutcome.TARGET_HIT and r.exit_price == 115.0


def test_time_stop_after_ten_sessions():
    r = evaluate(rec(), bars([(99, 101, 98, 100)] + [(100, 102, 98, 101)] * 9), SESS[9])
    assert r.outcome == PaperOutcome.EXPIRED_TIME_STOP and r.exit_session == SESS[9] and r.exit_price == 101.0


def test_no_future_or_same_day_bar_leakage():
    leak = Bar(DSESS, 100, 130, 90, 120)          # decision-day bar with high >= trigger must be ignored
    r = evaluate(rec(), [leak] + bars([FLAT] * 3), SESS[2])
    assert r.entry_fill_price is None and r.status == Status.PENDING


def test_partial_data_stays_pending_with_partial_horizons():
    r = evaluate(rec(), bars([(99, 101, 98, 100), (100, 102, 99, 101), (101, 103, 99, 102)]), SESS[2])
    assert r.status == Status.PENDING and not r.is_complete
    assert r.horizons["1d"]["status"] == "OBSERVED" and r.horizons["3d"]["status"] == "OBSERVED"
    assert r.horizons["5d"]["status"] == "PENDING" and r.horizons["10d"]["status"] == "PENDING"
    assert r.horizons["3d"]["gross_return"] == pytest.approx(102 / 100 - 1, abs=1e-6)


def test_missing_session_marks_data_unavailable_and_excludes():
    b = bars([(99, 101, 98, 100), (100, 102, 99, 101), (101, 103, 99, 102)])
    del b[1]                                                       # hole at session 2 although data exists through session 3
    r = evaluate(rec(), b, SESS[2])
    assert (r.status, r.outcome) == (Status.DATA_UNAVAILABLE, PaperOutcome.DATA_UNAVAILABLE) and "no valid daily bar" in r.status_reason
    assert r.realized_net_return is None


def test_invalid_bar_is_data_unavailable_and_terminal_reason_censors():
    bad = [Bar(SESS[0], 100, 90, 95, 100)]                         # high < low
    assert evaluate(rec(), bad, SESS[0]).status == Status.DATA_UNAVAILABLE
    r = evaluate(rec(), bars([(99, 101, 98, 100)]) + [], SESS[3], terminal_reason="symbol delisted 2026-03-10")
    assert r.status == Status.CENSORED and "delisted" in r.status_reason


def test_repeated_evaluation_is_idempotent_and_terminal_untouched():
    b = bars([(99, 101, 98, 100), (100, 112, 99, 111)])
    r1 = evaluate(rec(), b, SESS[1])
    r2 = evaluate(rec(), b, SESS[1])
    assert r1 == r2
    again = evaluate(r1, b + bars([FLAT] * 2)[:0], SESS[1])
    assert again is r1                                             # terminal: returned as-is
    p = evaluate(rec(), b[:1], SESS[0])
    p2 = evaluate(p, b[:1], SESS[0])
    assert len(p2.sessions_observed) == 1 == len(p.sessions_observed)   # pending re-run does not duplicate observations


def test_snapshot_hash_unchanged_by_evaluation():
    r0 = rec()
    r1 = evaluate(r0, bars([(99, 101, 98, 100), (100, 112, 99, 111)]), SESS[1])
    assert snapshot_hash(r0) == snapshot_hash(r1) == r0.snapshot_sha256


def test_net_return_comes_from_itemized_ledger_not_a_constant():
    r = evaluate(rec(), bars([(99, 101, 98, 100), (100, 112, 99, 111)]), SESS[1])
    led = r.itemized_costs
    assert led["model_version"] == "COST_MODEL_2026_Q4_V2" and len(led["ledger"]) == 16
    qty = led["quantity"]
    assert r.realized_net_return == pytest.approx(led["net_pnl"] / (qty * r.entry_fill_price), abs=1e-6)
    assert r.realized_net_return < r.realized_gross_return
    assert abs((r.realized_gross_return - r.realized_net_return) * 1e4 - led["effective_cost_bps"]) < 15   # same order; not a fixed 28 bps
    assert r.realized_gross_return - r.realized_net_return != pytest.approx(0.0028, abs=1e-6)


def test_zero_sample_metrics_are_none_not_zero():
    s = summarize([])
    assert s["gross"]["mean_pct"] is None and s["net"]["win_rate_pct"] is None
    assert all(h["n"] == 0 and h["mean_pct"] is None and h["win_rate_pct"] is None and not h["metric_available"] for h in s["horizons"].values())
    s2 = summarize([rec()])        # one pending candidate
    assert s2["counts"]["completed_trades"] == 0 and s2["net"]["mean_pct"] is None
    assert s2["horizons"]["1d"]["pending"] == 1


def test_invalidated_excluded_and_independent_subset():
    a = evaluate(rec(), bars([(99, 101, 98, 100), (100, 112, 99, 111)]), SESS[1])
    b = evaluate(rec(), bars([(99, 101, 98, 100), (100, 112, 99, 111)]), SESS[1])   # same symbol, overlapping window
    assert len(independent_subset([a, b])) == 1
    s = summarize([a, b])
    assert s["counts"]["completed_trades"] == 2 and s["counts"]["independent_trades"] == 1
    assert s["significance_gate"]["gate_met"] is False


# ---------------- PostgreSQL behaviour on a scratch database built from the real migrations ----------------
MIG = os.getenv("P17_MIGRATIONS_DIR", "/app/migrations" if os.path.exists("/app/migrations") else (
    os.path.join(os.path.dirname(__file__), "..", "migrations") if os.path.exists(os.path.join(os.path.dirname(__file__), "..", "migrations")) else
    os.path.join(os.path.dirname(__file__), "..", "..", "backend", "migrations")
))


def _admin_url():
    return os.getenv("ADMIN_DATABASE_URL") or "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable"


@pytest.fixture(scope="module")
def scratch():
    name = "p17_test_" + uuid.uuid4().hex[:8]
    try:
        admin = psycopg2.connect(_admin_url())
    except Exception as e:
        pytest.skip(f"PostgreSQL not reachable: {e}")
    admin.autocommit = True
    admin.cursor().execute(f"CREATE DATABASE {name}")
    url = _admin_url().rsplit("/", 1)[0] + "/" + name
    c = psycopg2.connect(url)
    cur = c.cursor()
    cur.execute("""CREATE TYPE data_quality_status AS ENUM ('VALID','SUSPECT','INVALID','DUPLICATE');
        CREATE TABLE symbols (id SERIAL PRIMARY KEY, symbol VARCHAR(30) UNIQUE);
        CREATE TABLE ohlcv_daily (symbol_id int REFERENCES symbols(id), timestamp timestamptz NOT NULL, open numeric(18,4), high numeric(18,4),
            low numeric(18,4), close numeric(18,4), volume bigint DEFAULT 0, adjusted_close numeric(18,4), source varchar(50) DEFAULT 't',
            retrieved_at timestamptz DEFAULT now(), quality_status data_quality_status DEFAULT 'VALID', PRIMARY KEY (symbol_id, timestamp));
        INSERT INTO symbols(symbol) VALUES ('TEST');""")
    for f in ("02-paper-observations.sql", "05-p17-paper-observation-integrity.sql", "06-p17-observation-dedupe-and-derived-bar-flag.sql"):
        cur.execute(open(os.path.join(MIG, f)).read())
    c.commit()
    yield url, c
    c.close()
    admin.cursor().execute(f"DROP DATABASE {name} WITH (FORCE)")
    admin.close()


def _fill_bars(conn, spec, start_idx=0, other_symbols_bars=True):
    cur = conn.cursor()
    for i, (o, h, l, c) in enumerate(spec, start=start_idx):
        d = SESS[i]
        ts = datetime(d.year, d.month, d.day, 3, 45, tzinfo=UTC)
        cur.execute("INSERT INTO ohlcv_daily (symbol_id,timestamp,open,high,low,close,volume) VALUES (1,%s,%s,%s,%s,%s,1000) ON CONFLICT DO NOTHING", (ts, o, h, l, c))
    conn.commit()


def _cand(conn, entry=100.0, ts=None, setup="CONFIRMED_BREAKOUT"):
    from src.engines.research.paper_observation_store import insert_candidate
    r = rec(entry=entry, stop=entry * .95, target=entry * 1.1, setup=setup)
    if ts:
        r = replace(r, candidate_timestamp=ts, signal_timestamp=ts)
    oid, created = insert_candidate(conn, 1, r)
    conn.commit()
    return oid, created


def test_db_duplicate_candidate_is_idempotent_and_does_not_touch_row(scratch):
    url, conn = scratch
    oid, created = _cand(conn)
    cur = conn.cursor()
    cur.execute("SELECT updated_at, decision_snapshot_sha256 FROM paper_observations WHERE id=%s", (oid,))
    before = cur.fetchone()
    oid2, created2 = _cand(conn)
    cur.execute("SELECT updated_at, decision_snapshot_sha256 FROM paper_observations WHERE id=%s", (oid,))
    assert created and not created2 and oid == oid2 and cur.fetchone() == before
    cur.execute("SELECT count(*) FROM paper_observations WHERE symbol_id=1 AND candidate_timestamp=%s", (DECISION,))
    assert cur.fetchone()[0] == 1


def test_db_same_session_resubmission_is_deduplicated(scratch):
    url, conn = scratch
    base = datetime(2026, 5, 6, 9, 0, tzinfo=UTC)
    oid1, c1 = _cand(conn, entry=222.0, ts=base)
    oid2, c2 = _cand(conn, entry=222.0, ts=base + timedelta(hours=3))     # same IST session, different timestamp
    assert c1 and not c2 and oid1 == oid2


def test_db_triggers_block_snapshot_mutation_deletes_and_terminal_edits(scratch):
    url, conn = scratch
    oid, _ = _cand(conn, entry=101.0, ts=DECISION + timedelta(minutes=1))
    cur = conn.cursor()
    for sql in ("UPDATE paper_observations SET entry_trigger_price=1 WHERE id=%s",
                "UPDATE paper_observations SET candidate_timestamp=now() WHERE id=%s",
                "UPDATE paper_observations SET decision_snapshot='{}'::jsonb WHERE id=%s",
                "DELETE FROM paper_observations WHERE id=%s"):
        with pytest.raises(psycopg2.errors.RaiseException):
            cur.execute(sql, (oid,))
        conn.rollback()
    cur.execute("UPDATE paper_observations SET status='COMPLETED', outcome='NOT_TRIGGERED', is_complete=true, completed_at=now() WHERE id=%s", (oid,))
    conn.commit()
    with pytest.raises(psycopg2.errors.RaiseException):
        cur.execute("UPDATE paper_observations SET outcome='TARGET_HIT' WHERE id=%s", (oid,))
    conn.rollback()


def test_db_constraints_reject_bad_rows(scratch):
    url, conn = scratch
    cur = conn.cursor()
    for frag in ("shariah_status='FAIL'", "data_cutoff_at=n + interval '1 day'", "invalidation_price=100", "status='BOGUS'"):
        with pytest.raises(psycopg2.errors.CheckViolation):
            cur.execute(_bad_insert(frag))
        conn.rollback()


def _bad_insert(frag):
    cols = {"shariah_status": "'PASS'", "data_cutoff_at": "n", "invalidation_price": "95", "status": "'PENDING'"}
    key, val = frag.split("=", 1)
    vals = dict(cols)
    vals[key] = val
    return f"""WITH s AS (SELECT now() n) INSERT INTO paper_observations (symbol_id,symbol,setup_type,candidate_timestamp,signal_timestamp,data_cutoff_at,provider,rule_version,
        entry_trigger_price,invalidation_price,target_price,risk_reward_ratio,shariah_status,decision_snapshot,decision_snapshot_sha256,status)
        SELECT 1,'TEST','X',n,n,{vals['data_cutoff_at']},'p','v'||random(),100,{vals['invalidation_price']},110,2,{vals['shariah_status']},'{{}}','{'0'*64}',{vals['status']} FROM s"""


def test_db_partial_horizons_then_completion_then_rerun_is_noop(scratch):
    from src.engines.research.paper_observation_store import evaluate_pending
    url, conn = scratch
    cur = conn.cursor()
    cur.execute("DELETE FROM ohlcv_daily")
    conn.commit()
    ts = DECISION + timedelta(days=0, minutes=7)
    oid, _ = _cand(conn, entry=100.0, ts=ts, setup='CONSOLIDATION')
    # ensure the "daily table complete" gate has >=1 symbol per session: only symbol 1 exists so every loaded session counts
    _fill_bars(conn, [(99, 101, 98, 100), (100, 102, 99, 101), (101, 103, 99, 102)])
    s1 = evaluate_pending(conn)
    cur.execute("SELECT status, evaluated_through_session, predetermined_horizons FROM paper_observations WHERE id=%s", (oid,))
    st, through, hz = cur.fetchone()
    assert st == "PENDING" and through == SESS[2] and hz["1d"]["status"] == "OBSERVED" and hz["5d"]["status"] == "PENDING"
    cur.execute("SELECT updated_at FROM paper_observations WHERE id=%s", (oid,)); u1 = cur.fetchone()[0]
    s2 = evaluate_pending(conn)                       # nothing new -> no write
    cur.execute("SELECT updated_at FROM paper_observations WHERE id=%s", (oid,)); assert cur.fetchone()[0] == u1
    assert s2["updated"] == 0
    _fill_bars(conn, [(102, 112, 101, 111)], start_idx=3)
    s3 = evaluate_pending(conn)
    cur.execute("SELECT status, outcome, exit_price, realized_net_return, itemized_costs->>'model_version' FROM paper_observations WHERE id=%s", (oid,))
    row = cur.fetchone()
    assert row[0] == "COMPLETED" and row[1] == "TARGET_HIT" and float(row[2]) == 110.0 and row[3] is not None and row[4] == "COST_MODEL_2026_Q4_V2"
    cur.execute("SELECT updated_at, realized_net_return FROM paper_observations WHERE id=%s", (oid,)); snap = cur.fetchone()
    s4 = evaluate_pending(conn)
    cur.execute("SELECT updated_at, realized_net_return FROM paper_observations WHERE id=%s", (oid,))
    assert cur.fetchone() == snap and s4["updated"] == 0


def test_db_concurrent_workers_process_a_row_once(scratch):
    from src.engines.research.paper_observation_store import evaluate_pending
    url, conn = scratch
    cur = conn.cursor()
    ts = DECISION + timedelta(minutes=30)
    oid, _ = _cand(conn, entry=100.0, ts=ts, setup='APPROACHING_RESISTANCE')
    results = []

    def worker():
        c = psycopg2.connect(url)
        try:
            results.append(evaluate_pending(c))
        finally:
            c.close()
    threads = [threading.Thread(target=worker) for _ in range(4)]
    [t.start() for t in threads]; [t.join() for t in threads]
    assert sum(r["updated"] for r in results) <= 1          # at most one worker wrote the (already-available) evaluation
    cur.execute("SELECT status FROM paper_observations WHERE id=%s", (oid,))
    assert cur.fetchone()[0] in ("COMPLETED", "PENDING")
