"""P1.9.29: upcoming-breakout tracking, reasons, per-stock tests, and migration 46 on a scratch database."""
import os
import uuid
from datetime import date, timedelta

import numpy as np
import pandas as pd
import psycopg2
import pytest

from src.engines.research import breakout_evidence as be
from src.engines.research import breakout_watch as bw
from src.engines.research.breakout_watch import CANDIDATE, CONFIRMED, SymbolSeries


def series(n=60, stages=None, closes=None, highs=None, lows=None, stop=95.0, sym="TEST", sid=1):
    dates = np.array([str(date(2026, 1, 1) + timedelta(days=i)) for i in range(n)])
    c = np.array(closes if closes is not None else [100.0] * n)
    h = np.array(highs if highs is not None else c + 1)
    l = np.array(lows if lows is not None else c - 1)
    st = [None] * n
    detail = {}
    for t, s in (stages or {}).items():
        st[t] = s
        if s in (CANDIDATE, CONFIRMED):
            detail[t] = {"stage": s, "stop": stop, "t1": 110.0, "t2": 120.0, "entry_low": 100.0, "entry_high": 101.0, "resistance": 100.0, "atr": 2.0,
                         "rvol": 2.0, "score": 70.0, "breakdown": {"price_structure": 20.0}, "reasons": [f"reason on {t}"], "invalidation": []}
    return SymbolSeries(sym, sid, dates, c.copy(), h, l, c, np.full(n, 1000), st, detail)


# ---------------------------------------------------------------------------------------------- outcomes
def test_candidate_that_breaks_out_is_a_hit_with_the_session_count():
    s = series(stages={45: CANDIDATE, 48: CONFIRMED})
    o = bw.candidate_outcome(s, 45)
    assert o["status"] == "BROKE_OUT" and o["sessions_to_breakout"] == 3 and o["broke_out_session"] == s.dates[48]


def test_close_below_stop_before_any_breakout_invalidates():
    c = [100.0] * 60
    c[47] = 94.0
    s = series(stages={45: CANDIDATE, 50: CONFIRMED}, closes=c)          # the breakout comes later than the stop: too late
    assert bw.candidate_outcome(s, 45)["status"] == "INVALIDATED"


def test_ten_quiet_sessions_expire_and_moves_are_measured_from_the_flag_close():
    c = [100.0] * 60
    c[50], c[55] = 103.0, 98.0
    s = series(stages={45: CANDIDATE}, closes=c, highs=[x + 1 for x in c], lows=[x - 1 for x in c])
    o = bw.candidate_outcome(s, 45)
    assert o["status"] == "EXPIRED" and o["sessions_elapsed"] == 10
    assert o["ret_5d_pct"] == pytest.approx((c[50] / 100 - 1) * 100) and o["ret_10d_pct"] == pytest.approx((c[55] / 100 - 1) * 100)
    assert o["max_gain_pct"] == pytest.approx(4.0) and o["max_drawdown_pct"] == pytest.approx(-3.0)


def test_unfinished_flag_stays_watching_and_never_peeks_past_the_data():
    s = series(n=48, stages={45: CANDIDATE})
    o = bw.candidate_outcome(s, 45)
    assert o["status"] == "WATCHING" and o["sessions_elapsed"] == 2 and o["ret_5d_pct"] is None and o["ret_10d_pct"] is None


def test_repeat_flags_within_the_horizon_are_not_counted_twice():
    s = series(stages={40: CANDIDATE, 42: CANDIDATE, 44: CANDIDATE, 53: CANDIDATE})
    rows = bw.candidates({"TEST": s})
    assert [r["counts"] for r in rows] == [True, False, False, True]


# ---------------------------------------------------------------------------------------------- one engine pass == the old collector
def synthetic_frame(seed=3, n=520, symbols=4):
    rng = np.random.default_rng(seed)
    rows = []
    for k in range(symbols):
        px, base = 100.0 + k * 10, pd.Timestamp("2023-01-02")
        for i in range(n):
            drift = 0.0006 + (0.03 if i % 90 == 89 else 0.0)                    # periodic jumps create breakouts
            r = rng.normal(drift, 0.01)
            o = px
            px = px * (1 + r)
            vol = int(rng.integers(8e5, 1.2e6) * (3 if i % 90 == 89 else 1))
            rows.append({"symbol": f"S{k}", "symbol_id": k + 1, "timestamp": base + pd.Timedelta(days=i), "open": o, "high": max(o, px) * 1.004,
                         "low": min(o, px) * 0.996, "close": px, "volume": vol})
    return pd.DataFrame(rows)


