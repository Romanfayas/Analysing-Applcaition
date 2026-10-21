"""
Tests for Provider-Readiness Layer
"""

import pytest
from src.engines.research.provider_readiness import ProviderAuditor


def test_provider_readiness_audit_structure():
    """Verify provider readiness audit contains all required providers and fields."""
    report = ProviderAuditor.audit_all()

    assert "timestamp" in report
    assert "summary" in report
    assert "providers" in report
    assert "integration_roadmap" in report
    assert len(report["providers"]) == 4

    names = {p["name"] for p in report["providers"]}
    assert "yahoo_finance" in names
    assert "nse_india" in names
    assert "angel_one_smartapi" in names
    assert "mock_provider" in names


def test_yahoo_finance_limitations_and_licensing():
    """Verify Yahoo Finance is honestly classified with 15m delay and non-commercial license."""
    report = ProviderAuditor.audit_all()
    yf = next(p for p in report["providers"] if p["name"] == "yahoo_finance")

    assert yf["feed_classification"] == "DELAYED_15MIN"
    assert "NONE" in yf["market_depth"]
    assert yf["redistribution_allowed"] is False
    assert yf["production_ready"] is False
    assert "Non-commercial" in yf["licensing_requirements"]
    assert any("delay" in lim.lower() for lim in yf["limitations"])


def test_angel_one_credentials_status():
    """Verify Angel One SmartAPI is not claimed operational without credentials."""
    report = ProviderAuditor.audit_all()
    angel = next(p for p in report["providers"] if p["name"] == "angel_one_smartapi")

    assert angel["feed_classification"] == "LIVE_STREAMING"
    assert "LEVEL_2" in angel["market_depth"]
    # In test environment without active credentials, must be CONFIG_REQUIRED
    if not report["licensed_vendor_active"]:
        assert angel["operational_status"] == "CONFIG_REQUIRED"
        assert angel["production_ready"] is False


def test_nse_india_scraper_limitations():
    """Verify NSE India scraper is flagged as degraded/brittle due to cookie sessions."""
    report = ProviderAuditor.audit_all()
    nse = next(p for p in report["providers"] if p["name"] == "nse_india")

    assert nse["feed_classification"] == "WEB_SCRAPED_SNAPSHOT"
    assert nse["operational_status"] == "DEGRADED"
    assert nse["production_ready"] is False
    assert any("cookie" in mode.lower() for mode in nse["observed_failure_modes"])
