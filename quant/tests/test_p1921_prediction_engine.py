"""P1.9.21 intraday prediction engine: the guarantees that make its evidence trustworthy."""
import os
from datetime import date, datetime, timedelta, timezone

import numpy as np
import pandas as pd
import psycopg2
import pytest

from src.engines.prediction import model as M
from src.engines.prediction import walkforward as W
from src.engines.prediction.config import FEATURES, MIN_TRAIN_DAYS
from src.engines.prediction.costs import round_trip_cost_fraction
from src.engines.prediction.data import IST, to_session_bars
from src.engines.prediction.features import add_labels, build_features
from src.engines.prediction.service import decide_action


def synthetic_minutes(days=16, symbols=4, seed=1):
    rng = np.random.default_rng(seed)
    rows = []
    d0 = date(2026, 9, 1)
    for di in range(days):
        d = d0 + timedelta(days=di)
        for s in range(symbols):
            price = 100.0 + s
            t = datetime(d.year, d.month, d.day, 9, 15, tzinfo=IST)
            for _ in range(375):
                price *= float(np.exp(rng.normal(0, 0.001)))
                rows.append((s + 1, f"S{s}", t.astimezone(timezone.utc), price, price * 1.001, price * 0.999, price, 1000.0))
                t += timedelta(minutes=1)
    return pd.DataFrame(rows, columns=["symbol_id", "symbol", "ts", "open", "high", "low", "close", "volume"]).assign(
        ts=lambda x: pd.to_datetime(x["ts"], utc=True))


@pytest.fixture(scope="module")
def bars():
    return to_session_bars(synthetic_minutes())


def test_bars_stay_inside_the_session(bars):
    t = bars["bar_start"].dt.time
    assert t.min().strftime("%H:%M") == "09:15"
    assert t.max().strftime("%H:%M") == "15:25"
    assert (bars["n_ticks"] >= 3).all()


