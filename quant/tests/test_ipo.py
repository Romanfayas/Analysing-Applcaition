"""
Unit tests for the IPO Predictor Engine.
"""

import pytest
from src.engines.ipo.predictor import IPOPredictor, IPOData, IPODecision


@pytest.fixture
def predictor():
    return IPOPredictor()


class TestIPOPredictor:
    def test_strong_apply_with_high_gmp_and_sub(self, predictor):
        """High GMP and high subscriptions should yield APPLY."""
        data = IPOData(
            issue_price=100.0,
            gmp=25.0, # 25% premium
            qib_subscription=100.0, # log2(100) ~ 6.6
            nii_subscription=50.0,
            retail_subscription=10.0,
            shariah_status="PASS"
        )
        result = predictor.predict(data)
        
        assert result.decision == IPODecision.APPLY
        assert result.expected_gain_percent > 25.0 # GMP + sub boost
        assert result.bull_listing_price > result.base_listing_price
        assert result.bear_listing_price < result.base_listing_price
        assert result.shariah_override is False

    def test_avoid_with_negative_gmp(self, predictor):
        """Negative GMP should yield AVOID."""
        data = IPOData(
            issue_price=100.0,
            gmp=-10.0, # -10% premium
            qib_subscription=1.0,
            nii_subscription=1.0,
            retail_subscription=1.0,
            shariah_status="PASS"
        )
        result = predictor.predict(data)
        
        assert result.decision == IPODecision.AVOID
        assert result.expected_gain_percent == -10.0
        assert result.base_listing_price == 90.0

    def test_watch_with_moderate_gmp_and_low_sub(self, predictor):
        """Moderate GMP with low subs should yield WATCH."""
        data = IPOData(
            issue_price=100.0,
            gmp=10.0, # 10% premium
            qib_subscription=1.5,
            nii_subscription=1.5,
            retail_subscription=1.5,
            shariah_status="PASS"
        )
        result = predictor.predict(data)
        
        assert result.decision == IPODecision.WATCH
        assert result.expected_gain_percent > 10.0
        assert result.expected_gain_percent < 15.0

    def test_shariah_fail_hard_override(self, predictor):
        """CRITICAL: Shariah FAIL must override a strong APPLY recommendation."""
        data = IPOData(
            issue_price=100.0,
            gmp=100.0, # 100% premium - massive
            qib_subscription=500.0, # Massive subscription
            nii_subscription=200.0,
            retail_subscription=100.0,
            shariah_status="FAIL" # MUST OVERRIDE
        )
        result = predictor.predict(data)
        
        assert result.decision == IPODecision.AVOID
        assert result.shariah_override is True
        assert "SHARIAH FAIL" in result.explanation
        assert "override to AVOID" in result.explanation
        
        # Financial predictions should still calculate correctly
        assert result.expected_gain_percent > 100.0

    def test_zero_issue_price_raises_error(self, predictor):
        """Zero issue price is invalid."""
        with pytest.raises(ValueError):
            predictor.predict(IPOData(issue_price=0))
