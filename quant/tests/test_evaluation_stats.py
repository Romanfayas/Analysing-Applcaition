from datetime import date, timedelta
import pytest
from src.engines.research.evaluation_stats import (
    ClosedTrade, equity_curve, max_drawdown, mean_ci, reconcile_to_ledger, verdict, wilson_interval,
)

D0 = date(2026, 1, 5)


def test_unavailable_when_empty():
    assert mean_ci([])["mean"] is None and wilson_interval(0, 0) is None
    assert mean_ci([0.01])["t_ci"] is None


def test_drawdown_is_chronological_equity_not_summed_returns():
    # two overlapping trades: -5% then +5% (sums to 0) but equity path draws down 10% first
    t = [ClosedTrade(D0, D0 + timedelta(days=1), -50000, 100000), ClosedTrade(D0 + timedelta(days=2), D0 + timedelta(days=3), -50000, 100000),
         ClosedTrade(D0 + timedelta(days=4), D0 + timedelta(days=7), 100000, 100000)]
    c = equity_curve(t, 1_000_000, mtm=False)
    dd = max_drawdown(c)
    assert dd["max_drawdown_pct"] == pytest.approx(-10.0, abs=0.01)
    assert reconcile_to_ledger(c, t, 1_000_000)["reconciled"]


def test_mtm_uses_marks_and_still_reconciles_at_end():
    t = [ClosedTrade(D0, D0 + timedelta(days=4), 1000, 100000, marks={D0 + timedelta(days=i): -0.03 for i in range(4)})]
    c = equity_curve(t, 1_000_000, mtm=True)
    assert max_drawdown(c)["max_drawdown_pct"] < 0
    assert reconcile_to_ledger(c, t, 1_000_000)["reconciled"]


def test_verdict_states():
    few = verdict([0.01] * 10, {"BULL": 10}, 10, 3)
    assert few["state"] == "INSUFFICIENT_EVIDENCE" and few["live_trading_approved"] is False
    rng = [0.02 + (i % 7 - 3) * 0.004 for i in range(120)]
    assert verdict(rng, {"BULL": 60, "RANGE": 60}, 120, 0)["state"] == "PASSED_PREREGISTERED_CRITERIA"
    assert verdict([-v for v in rng], {"BULL": 60, "RANGE": 60}, 120, 0)["state"] == "FAILED_VALIDATION"
    noisy = [(-1) ** i * 0.03 for i in range(120)]
    assert verdict(noisy, {"BULL": 60, "RANGE": 60}, 120, 0)["state"] == "INCONCLUSIVE"
    assert verdict(rng, {"BULL": 120}, 120, 0)["state"] == "INSUFFICIENT_EVIDENCE"      # one regime only


def test_wilson_interval_bounds():
    lo, hi = wilson_interval(5, 10)
    assert 0.2 < lo < 0.5 < hi < 0.8