def test_features_have_no_lookahead(bars):
    """Changing the future must not change any feature at time t."""
    full = build_features(bars)
    cut_time = bars["bar_start"].sort_values().unique()[len(bars["bar_start"].unique()) // 2]
    altered = bars.copy()
    future = altered["bar_start"] > cut_time
    altered.loc[future, ["open", "high", "low", "close"]] *= 1.5
    altered.loc[future, "volume"] *= 10
    alt = build_features(altered)
    past = full["bar_start"] <= cut_time
    pd.testing.assert_frame_equal(full.loc[past, FEATURES].reset_index(drop=True),
                                  alt.loc[past, FEATURES].reset_index(drop=True))


def test_labels_never_cross_the_session_close(bars):
    lab = add_labels(build_features(bars), h_bars=12, cost=0.002)
    last_hour = lab["bar_start"].dt.time >= pd.Timestamp("14:30").time()
    assert lab.loc[last_hour, "fwd"].isna().all(), "a 60-minute label from the last hour would cross the close"
    assert (lab.loc[last_hour, "label"] == -1).all()
    assert lab.loc[~last_hour, "fwd"].notna().mean() > 0.95


def test_labels_are_cost_aware():
    df = pd.DataFrame({"symbol_id": 1, "date": date(2026, 9, 1),
                       "bar_start": pd.date_range("2026-09-01 09:15", periods=4, freq="5min", tz=IST),
                       "close": [100.0, 100.1, 99.7, 100.5]})
    lab = add_labels(df, 1, cost=0.002)
    # +0.1% < cost -> NEUTRAL; -0.4% -> DOWN; +0.8% -> UP; last bar has no future -> -1
    assert lab["label"].tolist() == [1, 0, 2, -1]


def test_walk_forward_never_trains_on_or_after_the_test_day(bars, monkeypatch):
    lab = add_labels(build_features(bars), 3, cost=0.0005)
    seen = []
    real_fit = M.fit

    def spy(X, y):
        seen.append(len(X))
        return real_fit(X, y)

    monkeypatch.setattr(M, "fit", spy)
    days = sorted(lab["date"].unique())
    res = W.run(lab, cost=0.0005)
    assert res.verdict in ("ACTIONABLE", "NO_DEMONSTRATED_EDGE")
    labelled = lab[lab["label"] >= 0]
    for i, n in enumerate(seen):
        test_day = days[MIN_TRAIN_DAYS + i]
        assert n == (labelled["date"] < test_day).sum(), "training set must be exactly the days before the test day"
    assert set(res.oof["date"]) == set(days[MIN_TRAIN_DAYS:])


def test_random_walk_has_no_demonstrated_edge(bars):
    """On a pure random walk the engine must not claim an edge (guards against leakage / overfitting)."""
    res = W.run(add_labels(build_features(bars), 3, cost=round_trip_cost_fraction()), cost=round_trip_cost_fraction())
    assert res.verdict == "NO_DEMONSTRATED_EDGE"


def test_too_little_history_is_reported_not_guessed(bars):
    few = bars[bars["date"] <= sorted(bars["date"].unique())[5]]
    res = W.run(add_labels(build_features(few), 1, cost=0.002), cost=0.002)
    assert res.verdict == "DATA_INSUFFICIENT"
    assert "trading days" in res.metrics["reason"]


def test_intraday_costs_come_from_the_cost_engine():
    c = round_trip_cost_fraction()
    assert 0.0015 < c < 0.0025  # ~0.18%: STT sell-only 0.025%, brokerage, GST, stamp, 5 bps slippage per side


P_BUY = np.array([0.1, 0.3, 0.6])
P_SELL = np.array([0.6, 0.3, 0.1])


@pytest.mark.parametrize("verdict,latency,shariah,expected,codes", [
    ("ACTIONABLE", 30, "PASS", "BUY_CANDIDATE", []),
    ("ACTIONABLE", 30, "DATA_INSUFFICIENT", "BUY_CANDIDATE", ["SHARIAH_UNEVALUATED"]),
    ("ACTIONABLE", 30, "FAIL", "NO_TRADE", ["SHARIAH_FAIL"]),
    ("NO_DEMONSTRATED_EDGE", 30, "PASS", "NO_TRADE", ["NO_DEMONSTRATED_EDGE"]),
    ("ACTIONABLE", 30 * 60, "PASS", "NO_TRADE", ["DATA_TOO_STALE"]),
])
def test_action_gating(verdict, latency, shariah, expected, codes):
    action, reasons = decide_action(P_BUY, 0.01, 0.002, verdict, latency, 60, shariah)
    assert action == expected
    got = {r["code"] for r in reasons}
    assert set(codes) <= got
    assert any(r["code"] == "MODEL_VIEW" and r["detail"] == "BUY" for r in reasons), "model view is always recorded"


def test_exit_signal_and_no_edge():
    assert decide_action(P_SELL, -0.01, 0.002, "ACTIONABLE", 30, 60, "PASS")[0] == "EXIT_OR_AVOID"
    assert decide_action(P_BUY, 0.001, 0.002, "ACTIONABLE", 30, 60, "PASS")[0] == "NO_TRADE"  # move < cost


def test_conditional_returns_lookup():
    score = np.linspace(-1, 1, 1000)
    fwd = score * 0.01
    cr = M.ConditionalReturns.from_oof(score, fwd)
    lo, hi = cr.lookup(-0.95), cr.lookup(0.95)
    assert lo["q50"] < 0 < hi["q50"]
    assert all(q["q10"] <= q["q50"] <= q["q90"] for q in cr.quantiles)


@pytest.mark.skipif(not os.environ.get("DATABASE_URL"), reason="needs the application database")
def test_issued_forecasts_are_immutable_in_the_database():
    conn = psycopg2.connect(os.environ["DATABASE_URL"])
    try:
        cur = conn.cursor()
        cur.execute("""INSERT INTO prediction_model_runs (model_version, horizon_minutes, verdict, metrics, params, cost_model)
                       VALUES ('TEST', 60, 'DATA_INSUFFICIENT', '{}', '{}', '{}') RETURNING id""")
        run = cur.fetchone()[0]
        cur.execute("SELECT id FROM canonical.symbols WHERE symbol = 'TCS'")
        sid = cur.fetchone()[0]
        cur.execute("""INSERT INTO predictions (model_run_id, symbol_id, horizon_minutes, as_of, target_time, reference_price,
                         p_down, p_neutral, p_up, expected_return, return_q10, return_q50, return_q90, action, shariah_status)
                       VALUES (%s,%s,60, now(), now() + interval '60 minutes', 100, 0.2, 0.5, 0.3, 0.001, -0.01, 0, 0.01,
                               'NO_TRADE', 'UNEVALUATED') RETURNING id""", (run, sid))
        pid = cur.fetchone()[0]
        cur.execute("SAVEPOINT s")
        with pytest.raises(psycopg2.Error, match="immutable"):
            cur.execute("UPDATE predictions SET p_up = 0.9, p_neutral = 0.0, p_down = 0.1 WHERE id = %s", (pid,))
        cur.execute("ROLLBACK TO SAVEPOINT s")
        # Two independent layers: the runtime role has no DELETE grant, and the trigger forbids it anyway.
        with pytest.raises(psycopg2.Error, match="append-only|permission denied"):
            cur.execute("DELETE FROM predictions WHERE id = %s", (pid,))
        cur.execute("ROLLBACK TO SAVEPOINT s")
        cur.execute("UPDATE predictions SET realized_price = 101, realized_return = 0.01, realized_class = 'UP', resolved_at = now() WHERE id = %s", (pid,))
        with pytest.raises(psycopg2.Error, match="already resolved"):
            cur.execute("UPDATE predictions SET realized_price = 99 WHERE id = %s", (pid,))
    finally:
        conn.rollback()  # nothing persists
        conn.close()