def test_single_pass_reproduces_collect_signals_exactly():
    frame = synthetic_frame()
    analysis = bw.analyze_universe(frame)
    mine = bw.signals_from(analysis)
    ref = be.collect_signals(frame)
    assert sum(len(v["signals"]) for v in ref.values()) > 0, "fixture must contain confirmed breakouts"
    for sym in ref:
        assert mine[sym]["signals"] == ref[sym]["signals"], sym


# ---------------------------------------------------------------------------------------------- per-stock tests
def test_stock_stats_are_honest_about_small_samples():
    frame = synthetic_frame()
    analysis = bw.analyze_universe(frame)
    sig = bw.signals_from(analysis)
    st = bw.stock_stats(analysis, sig, pd.Series(dtype=float))
    for sym, r in st.items():
        n = r["confirmed_breakouts"]["trades"]
        if n < bw.MIN_TRADES_FOR_VERDICT:
            assert r["verdict"] == "INSUFFICIENT_DATA" and r["confirmed_breakouts"]["net_ci95"] is None
        assert r["upcoming_flags"]["hit_rate_pct"] is None or 0 <= r["upcoming_flags"]["hit_rate_pct"] <= 100
    assert bw.universe_rates(analysis)["note"]


# ---------------------------------------------------------------------------------------------- PostgreSQL (scratch DB, real migrations)
MIG = os.getenv("P17_MIGRATIONS_DIR", os.path.join(os.path.dirname(__file__), "..", "..", "backend", "migrations"))


@pytest.fixture()
def scratch():
    url = os.getenv("ADMIN_DATABASE_URL") or os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    try:
        admin = psycopg2.connect(url)
        admin.autocommit = True
        name = "p1929_test_" + uuid.uuid4().hex[:8]
        admin.cursor().execute(f"CREATE DATABASE {name}")
    except Exception as e:
        pytest.skip(f"cannot create a scratch database ({type(e).__name__})")
    conn = psycopg2.connect(url.rsplit("/", 1)[0].split("?")[0] + "/" + name)
    cur = conn.cursor()
    cur.execute("SELECT 1 FROM pg_roles WHERE rolname = 'halal_app_user'")
    if not cur.fetchone():
        cur.execute("CREATE ROLE halal_app_user")
    cur.execute("CREATE TABLE symbols (id serial PRIMARY KEY, symbol text)")
    cur.execute("INSERT INTO symbols (symbol) VALUES ('TEST'), ('OTHER')")
    cur.execute("CREATE TABLE portfolios (id serial PRIMARY KEY, user_id int, cash_balance numeric DEFAULT 1000000, initial_capital numeric DEFAULT 1000000, owner_id uuid)")
    cur.execute(open(os.path.join(MIG, "44-p1927-breakout-tracking.sql")).read())
    conn.commit()
    yield conn
    conn.close()
    admin.cursor().execute(f"DROP DATABASE {name} WITH (FORCE)")
    admin.close()


def add_signal(conn, sid, day, resolved):
    cur = conn.cursor()
    cur.execute("""INSERT INTO breakout_tracking (symbol_id, signal_date, signal_close, entry_low, entry_high, stop_level, target_1, target_2, resolved_at)
                   VALUES (%s,%s,100,99,101,95,110,120, %s)""", (sid, day, "2026-11-01" if resolved else None))
    conn.commit()


def migrate46(conn):
    conn.cursor().execute(open(os.path.join(MIG, "46-p1929-breakout-watch-news-autopaper.sql")).read())
    conn.commit()


def one(conn, sql, args=None):
    cur = conn.cursor()
    cur.execute(sql, args)
    r = cur.fetchone()
    conn.rollback()
    return r[0] if r else None


