"""
Unit Tests for Breakout Research Engine
"""

import pytest
from src.engines.technical.breakout import BreakoutEngine, BreakoutState


def test_breakout_deterministic():
    """Verify that identical OHLCV inputs produce exact identical output."""
    highs = [100.0] * 34 + [112.0]
    lows = [95.0] * 34 + [106.0]
    closes = [98.0] * 34 + [111.0]
    opens = [96.0] * 34 + [106.5]
    volumes = [1000] * 34 + [3500]

    b1 = BreakoutEngine("RELIANCE", opens, highs, lows, closes, volumes, "PASS")
    r1 = b1.analyze()

    b2 = BreakoutEngine("RELIANCE", opens, highs, lows, closes, volumes, "PASS")
    r2 = b2.analyze()

    assert r1.stage == r2.stage
    assert r1.overall_score == r2.overall_score
    assert r1.resistance_level == r2.resistance_level
    assert r1.invalidation_level == r2.invalidation_level
    assert r1.entry_zone == r2.entry_zone


def test_shariah_hard_gate():
    """Verify that non-PASS Shariah statuses cannot pass the buy gate."""
    highs = [100.0] * 34 + [112.0]
    lows = [95.0] * 34 + [106.0]
    closes = [98.0] * 34 + [111.0]
    opens = [96.0] * 34 + [106.5]
    volumes = [1000] * 34 + [3500]

    for status in ["FAIL", "UNKNOWN", "DATA_INSUFFICIENT", "REVIEW_REQUIRED"]:
        b = BreakoutEngine("TEST_SYM", opens, highs, lows, closes, volumes, status)
        r = b.analyze()
        assert r.is_shariah_eligible is False
        assert r.overall_score == 0.0
        assert any("SHARIAH INELIGIBLE" in reason for reason in r.reasons)


def test_confirmed_breakout_conditions():
    """Verify mathematical rules for CONFIRMED_BREAKOUT state."""
    highs = [100.0] * 34 + [112.0]
    lows = [95.0] * 34 + [106.0]
    closes = [98.0] * 34 + [111.0]
    opens = [96.0] * 34 + [106.5]
    volumes = [1000] * 34 + [3500]

    b = BreakoutEngine("INFY", opens, highs, lows, closes, volumes, "PASS")
    r = b.analyze()

    assert r.stage == BreakoutState.CONFIRMED_BREAKOUT
    assert r.current_price > r.resistance_level
    assert r.rvol >= 1.30
    assert r.overall_score >= 60.0
    assert r.risk_reward_ratio >= 1.0


def test_insufficient_data():
    """Verify that fewer than 30 bars produces UNAVAILABLE state."""
    b = BreakoutEngine("TCS", [100] * 15, [105] * 15, [95] * 15, [102] * 15, [1000] * 15, "PASS")
    r = b.analyze()

    assert r.stage == BreakoutState.UNAVAILABLE
    assert r.overall_score == 0.0


def test_failed_breakout_detection():
    """Verify that false breakout followed by immediate reversal is marked FAILED_BREAKOUT."""
    # Breakout was attempted 2 bars ago to 112, but current close drops to 97
    highs = [100.0] * 32 + [112.0, 102.0, 99.0]
    lows = [95.0] * 32 + [98.0, 96.0, 95.0]
    closes = [98.0] * 32 + [102.0, 97.5, 96.0]
    opens = [96.0] * 32 + [98.0, 101.0, 97.0]
    volumes = [1000] * 32 + [3000, 1500, 1200]

    b = BreakoutEngine("HDFCBANK", opens, highs, lows, closes, volumes, "PASS")
    r = b.analyze()

    assert r.stage == BreakoutState.FAILED_BREAKOUT
    assert any("False breakout" in reason for reason in r.reasons)


# ---- P1.9.25: structure_score is visible even when the Shariah gate zeroes overall_score ----

def _trend_series(n=45):
    import numpy as np
    base = 100 + np.cumsum(np.full(n, 0.3))
    close = base.tolist()
    high = (base + 1).tolist()
    low = (base - 1).tolist()
    open_ = (base - 0.2).tolist()
    vol = [1_000_000] * n
    return open_, high, low, close, vol


def test_structure_score_survives_the_shariah_gate():
    from src.engines.technical.breakout import BreakoutEngine
    o, h, l, c, v = _trend_series()
    gated = BreakoutEngine("X", o, h, l, c, v, shariah_status="DATA_INSUFFICIENT").analyze()
    passed = BreakoutEngine("X", o, h, l, c, v, shariah_status="PASS").analyze()
    assert gated.overall_score == 0.0, "the hard gate on overall_score is unchanged"
    assert gated.is_shariah_eligible is False
    assert gated.structure_score is not None and gated.structure_score > 0
    assert gated.structure_score == passed.overall_score, "same structural evidence, with or without the gate"
    assert abs(sum(gated.score_breakdown.values()) - gated.structure_score) < 0.11


def test_structure_score_is_absent_when_there_is_too_little_data():
    from src.engines.technical.breakout import BreakoutEngine
    o, h, l, c, v = _trend_series(12)
    res = BreakoutEngine("X", o, h, l, c, v, shariah_status="PASS").analyze()
    assert res.stage == "UNAVAILABLE"
    assert res.structure_score is None, "never a made-up score for insufficient history"
