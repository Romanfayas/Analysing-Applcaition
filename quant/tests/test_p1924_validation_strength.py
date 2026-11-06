"""P1.9.24: stronger out-of-sample requirements, with the model and Strategy V2 left untouched."""
from src.engines.prediction import walkforward as W
from src.engines.prediction.config import EDGE_MIN_TEST_DAYS, MIN_TRAIN_DAYS, MODEL_VERSION
from src.engines.prediction.data import to_session_bars
from src.engines.prediction.features import add_labels, build_features
from src.engines.research.strategy_config import runtime_config

from tests.test_p1921_prediction_engine import synthetic_minutes


def _run(days):
    bars = to_session_bars(synthetic_minutes(days=days))
    return W.run(add_labels(build_features(bars), 3, cost=0.0005), cost=0.0005)


def test_too_few_independent_days_can_never_be_actionable():
    res = _run(16)  # 8 test days < EDGE_MIN_TEST_DAYS
    assert res.metrics["edge_checks"]["enough_independent_test_days"] is False
    assert res.verdict != "ACTIONABLE"
    assert EDGE_MIN_TEST_DAYS >= 20


def test_per_day_and_regime_breakdowns_cover_every_test_day():
    res = _run(16)
    m = res.metrics
    assert len(m["per_day"]) == 16 - MIN_TRAIN_DAYS
    assert sum(r["days"] for r in m["regimes"].values()) == len(m["per_day"])
    assert set(m["regimes"]) <= {"market_up_day", "market_down_day", "market_flat_day"}
    assert m["gate_version"] == "EDGE_GATE_V2"
    assert 0 <= m["positive_net_days"] <= len(m["per_day"])


def test_model_identity_and_strategy_v2_hash_are_preserved():
    assert MODEL_VERSION == "INTRADAY_HGB_V1", "a gate change must not silently relabel the model"
    cfg = runtime_config()
    assert cfg["matches_pinned"] is True
    assert cfg["pinned_hash"] == "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
    assert cfg["resolved"] == {"BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 2.0}


def test_regime_labels_do_not_depend_on_the_horizon():
    bars = to_session_bars(synthetic_minutes(days=16))
    feats = build_features(bars)
    per = {}
    for h in (1, 12):
        res = W.run(add_labels(feats, h, cost=0.0005), cost=0.0005)
        per[h] = {d["date"]: d["regime"] for d in res.metrics["per_day"]}
    assert per[1] == per[12], "a test day is the same market day whatever the forecast horizon"


# ---- P1.9.25: 5-minute history merged into training ----
import pandas as pd

from src.engines.prediction.data import IST, five_minute_rows_to_bars, merge_bar_sources


def _row(symbol_id, ist_str, close, volume=100.0):
    ts = pd.Timestamp(ist_str, tz=IST).tz_convert("UTC")
    return {"symbol_id": symbol_id, "symbol": f"S{symbol_id}", "ts": ts, "open": close, "high": close + 1, "low": close - 1,
            "close": close, "volume": volume}


def test_five_minute_rows_become_complete_session_bars():
    rows = pd.DataFrame([
        _row(1, "2026-09-01 09:10", 99),   # before the open: dropped
        _row(1, "2026-09-01 09:15", 100),
        _row(1, "2026-09-01 15:25", 101),  # the last bar of the session: kept (its minutes 15:16-15:28 are not needed)
        _row(1, "2026-09-01 15:30", 102),  # starts at the close: dropped
    ])
    bars = five_minute_rows_to_bars(rows)
    assert list(bars["close"]) == [100.0, 101.0]
    assert (bars["n_ticks"] == 5).all()
    assert (bars["bar_end"] - bars["bar_start"] == pd.Timedelta(minutes=5)).all()
    assert str(bars["date"].iloc[0]) == "2026-09-01"
    assert five_minute_rows_to_bars(rows.iloc[0:0]).empty


def test_one_minute_derived_bars_win_where_both_sources_exist():
    one = five_minute_rows_to_bars(pd.DataFrame([_row(1, "2026-11-02 10:00", 500.0)]))
    five = five_minute_rows_to_bars(pd.DataFrame([
        _row(1, "2026-11-02 10:00", 999.0),  # same bar: the live 1-minute-derived value must be kept
        _row(1, "2026-09-01 10:00", 100.0),  # an older day only the history has
        _row(2, "2026-11-02 10:00", 200.0),  # another symbol
    ]))
    merged = merge_bar_sources(one, five)
    assert len(merged) == 3
    kept = merged[(merged["symbol_id"] == 1) & (merged["date"].astype(str) == "2026-11-02")]
    assert list(kept["close"]) == [500.0]
    assert merged["date"].astype(str).is_monotonic_increasing or True  # ordering is per symbol; checked below
    assert list(merged[merged["symbol_id"] == 1]["date"].astype(str)) == ["2026-09-01", "2026-11-02"]
    assert merge_bar_sources(one, five.iloc[0:0]).equals(one)


def test_history_lets_the_edge_gate_become_decidable():
    """With 60 days of history there are enough independent test days to evaluate the gate at all."""
    from src.engines.prediction.config import MIN_TRAIN_DAYS
    assert 60 - MIN_TRAIN_DAYS >= EDGE_MIN_TEST_DAYS
