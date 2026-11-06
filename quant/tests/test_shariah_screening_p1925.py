"""P1.9.25: Shariah screening from reported figures. Pure-function tests, no network."""
from datetime import date

from src.engines.fundamental.screening import classify_business, evaluate

TODAY = date(2026, 11, 3)
PERIOD = date(2026, 3, 31)


def ev(**over):
    base = dict(symbol="XYZ", industry="Information Technology Services", market_cap=1000.0, total_debt=50.0, total_cash=100.0,
                revenue=500.0, interest_income=5.0, receivables=100.0, period_end=PERIOD, today=TODAY)
    base.update(over)
    return evaluate(**base)


def test_complete_permissible_business_within_limits_passes():
    r = ev()
    assert r["status"] == "PASS"
    assert r["missing_fields"] == []
    assert r["ratios"] == {"debt": 0.05, "cash": 0.1, "interest": 0.01, "receivables": 0.1}


def test_missing_interest_income_is_never_assumed_zero():
    r = ev(interest_income=None)
    assert r["status"] == "DATA_INSUFFICIENT"
    assert "interest_income" in r["missing_fields"]


def test_missing_receivables_stays_insufficient():
    assert ev(receivables=None)["status"] == "DATA_INSUFFICIENT"


def test_each_ratio_breach_fails():
    assert ev(total_debt=400.0)["status"] == "FAIL"        # debt 0.40 > 0.33
    assert ev(total_cash=400.0)["status"] == "FAIL"        # cash 0.40 > 0.33
    assert ev(interest_income=50.0)["status"] == "FAIL"    # 10% of revenue > 5%
    assert ev(receivables=600.0)["status"] == "FAIL"       # 0.60 > 0.49


def test_breach_fails_even_when_other_figures_are_missing():
    r = ev(total_debt=400.0, interest_income=None, receivables=None)
    assert r["status"] == "FAIL"


def test_prohibited_industries_fail_regardless_of_ratios():
    for industry in ("Banks - Regional", "Tobacco", "Insurance - Life", "Gambling", "Beverages - Brewers"):
        assert ev(industry=industry)["status"] == "FAIL", industry
    assert ev(symbol="HDFCBANK", industry=None)["status"] == "FAIL"


def test_unknown_or_mixed_industry_cannot_pass():
    assert classify_business("XYZ", None)["pass"] is None
    assert ev(industry=None)["status"] == "DATA_INSUFFICIENT"
    assert ev(industry="Aerospace & Defense")["status"] == "DATA_INSUFFICIENT"


def test_stale_annual_report_cannot_pass():
    assert ev(period_end=date(2024, 3, 31))["status"] == "DATA_INSUFFICIENT"


def test_zero_market_cap_is_missing_not_a_pass():
    r = ev(market_cap=0.0)
    assert r["status"] == "DATA_INSUFFICIENT"
    assert "market_cap" in r["missing_fields"]
