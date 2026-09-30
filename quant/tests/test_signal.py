"""
Unit tests for Signal Engine.

Tests the composite weighted signal calculation and
critically verifies the Shariah hard override.
"""

import pytest
from src.engines.signal.engine import SignalEngine, Signal, SignalWeights


@pytest.fixture
def engine():
    return SignalEngine()


class TestSignalCalculation:
    def test_strong_buy_signal(self, engine):
        """High scores across all dimensions should yield STRONG_BUY."""
        result = engine.calculate(
            fundamental_score=90,
            technical_score=85,
            candlestick_score=80,
            momentum_score=85,
            volume_score=75,
            valuation_score=80,
            market_regime_score=70,
            risk_score=80,
            quality_score=85,
            shariah_status="PASS",
        )
        assert result.signal == Signal.STRONG_BUY
        assert result.overall_score >= 80

    def test_watch_signal(self, engine):
        """Mixed scores should yield WATCH."""
        result = engine.calculate(
            fundamental_score=60,
            technical_score=50,
            candlestick_score=55,
            momentum_score=45,
            volume_score=50,
            valuation_score=50,
            market_regime_score=50,
            risk_score=50,
            quality_score=50,
            shariah_status="PASS",
        )
        assert result.signal == Signal.WATCH

    def test_avoid_signal(self, engine):
        """Low scores should yield AVOID."""
        result = engine.calculate(
            fundamental_score=20,
            technical_score=30,
            candlestick_score=25,
            momentum_score=20,
            volume_score=30,
            valuation_score=25,
            market_regime_score=30,
            risk_score=20,
            quality_score=25,
            shariah_status="PASS",
        )
        assert result.signal in (Signal.AVOID, Signal.STRONG_AVOID)


class TestShariahHardOverride:
    """CRITICAL: Shariah FAIL must ALWAYS force AVOID."""

    def test_shariah_fail_overrides_strong_buy(self, engine):
        """Even with perfect scores, Shariah FAIL → AVOID."""
        result = engine.calculate(
            fundamental_score=95,
            technical_score=95,
            candlestick_score=95,
            momentum_score=95,
            volume_score=95,
            valuation_score=95,
            market_regime_score=95,
            risk_score=95,
            quality_score=95,
            shariah_status="FAIL",  # THIS MUST OVERRIDE
        )
        assert result.signal == Signal.AVOID
        assert result.shariah_override is True
        assert result.shariah_status == "FAIL"

    def test_shariah_pass_does_not_override(self, engine):
        """Shariah PASS should not force any override."""
        result = engine.calculate(
            fundamental_score=90,
            technical_score=85,
            shariah_status="PASS",
        )
        assert result.shariah_override is False


class TestWeights:
    def test_weights_sum_to_one(self):
        """Default weights must sum to 1.0."""
        w = SignalWeights()
        assert w.validate() is True

    def test_custom_weights(self):
        """Custom weights should work if they sum to 1.0."""
        w = SignalWeights(
            fundamental=0.40,
            technical=0.20,
            candlestick=0.05,
            momentum=0.05,
            volume=0.05,
            valuation=0.10,
            market_regime=0.05,
            risk=0.05,
            quality=0.05,
        )
        engine = SignalEngine(w)
        result = engine.calculate(fundamental_score=90, shariah_status="PASS")
        assert result.overall_score > 0

    def test_invalid_weights(self):
        """Weights not summing to 1.0 should raise ValueError."""
        w = SignalWeights(fundamental=0.90)  # Sums to >1
        with pytest.raises(ValueError):
            SignalEngine(w)


class TestConfidence:
    def test_full_data_confidence(self, engine):
        """All scores provided → 100% confidence."""
        result = engine.calculate(
            fundamental_score=50, technical_score=50,
            candlestick_score=50, momentum_score=50,
            volume_score=50, valuation_score=50,
            market_regime_score=50, risk_score=50,
            quality_score=50, shariah_status="PASS",
        )
        assert result.confidence == 100.0

    def test_partial_data_confidence(self, engine):
        """Only some scores → reduced confidence."""
        result = engine.calculate(
            fundamental_score=50,
            technical_score=50,
            shariah_status="PASS",
        )
        # 2 out of 9 dimensions provided
        assert result.confidence < 100.0
        assert result.confidence > 0

    def test_missing_data_uses_neutral(self, engine):
        """Missing scores should use 50 (neutral) for calculation."""
        result = engine.calculate(shariah_status="PASS")
        assert result.overall_score == pytest.approx(50.0, abs=0.1)


class TestExplanation:
    def test_explanation_contains_score(self, engine):
        """Explanation should contain the composite score."""
        result = engine.calculate(fundamental_score=80, shariah_status="PASS")
        assert "Composite score:" in result.explanation

    def test_shariah_override_in_explanation(self, engine):
        """Shariah override should be prominently noted."""
        result = engine.calculate(fundamental_score=80, shariah_status="FAIL")
        assert "SHARIAH FAIL" in result.explanation

    def test_disclaimer_present(self, engine):
        """Every result must include a disclaimer."""
        result = engine.calculate(shariah_status="PASS")
        assert "NOT a guaranteed" in result.disclaimer
