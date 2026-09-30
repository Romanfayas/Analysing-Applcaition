"""
Unit tests for Shariah Screening Engine.

Tests the configurable screening rules, ratio checks,
and hard trading restriction enforcement.
"""

import pytest
from src.engines.shariah.screener import ShariahScreener, ShariahRules, ShariahStatus


@pytest.fixture
def default_rules():
    """Default AAOIFI-based screening rules."""
    return ShariahRules()


@pytest.fixture
def screener(default_rules):
    """Shariah screener with default rules."""
    return ShariahScreener(default_rules)


class TestShariahScreening:
    def test_pass_all_criteria(self, screener):
        """Stock that passes all criteria should be PASS."""
        result = screener.screen(
            total_debt=200_000,       # 20% of market cap (< 33%)
            market_cap=1_000_000,
            interest_income=3_000,     # 3% of revenue (< 5%)
            revenue=100_000,
            cash_and_deposits=200_000, # 20% of market cap (< 33%)
            receivables=400_000,       # 40% of market cap (< 49%)
            business_activities=["technology", "manufacturing"],
        )
        assert result.status == ShariahStatus.PASS
        assert all(r.passed is True for r in result.ratio_results)
        assert result.business_activity_pass is True

    def test_fail_debt_ratio(self, screener):
        """Stock with debt > 33% of market cap should FAIL."""
        result = screener.screen(
            total_debt=400_000,       # 40% of market cap (> 33%)
            market_cap=1_000_000,
            interest_income=3_000,
            revenue=100_000,
            cash_and_deposits=200_000,
            receivables=400_000,
            business_activities=["technology"],
        )
        assert result.status == ShariahStatus.FAIL
        # Debt ratio should be failed
        debt_result = next(r for r in result.ratio_results if r.name == "debt_to_market_cap")
        assert debt_result.passed is False
        assert debt_result.value == pytest.approx(0.4, abs=0.001)

    def test_fail_interest_income(self, screener):
        """Stock with interest income > 5% of revenue should FAIL."""
        result = screener.screen(
            total_debt=200_000,
            market_cap=1_000_000,
            interest_income=8_000,     # 8% of revenue (> 5%)
            revenue=100_000,
            cash_and_deposits=200_000,
            receivables=400_000,
            business_activities=["technology"],
        )
        assert result.status == ShariahStatus.FAIL

    def test_fail_prohibited_business(self, screener):
        """Stock in prohibited business should FAIL."""
        result = screener.screen(
            total_debt=200_000,
            market_cap=1_000_000,
            interest_income=3_000,
            revenue=100_000,
            cash_and_deposits=200_000,
            receivables=400_000,
            business_activities=["technology", "alcohol", "gambling"],
        )
        assert result.status == ShariahStatus.FAIL
        assert result.business_activity_pass is False
        assert "alcohol" in result.business_activity_notes

    def test_data_insufficient(self, screener):
        """Missing financial data should return DATA_INSUFFICIENT."""
        result = screener.screen(
            total_debt=None,
            market_cap=None,
            interest_income=None,
            revenue=None,
            cash_and_deposits=None,
            receivables=None,
        )
        assert result.status == ShariahStatus.DATA_INSUFFICIENT

    def test_partial_data(self, screener):
        """Some missing data should return DATA_INSUFFICIENT if no FAIL."""
        result = screener.screen(
            total_debt=200_000,
            market_cap=1_000_000,
            interest_income=None,  # missing
            revenue=None,          # missing
            cash_and_deposits=200_000,
            receivables=400_000,
            business_activities=["technology"],
        )
        # Interest income ratio can't be calculated
        assert result.status == ShariahStatus.DATA_INSUFFICIENT

    def test_fail_overrides_insufficient(self, screener):
        """FAIL should take precedence over DATA_INSUFFICIENT."""
        result = screener.screen(
            total_debt=500_000,       # 50% - FAIL
            market_cap=1_000_000,
            interest_income=None,     # insufficient
            revenue=None,             # insufficient
            cash_and_deposits=200_000,
            receivables=400_000,
            business_activities=["technology"],
        )
        assert result.status == ShariahStatus.FAIL

    def test_custom_rules(self):
        """Custom rule thresholds should be respected."""
        custom_rules = ShariahRules(
            debt_to_market_cap_max=0.25,  # Stricter
            interest_income_to_revenue_max=0.03,
        )
        screener = ShariahScreener(custom_rules)

        # Would pass default (33%) but fails stricter (25%)
        result = screener.screen(
            total_debt=280_000,       # 28% > 25%
            market_cap=1_000_000,
            interest_income=2_000,
            revenue=100_000,
            cash_and_deposits=200_000,
            receivables=400_000,
            business_activities=["technology"],
        )
        assert result.status == ShariahStatus.FAIL

    def test_rules_from_dict(self):
        """Rules should be loadable from dictionary (as from DB)."""
        config = {
            "debt_to_market_cap_max": 0.30,
            "interest_income_to_revenue_max": 0.04,
            "cash_and_deposits_to_market_cap_max": 0.30,
            "receivables_to_market_cap_max": 0.45,
            "prohibited_business_activities": ["alcohol", "gambling"],
            "purification_method": "dividend_based",
        }
        rules = ShariahRules.from_dict(config)
        assert rules.debt_to_market_cap_max == 0.30
        assert rules.interest_income_to_revenue_max == 0.04
        assert len(rules.prohibited_business_activities) == 2

    def test_purification_calculation(self, screener):
        """Purification should be calculated for PASS stocks."""
        result = screener.screen(
            total_debt=200_000,
            market_cap=1_000_000,
            interest_income=3_000,
            revenue=100_000,
            cash_and_deposits=200_000,
            receivables=400_000,
            business_activities=["technology"],
            dividend_per_share=10.0,
        )
        assert result.status == ShariahStatus.PASS
        # Purification = DPS * (interest_income / revenue) = 10 * 0.03 = 0.3
        assert result.purification_per_share == pytest.approx(0.3, abs=0.001)

    def test_zero_denominator(self, screener):
        """Zero denominator should not crash, return DATA_INSUFFICIENT."""
        result = screener.screen(
            total_debt=200_000,
            market_cap=0,  # zero!
            interest_income=3_000,
            revenue=100_000,
            cash_and_deposits=200_000,
            receivables=400_000,
        )
        # Should handle gracefully
        assert result.status in (ShariahStatus.DATA_INSUFFICIENT, ShariahStatus.FAIL)


class TestShariahHardRestriction:
    """Tests that Shariah FAIL is a hard trading restriction."""

    def test_fail_cannot_be_overridden(self, screener):
        """A FAIL status must remain FAIL regardless of other scores."""
        result = screener.screen(
            total_debt=500_000,
            market_cap=1_000_000,
            interest_income=8_000,
            revenue=100_000,
            cash_and_deposits=500_000,
            receivables=600_000,
            business_activities=["alcohol"],
        )
        # Multiple failures - must be FAIL
        assert result.status == ShariahStatus.FAIL

    def test_never_replace_missing_with_zero(self, screener):
        """Missing financial data must never be silently replaced with zero."""
        result = screener.screen(
            total_debt=None,
            market_cap=1_000_000,
            interest_income=None,
            revenue=None,
            cash_and_deposits=None,
            receivables=None,
            business_activities=["technology"],
        )
        # Should NOT pass just because missing values default to 0
        assert result.status != ShariahStatus.PASS
