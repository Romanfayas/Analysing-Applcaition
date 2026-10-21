"""
Tests for Evidence-Driven Opportunity Discovery Screener
"""

import pytest
import numpy as np
from src.engines.technical.breakout import BreakoutEngine, BreakoutState


def test_four_structural_setups_detection():
    """Verify detection of the four required structural setups."""
    # 1. CONFIRMED_BREAKOUT setup
    highs = [100.0] * 34 + [112.0]
    lows = [95.0] * 34 + [106.0]
    closes = [98.0] * 34 + [111.0]
    opens = [96.0] * 34 + [106.5]
    volumes = [1000] * 34 + [3500]

    b_conf = BreakoutEngine("CONF_SYM", opens, highs, lows, closes, volumes, "PASS")
    r_conf = b_conf.analyze()
    assert r_conf.stage == BreakoutState.CONFIRMED_BREAKOUT
    assert r_conf.setup_type == "CONFIRMED_BREAKOUT"
    assert r_conf.rvol >= 1.30

    # 2. CONSOLIDATION setup (tight BBW near resistance)
    highs_cons = [100.0] * 30 + [100.2, 100.1, 100.3, 100.2, 100.1]
    lows_cons = [96.0] * 30 + [99.5, 99.4, 99.6, 99.5, 99.4]
    closes_cons = [98.0] * 30 + [100.0, 99.9, 100.1, 100.0, 99.9]
    opens_cons = [97.0] * 30 + [99.8, 99.7, 99.9, 99.8, 99.7]
    volumes_cons = [1000] * 35

    b_cons = BreakoutEngine("CONS_SYM", opens_cons, highs_cons, lows_cons, closes_cons, volumes_cons, "PASS")
    r_cons = b_cons.analyze()
    assert r_cons.setup_type in ["CONSOLIDATION", "APPROACHING_RESISTANCE"]
    assert r_cons.bollinger_band_width <= 0.085

    # 3. FAILED_BREAKOUT setup
    # Day 33 broke high 105, day 34 closes at 96 (below 98.5% of resistance)
    highs_fail = [100.0] * 33 + [106.0, 99.0]
    lows_fail = [95.0] * 33 + [99.0, 94.0]
    closes_fail = [98.0] * 33 + [100.0, 95.0]
    opens_fail = [96.0] * 33 + [99.0, 98.0]
    volumes_fail = [1000] * 35

    b_fail = BreakoutEngine("FAIL_SYM", opens_fail, highs_fail, lows_fail, closes_fail, volumes_fail, "PASS")
    r_fail = b_fail.analyze()
    assert r_fail.stage == BreakoutState.FAILED_BREAKOUT
    assert r_fail.setup_type == "FAILED_BREAKOUT"


def test_opportunity_discovery_disclaimers_and_evidence():
    """Verify that every result includes explicit disclaimers and multi-factor evidence."""
    highs = [100.0] * 34 + [112.0]
    lows = [95.0] * 34 + [106.0]
    closes = [98.0] * 34 + [111.0]
    opens = [96.0] * 34 + [106.5]
    volumes = [1000] * 34 + [3500]

    b = BreakoutEngine("INFY", opens, highs, lows, closes, volumes, "PASS")
    r = b.analyze()

    # Evidence fields
    assert r.liquidity_adt_cr >= 0.0
    assert r.market_regime in ["BULLISH_TREND", "BEARISH_TREND", "RANGING"]
    assert len(r.score_breakdown) >= 5
    assert "price_structure" in r.score_breakdown
    assert "volume_participation" in r.score_breakdown
    assert "volatility_compression" in r.score_breakdown
    assert "trend_momentum" in r.score_breakdown
    assert "risk_reward_quality" in r.score_breakdown

    # Disclaimers
    assert "NOT a calibrated probability of profit" in r.score_disclaimer
    assert "Volume demand proxy is derived from bar volume distribution" in r.market_depth_disclosure
    assert "DATA_UNAVAILABLE" in r.order_book_depth_status


def test_shariah_hard_gate_enforcement():
    """Verify that non-PASS status zeros out score and blocks candidate discovery."""
    highs = [100.0] * 34 + [112.0]
    lows = [95.0] * 34 + [106.0]
    closes = [98.0] * 34 + [111.0]
    opens = [96.0] * 34 + [106.5]
    volumes = [1000] * 34 + [3500]

    for invalid_status in ["FAIL", "UNKNOWN", "DATA_INSUFFICIENT", "REVIEW_REQUIRED"]:
        b = BreakoutEngine("TEST_GATE", opens, highs, lows, closes, volumes, invalid_status)
        r = b.analyze()
        assert r.is_shariah_eligible is False
        assert r.overall_score == 0.0
        assert any("SHARIAH INELIGIBLE" in r_msg for r_msg in r.reasons)
