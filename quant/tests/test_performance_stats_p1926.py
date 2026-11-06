"""P1.9.26: performance statistics. Pure-function tests, no database."""
import math

import numpy as np
import pytest

from src.engines.research.performance import (
    ClosedTrade, bca_interval, drawdown_distribution, max_drawdown_pct, performance_report, profit_factor,
    sharpe_ratio, ulcer_index,
)


def test_profit_factor_cases():
    assert profit_factor([10, -5, 5]) == pytest.approx(3.0)
    assert math.isinf(profit_factor([1, 2]))
    assert math.isnan(profit_factor([0, 0]))


def test_max_drawdown_and_ulcer_index():
    # 100 -> 110 -> 99 -> 120: worst fall is 11 from the 110 peak = 10%
    assert max_drawdown_pct([10, -11, 21], 100) == pytest.approx(10.0)
    assert max_drawdown_pct([5, 5, 5], 100) == 0.0
    assert ulcer_index([5, 5, 5], 100) == 0.0
    assert ulcer_index([10, -11, 21], 100) > 0


def test_bca_interval_brackets_the_estimate_and_narrows_with_more_data():
    rng = np.random.default_rng(1)
    small = rng.normal(1.0, 3.0, 20)
    big = rng.normal(1.0, 3.0, 400)
    a, b = bca_interval(small, np.mean), bca_interval(big, np.mean)
    assert a["low"] < a["estimate"] < a["high"]
    assert (b["high"] - b["low"]) < (a["high"] - a["low"])


def test_bca_is_reproducible_and_refuses_tiny_samples():
    x = [1.0, -2.0, 3.0, 0.5, 2.0, -1.0, 4.0]
    assert bca_interval(x, np.mean) == bca_interval(x, np.mean)
    assert bca_interval([1, 2, 3], np.mean) is None
    assert bca_interval([1, 1, 1, 1, 1, 1], profit_factor) is None  # all wins -> infinite factor, no interval claimed


def test_a_random_walk_record_does_not_show_skill():
    rng = np.random.default_rng(7)
    pnl = rng.normal(0, 1000, 60)
    trades = [ClosedTrade(pnl=float(p), invested=50_000, fees=40) for p in pnl]
    rep = performance_report(trades, 1_000_000)
    assert rep["reliability"] == "ADEQUATE"
    e = rep["expectancy_pct"]
    assert e["low"] < 0 < e["high"], "noise must not be reported as an edge"
    assert "not distinguishable from zero" in rep["verdict"]


def test_a_clear_edge_is_detected_and_a_clear_loss_too():
    good = performance_report([ClosedTrade(pnl=1000 + (i % 5) * 50, invested=50_000, fees=40) for i in range(40)], 1_000_000)
    assert good["expectancy_pct"]["low"] > 0 and "positive with 95% confidence" in good["verdict"]
    bad = performance_report([ClosedTrade(pnl=-500 - (i % 5) * 50, invested=50_000, fees=40) for i in range(40)], 1_000_000)
    assert bad["expectancy_pct"]["high"] < 0 and "negative with 95% confidence" in bad["verdict"]


def test_too_few_trades_are_called_insufficient_not_skilled():
    rep = performance_report([ClosedTrade(pnl=500, invested=10_000, fees=30) for _ in range(3)], 100_000)
    assert rep["reliability"] == "INSUFFICIENT"
    assert rep["expectancy_pct"] is None and rep["max_drawdown"] is None
    assert "too few" in rep["verdict"]
    assert rep["profit_factor"] is None and rep["profit_factor_note"]


def test_no_trades_and_benchmark_comparison_and_cost_drag():
    assert performance_report([], 100_000)["n_trades"] == 0
    trades = [ClosedTrade(pnl=800, invested=20_000, fees=80, benchmark_return_pct=1.0) for _ in range(12)]
    rep = performance_report(trades, 200_000)
    assert rep["cost_drag_pct_of_gross_profit"] == pytest.approx(80 * 12 / (800 * 12) * 100)
    assert rep["excess_vs_benchmark_pct"]["estimate"] == pytest.approx(4.0 - 1.0)
    assert rep["benchmark_beaten_pct_of_trades"] == 100.0
    partial = performance_report([ClosedTrade(pnl=1, invested=100, benchmark_return_pct=None) for _ in range(12)], 1000)
    assert partial["excess_vs_benchmark_pct"] is None, "no benchmark claim when it is missing for any trade"


def test_drawdown_distribution_is_about_ordering_luck():
    d = drawdown_distribution([100, -300, 200, -150, 400, -250, 120, -80], 10_000)
    assert d["p99"] >= d["p95"] >= d["median"] >= 0
    assert drawdown_distribution([1, 2], 100) is None


def test_sharpe_uses_indian_conventions(monkeypatch):
    monkeypatch.setenv("RISK_FREE_RATE_PCT", "0")
    r = [0.01, -0.005, 0.007, 0.002, -0.001, 0.004]
    s0 = sharpe_ratio(r)
    monkeypatch.setenv("RISK_FREE_RATE_PCT", "6.5")
    assert sharpe_ratio(r) < s0, "a positive risk-free rate lowers the Sharpe ratio"
    assert math.isnan(sharpe_ratio([0.01]))
