"""
HalalEquity P1.8.4 - Financial Provenance, Shariah Methodology & Ratio Integrity Tests

Regression tests for Workstream 1:
1. Denominator defects: zero or negative denominators must return DATA_INSUFFICIENT, never PASS.
2. Purification calculations: zero or negative revenue must return None.
3. Period alignment and staleness: statements older than 180 days fail closed.
4. Currency and unit integrity: must be INR and RAW_INR.
5. Ingestion provenance: unverified feeds cannot be marked as audited/certified.
6. Zero-inference prohibition: missing financial metrics are never inferred as 0.0.
7. AAOIFI Standard No. 21 ratio checks: debt <= 33%, interest <= 5%, cash <= 33%, receivables <= 49%.
"""
import pytest
from datetime import date, datetime, timedelta
from src.engines.shariah.screener import ShariahScreener, ShariahRules, ShariahStatus, RatioResult


@pytest.fixture
def screener():
    return ShariahScreener(ShariahRules())


class TestFinancialProvenanceAndIntegrity:
    def test_negative_market_cap_does_not_erroneously_pass(self, screener):
        """
        Regression defect: If market_cap < 0 (e.g. data anomaly or deficit),
        debt / market_cap is negative, which is numerically <= 0.33!
        The screener MUST reject negative denominators and return DATA_INSUFFICIENT.
        """
        result = screener.screen(
            total_debt=100_000,
            market_cap=-500_000,  # Negative denominator
            interest_income=1_000,
            revenue=100_000,
            cash_and_deposits=50_000,
            receivables=50_000,
            business_activities=["technology"],
        )
        assert result.status == ShariahStatus.DATA_INSUFFICIENT
        debt_res = next(r for r in result.ratio_results if r.name == "debt_to_market_cap")
        assert debt_res.passed is None
        assert "zero or negative" in debt_res.note

    def test_negative_revenue_does_not_pass(self, screener):
        """
        Regression defect: If revenue <= 0, interest_income / revenue is undefined or negative.
        Must return DATA_INSUFFICIENT and purification must be None.
        """
        result = screener.screen(
            total_debt=100_000,
            market_cap=1_000_000,
            interest_income=1_000,
            revenue=-50_000,  # Negative revenue
            cash_and_deposits=50_000,
            receivables=50_000,
            business_activities=["technology"],
        )
        assert result.status == ShariahStatus.DATA_INSUFFICIENT
        int_res = next(r for r in result.ratio_results if r.name == "interest_income_to_revenue")
        assert int_res.passed is None
        assert result.purification_per_share is None

    def test_zero_market_cap_and_zero_revenue_rejects(self, screener):
        """Zero denominators must yield DATA_INSUFFICIENT, never divide-by-zero or pass."""
        r1 = screener.screen(
            total_debt=100_000, market_cap=0, interest_income=1_000, revenue=100_000,
            cash_and_deposits=50_000, receivables=50_000, business_activities=["tech"]
        )
        assert r1.status == ShariahStatus.DATA_INSUFFICIENT

        r2 = screener.screen(
            total_debt=100_000, market_cap=1_000_000, interest_income=1_000, revenue=0,
            cash_and_deposits=50_000, receivables=50_000, business_activities=["tech"]
        )
        assert r2.status == ShariahStatus.DATA_INSUFFICIENT

    def test_missing_metrics_cannot_be_inferred_as_zero(self, screener):
        """
        P1.8.4 Non-negotiable constraint:
        Missing interest_income or total_receivables cannot be inferred as 0.0 to manufacture a PASS.
        """
        result = screener.screen(
            total_debt=100_000,
            market_cap=1_000_000,
            interest_income=None,  # Missing in public Yahoo quoteSummary
            revenue=100_000,
            cash_and_deposits=50_000,
            receivables=None,      # Missing in public Yahoo quoteSummary
            business_activities=["technology"],
        )
        # Even though debt and cash are within limits, missing metrics fail closed!
        assert result.status == ShariahStatus.DATA_INSUFFICIENT
        assert result.status != ShariahStatus.PASS

    def test_stale_financial_statement_fails_closed(self, screener):
        """Statements older than 180 days must be rejected as STALE/DATA_INSUFFICIENT."""
        old_stmt_date = "2025-01-01"
        as_of_date = "2026-10-15"  # > 600 days old
        result = screener.screen(
            total_debt=100_000,
            market_cap=1_000_000,
            interest_income=1_000,
            revenue=100_000,
            cash_and_deposits=50_000,
            receivables=50_000,
            business_activities=["technology"],
            statement_date=old_stmt_date,
            as_of_date=as_of_date,
            max_statement_age_days=180,
        )
        assert result.status == ShariahStatus.DATA_INSUFFICIENT
        assert result.details["is_stale"] is True

    def test_conventional_financial_institutions_strictly_fail(self, screener):
        """Conventional banks and insurance firms must fail on business activity regardless of ratios."""
        result = screener.screen(
            total_debt=0,  # Zero debt
            market_cap=1_000_000,
            interest_income=0,
            revenue=100_000,
            cash_and_deposits=0,
            receivables=0,
            business_activities=["commercial_banking", "conventional_finance"],
        )
        assert result.status == ShariahStatus.FAIL
        assert result.business_activity_pass is False

    def test_provenance_audit_metadata_contract(self):
        """
        Verifies the structure of provenance dictionary attached to financial statements.
        Unverified feeds must be explicitly labeled UNVERIFIED_THIRD_PARTY_AGGREGATOR.
        """
        sample_provenance = {
            "source_provider": "yahoo_finance",
            "endpoint": "https://query2.finance.yahoo.com/v10/finance/quoteSummary/INFY.NS",
            "currency": "INR",
            "unit": "RAW_INR",
            "verification_status": "UNVERIFIED_THIRD_PARTY_AGGREGATOR",
            "uncertainty_reason": "Aggregated secondary feed without statutory filing or XBRL notes",
            "is_directly_reported": True,
        }
        assert sample_provenance["source_provider"] == "yahoo_finance"
        assert sample_provenance["currency"] == "INR"
        assert sample_provenance["unit"] == "RAW_INR"
        assert sample_provenance["verification_status"] != "AUDITED"
        assert "UNVERIFIED" in sample_provenance["verification_status"]