def test_migration_marks_repeat_signals_but_keeps_them(scratch):
    d = date(2026, 11, 2)
    for k in (0, 1, 2):                        # SBIN-style: three consecutive days
        add_signal(scratch, 1, d + timedelta(days=k), resolved=(k == 0))
    add_signal(scratch, 1, d + timedelta(days=20), resolved=True)     # a genuinely later, separate signal
    add_signal(scratch, 2, d, resolved=True)                         # another stock
    migrate46(scratch)
    rows = scratch.cursor()
    rows.execute("SELECT symbol_id, signal_date, counts_in_stats FROM breakout_tracking ORDER BY symbol_id, signal_date")
    got = [(a, b.isoformat(), c) for a, b, c in rows.fetchall()]
    assert got == [(1, "2026-11-02", True), (1, "2026-11-03", False), (1, "2026-11-04", False), (1, "2026-11-22", True), (2, "2026-11-02", True)]
    assert one(scratch, "SELECT count(*) FROM breakout_tracking") == 5                      # nothing deleted
    assert "REPEAT_SIGNAL" in one(scratch, "SELECT excluded_reason FROM breakout_tracking WHERE signal_date = '2026-11-03'")
    migrate46(scratch)                                                                       # idempotent


def test_watch_flags_are_frozen_but_outcomes_can_resolve_once(scratch):
    migrate46(scratch)
    s = series(stages={45: CANDIDATE, 48: CONFIRMED}, sym="TEST", sid=1)
    n = bw.store_history(scratch, {"TEST": s}, sessions=200)
    assert n == 1 and bw.store_history(scratch, {"TEST": s}, sessions=200) == 0           # never duplicated
    assert one(scratch, "SELECT status FROM breakout_watch") == "BROKE_OUT"
    assert one(scratch, "SELECT reasons->>0 FROM breakout_watch") == "reason on 45"
    cur = scratch.cursor()
    for sql in ("UPDATE breakout_watch SET resistance_level = 1", "UPDATE breakout_watch SET reasons = '[]'::jsonb", "UPDATE breakout_watch SET status = 'EXPIRED'"):
        with pytest.raises(psycopg2.errors.RaiseException):
            cur.execute(sql)
        scratch.rollback()


def test_resolve_finishes_watching_flags_from_the_series_and_is_idempotent(scratch):
    migrate46(scratch)
    short = series(n=48, stages={45: CANDIDATE}, sym="TEST", sid=1)               # only 2 sessions of data so far
    bw.store_history(scratch, {"TEST": short}, sessions=200)
    assert one(scratch, "SELECT status FROM breakout_watch") == "WATCHING"
    longer = series(n=60, stages={45: CANDIDATE, 49: CONFIRMED}, sym="TEST", sid=1)
    r = bw.resolve(scratch, {"TEST": longer})
    assert r["resolved"] == 1 and one(scratch, "SELECT sessions_to_breakout FROM breakout_watch") == 4
    r2 = bw.resolve(scratch, {"TEST": longer})
    assert r2["checked"] == 0                                                          # nothing left to resolve


def test_reasons_backfill_only_fills_rows_without_reasons(scratch):
    migrate46(scratch)
    s = series(stages={45: CONFIRMED}, sym="TEST", sid=1)
    add_signal(scratch, 1, date.fromisoformat(str(s.dates[45])), resolved=True)
    assert bw.backfill_reasons(scratch, {"TEST": s}) == 1
    assert one(scratch, "SELECT reasons->>0 FROM breakout_tracking") == "reason on 45"
    assert bw.backfill_reasons(scratch, {"TEST": s}) == 0
    assert one(scratch, "SELECT resistance_level FROM breakout_tracking") == 100


def test_stock_stats_storage_is_append_only_history_with_a_latest_view(scratch):
    migrate46(scratch)
    bw.store_stock_stats(scratch, {"TEST": {"verdict": "A"}}, "2026-11-01")
    bw.store_stock_stats(scratch, {"TEST": {"verdict": "B"}}, "2026-11-02")
    assert one(scratch, "SELECT count(*) FROM stock_signal_stats") == 2
    assert one(scratch, "SELECT result->>'verdict' FROM stock_signal_stats_latest WHERE symbol_id = 1") == "B"
