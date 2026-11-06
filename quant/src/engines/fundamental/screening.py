"""
Shariah screening from reported financials (P1.9.25).

Previously the screener never asked the vendor for interest income or receivables and recorded them as "not in feed",
so every permissible business ended up DATA_INSUFFICIENT. Yahoo's fundamentals-timeseries endpoint does carry both
(annual figures from company filings), so they are fetched here and the full rule set is applied.

Policy, unchanged from P1.8.4:
  * a missing figure is never inferred as zero - the stock stays DATA_INSUFFICIENT and the missing fields are listed;
  * a prohibited business or any breached ratio is FAIL;
  * PASS needs a known, permissible business and every ratio present and within its limit;
  * the figures are an unverified third-party aggregation, so each record says so.

The evaluation itself (`evaluate`) is a pure function so it can be tested without a network.
"""
from __future__ import annotations

import http.cookiejar
import json
import logging
import urllib.parse
import urllib.request
from datetime import date, datetime, timezone
from typing import Any, Dict, List, Optional

logger = logging.getLogger("shariah_screening")

SOURCE = "YAHOO_FUNDAMENTALS_TIMESERIES_V2"
STALE_AFTER_DAYS = 550  # an annual report older than this is not "current"

# Limits come from shariah_rule_versions.rules (rule set 1, version 1); these defaults match it.
DEFAULT_RULES = {
    "debt_to_market_cap_max": 0.33,
    "cash_and_interest_bearing_deposits_to_market_cap_max": 0.33,
    "interest_income_to_revenue_max": 0.05,
    "receivables_to_market_cap_max": 0.49,
}

# Yahoo `industry` values that make the core business impermissible under the rule set's prohibited activities.
PROHIBITED_INDUSTRIES = {
    "Banks - Regional": "conventional banking",
    "Banks - Diversified": "conventional banking",
    "Credit Services": "interest-based lending",
    "Mortgage Finance": "interest-based lending",
    "Capital Markets": "conventional finance",
    "Insurance - Life": "conventional insurance",
    "Insurance - Property & Casualty": "conventional insurance",
    "Insurance - Diversified": "conventional insurance",
    "Insurance - Reinsurance": "conventional insurance",
    "Insurance - Specialty": "conventional insurance",
    "Insurance Brokers": "conventional insurance",
    "Financial Conglomerates": "conventional finance",
    "Tobacco": "tobacco",
    "Beverages - Wineries & Distilleries": "alcohol",
    "Beverages - Brewers": "alcohol",
    "Gambling": "gambling",
    "Resorts & Casinos": "gambling",
}
# Industries that can contain a prohibited line of business; a person has to look at the segment mix.
REVIEW_INDUSTRIES = {"Aerospace & Defense", "Asset Management", "Conglomerates"}

# Used when the vendor gives no industry but the business is known to be conventional finance.
KNOWN_CONVENTIONAL_FINANCE = {
    "HDFCBANK", "ICICIBANK", "SBIN", "KOTAKBANK", "AXISBANK", "INDUSINDBK",
    "BAJFINANCE", "BAJAJFINSV", "HDFCLIFE", "SBILIFE",
}

TIMESERIES_TYPES = [
    "annualInterestIncome", "annualInterestIncomeNonOperating", "annualAccountsReceivable",
    "annualTotalRevenue", "annualTotalDebt", "annualCashCashEquivalentsAndShortTermInvestments",
    "annualTotalAssets",
]


def classify_business(symbol: str, industry: Optional[str]) -> Dict[str, Any]:
    """Return {pass: True|False|None, note}. None means a person must decide - never a silent PASS."""
    if symbol in KNOWN_CONVENTIONAL_FINANCE:
        return {"pass": False, "note": "Prohibited business: conventional banking / interest-based finance"}
    if not industry:
        return {"pass": None, "note": "Industry not provided by the data vendor; business activity not assessed"}
    if industry in PROHIBITED_INDUSTRIES:
        return {"pass": False, "note": f"Prohibited business: {PROHIBITED_INDUSTRIES[industry]} (industry: {industry})"}
    if industry in REVIEW_INDUSTRIES:
        return {"pass": None, "note": f"Industry '{industry}' may include a prohibited line of business; needs manual review"}
    return {"pass": True, "note": f"Permissible core business (industry: {industry})"}


def _ratio(num: Optional[float], den: Optional[float]) -> Optional[float]:
    if num is None or den is None or den <= 0:
        return None
    return round(num / den, 4)


def evaluate(
    symbol: str,
    industry: Optional[str],
    market_cap: Optional[float],
    total_debt: Optional[float],
    total_cash: Optional[float],
    revenue: Optional[float],
    interest_income: Optional[float],
    receivables: Optional[float],
    period_end: Optional[date],
    today: Optional[date] = None,
    rules: Optional[Dict[str, float]] = None,
) -> Dict[str, Any]:
    """Apply the rule set. Returns status, each ratio with its pass flag, the reason and the missing fields."""
    r = {**DEFAULT_RULES, **(rules or {})}
    today = today or date.today()
    biz = classify_business(symbol, industry)

    debt = _ratio(total_debt, market_cap)
    cash = _ratio(total_cash, market_cap)
    interest = _ratio(interest_income, revenue)
    recv = _ratio(receivables, market_cap)
    flags = {
        "debt": None if debt is None else debt <= r["debt_to_market_cap_max"],
        "cash": None if cash is None else cash <= r["cash_and_interest_bearing_deposits_to_market_cap_max"],
        "interest": None if interest is None else interest <= r["interest_income_to_revenue_max"],
        "recv": None if recv is None else recv <= r["receivables_to_market_cap_max"],
    }

    missing: List[str] = []
    if market_cap is None or market_cap <= 0:
        missing.append("market_cap")
    for name, val in (("total_debt", total_debt), ("total_cash", total_cash), ("total_revenue", revenue),
                      ("interest_income", interest_income), ("total_receivables", receivables)):
        if val is None:
            missing.append(name)
    stale = period_end is None or (today - period_end).days > STALE_AFTER_DAYS

    breaches = []
    if flags["debt"] is False:
        breaches.append(f"debt/market cap {debt:.4f} > {r['debt_to_market_cap_max']}")
    if flags["cash"] is False:
        breaches.append(f"cash and deposits/market cap {cash:.4f} > {r['cash_and_interest_bearing_deposits_to_market_cap_max']}")
    if flags["interest"] is False:
        breaches.append(f"interest income/revenue {interest:.4f} > {r['interest_income_to_revenue_max']}")
    if flags["recv"] is False:
        breaches.append(f"receivables/market cap {recv:.4f} > {r['receivables_to_market_cap_max']}")

    if biz["pass"] is False:
        status, reason = "FAIL", biz["note"]
    elif breaches:
        status, reason = "FAIL", "Excluded by ratio: " + "; ".join(breaches)
    elif biz["pass"] is None:
        status, reason = "DATA_INSUFFICIENT", biz["note"]
    elif missing:
        status, reason = "DATA_INSUFFICIENT", "Financial figures not reported by the data vendor: " + ", ".join(missing) + " (missing values are never assumed to be zero)"
    elif stale:
        status, reason = "DATA_INSUFFICIENT", f"Latest annual report ({period_end}) is too old to rely on"
    else:
        status, reason = "PASS", "Permissible business and every ratio within its limit"

    return {
        "status": status, "reason": reason, "business": biz, "missing_fields": missing, "stale": stale,
        "ratios": {"debt": debt, "cash": cash, "interest": interest, "receivables": recv}, "flags": flags,
        "limits": r,
    }


# ----------------------------------------------------------------------------- data access

class YahooFundamentals:
    """Fetches profile, key figures and annual statement values for an NSE symbol."""

    UA = {"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)"}

    def __init__(self) -> None:
        self.cj = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.cj))
        self.crumb: Optional[str] = None

    def _get(self, url: str, timeout: int = 15) -> Any:
        req = urllib.request.Request(url, headers=self.UA)
        with self.opener.open(req, timeout=timeout) as resp:
            return json.loads(resp.read().decode("utf-8"))

    def _ensure_crumb(self) -> Optional[str]:
        if self.crumb:
            return self.crumb
        try:
            try:
                self.opener.open(urllib.request.Request("https://fc.yahoo.com", headers=self.UA), timeout=8)
            except Exception:
                pass
            with self.opener.open(urllib.request.Request("https://query2.finance.yahoo.com/v1/test/getcrumb", headers=self.UA), timeout=8) as r:
                self.crumb = r.read().decode("utf-8").strip()
        except Exception as e:  # noqa: BLE001 - network failure is reported as "no data"
            logger.warning("could not obtain Yahoo crumb: %s", e)
        return self.crumb

    def summary(self, ticker: str) -> Dict[str, Any]:
        crumb = self._ensure_crumb()
        if not crumb:
            return {}
        modules = "financialData,defaultKeyStatistics,summaryDetail,incomeStatementHistory,assetProfile,price"
        url = (f"https://query2.finance.yahoo.com/v10/finance/quoteSummary/{ticker}.NS?modules={modules}"
               f"&crumb={urllib.parse.quote(crumb)}")
        try:
            res = self._get(url).get("quoteSummary", {}).get("result")
            return res[0] if res else {}
        except Exception as e:  # noqa: BLE001
            logger.warning("quoteSummary failed for %s: %s", ticker, e)
            return {}

    def annual(self, ticker: str) -> Dict[str, Dict[str, Any]]:
        """Latest annual value of each statement item: {item: {"value": float, "as_of": "YYYY-MM-DD"}}."""
        url = ("https://query1.finance.yahoo.com/ws/fundamentals-timeseries/v1/finance/timeseries/"
               f"{ticker}.NS?symbol={ticker}.NS&type={','.join(TIMESERIES_TYPES)}&merge=false"
               f"&period1=1577836800&period2={int(datetime.now(timezone.utc).timestamp()) + 86400}")
        out: Dict[str, Dict[str, Any]] = {}
        try:
            for r in self._get(url).get("timeseries", {}).get("result", []):
                t = r.get("meta", {}).get("type", [None])[0]
                rows = [x for x in (r.get(t) or []) if x and x.get("reportedValue", {}).get("raw") is not None]
                if t and rows:
                    last = max(rows, key=lambda x: x["asOfDate"])
                    out[t.replace("annual", "", 1)] = {"value": float(last["reportedValue"]["raw"]), "as_of": last["asOfDate"]}
        except Exception as e:  # noqa: BLE001
            logger.warning("fundamentals-timeseries failed for %s: %s", ticker, e)
        return out


def _raw(obj: Optional[Dict[str, Any]], key: str) -> Optional[float]:
    v = (obj or {}).get(key)
    if isinstance(v, dict):
        v = v.get("raw")
    try:
        return float(v) if v is not None else None
    except (TypeError, ValueError):
        return None


def load_rules(cur) -> Dict[str, float]:
    cur.execute("SELECT rules FROM shariah_rule_versions WHERE rule_set_id = 1 AND version = 1")
    row = cur.fetchone()
    return {k: float(v) for k, v in (row[0] if row else {}).items() if k in DEFAULT_RULES}


def screen_and_store(cur, client: YahooFundamentals, symbol_id: int, symbol: str, rules: Dict[str, float]) -> Dict[str, Any]:
    """Fetch the figures, evaluate, and write the screening (plus profile and statement values). Returns the evaluation."""
    summary = client.summary(symbol)
    annual = client.annual(symbol)
    if not summary and not annual:
        ev = {"status": "DATA_INSUFFICIENT", "reason": "No financial data available from the vendor for this symbol",
              "business": {"pass": None, "note": "not assessed"}, "missing_fields": ["all"], "stale": True,
              "ratios": {}, "flags": {}, "limits": rules}
        _store(cur, symbol_id, ev, {}, None, "PROVIDER_DATA_UNAVAILABLE")
        return ev

    fd, sd, profile = summary.get("financialData", {}), summary.get("summaryDetail", {}), summary.get("assetProfile", {})
    price = summary.get("price", {})
    industry, sector = profile.get("industry"), profile.get("sector")
    market_cap = _raw(sd, "marketCap") or _raw(price, "marketCap")
    total_debt = _raw(fd, "totalDebt") if _raw(fd, "totalDebt") is not None else (annual.get("TotalDebt") or {}).get("value")
    total_cash = _raw(fd, "totalCash") if _raw(fd, "totalCash") is not None else (annual.get("CashCashEquivalentsAndShortTermInvestments") or {}).get("value")
    revenue = (annual.get("TotalRevenue") or {}).get("value") or _raw(fd, "totalRevenue")
    interest = (annual.get("InterestIncome") or annual.get("InterestIncomeNonOperating") or {}).get("value")
    receivables = (annual.get("AccountsReceivable") or {}).get("value")
    as_of = (annual.get("TotalRevenue") or annual.get("TotalAssets") or {}).get("as_of")
    period_end = date.fromisoformat(as_of) if as_of else None

    ev = evaluate(symbol, industry, market_cap, total_debt, total_cash, revenue, interest, receivables, period_end, rules=rules)
    facts = {
        "market_cap": market_cap, "total_debt": total_debt, "total_cash": total_cash, "total_revenue": revenue,
        "interest_income": interest, "total_receivables": receivables,
        "total_assets": (annual.get("TotalAssets") or {}).get("value"),
        "sector": sector, "industry": industry, "name": price.get("longName") or price.get("shortName"),
    }
    _store(cur, symbol_id, ev, facts, period_end, SOURCE)
    return ev


def _store(cur, symbol_id: int, ev: Dict[str, Any], facts: Dict[str, Any], period_end: Optional[date], source: str) -> None:
    ratios, flags = ev.get("ratios", {}), ev.get("flags", {})
    details = {
        "evaluation_policy": "P1.9.25_REPORTED_FIGURES_V2", "shariah_standard": "AAOIFI_SHARIAH_COMPLIANCE_STANDARD_NO_21",
        "reason": ev["reason"], "missing_fields": ev["missing_fields"], "stale": bool(ev.get("stale")), "currency": "INR", "unit": "RAW_INR",
        "as_of_period": str(period_end) if period_end else None, "reported": facts, "limits": ev.get("limits"),
        "verification_status": "UNVERIFIED_THIRD_PARTY_AGGREGATOR",
        "uncertainty_reason": ("Figures are a third-party aggregation of annual filings (Yahoo Finance). They are not audited "
                               "statutory notes: confirm against the company's filings and your own scholar before relying on a PASS."),
        "derivation_formulas": {
            "debt_to_market_cap": "total_debt / market_cap", "cash_to_market_cap": "total_cash / market_cap",
            "interest_to_revenue": "interest_income / total_revenue", "receivables_to_market_cap": "total_receivables / market_cap",
        },
    }
    if facts.get("sector") or facts.get("industry"):
        cur.execute("UPDATE symbols SET sector = COALESCE(%s, sector), industry = COALESCE(%s, industry), updated_at = NOW() WHERE id = %s",
                    (facts.get("sector"), facts.get("industry"), symbol_id))
    if period_end and facts.get("total_revenue") is not None:
        cur.execute("""
            INSERT INTO financial_statements (symbol_id, period_type, period_end, publication_date, pit_mode, fiscal_year,
                revenue, total_assets, total_debt, cash_and_equivalents, interest_income, total_receivables, source,
                quality_status, retrieved_at, currency, unit, filing_reference, provenance)
            VALUES (%s, 'ANNUAL', %s, %s, 'PIT_REPORTED', %s, %s, %s, %s, %s, %s, %s, 'yahoo_fundamentals_timeseries',
                'VALID', NOW(), 'INR', 'RAW_INR', 'YAHOO_FINANCE_AGGREGATED_FEED_UNVERIFIED', %s)
            ON CONFLICT (symbol_id, period_type, period_end) DO UPDATE SET
                revenue = EXCLUDED.revenue, total_assets = EXCLUDED.total_assets, total_debt = EXCLUDED.total_debt,
                cash_and_equivalents = EXCLUDED.cash_and_equivalents, interest_income = EXCLUDED.interest_income,
                total_receivables = EXCLUDED.total_receivables, provenance = EXCLUDED.provenance, retrieved_at = NOW()
        """, (symbol_id, period_end, period_end, period_end.year, facts.get("total_revenue"), facts.get("total_assets"),
              facts.get("total_debt"), facts.get("total_cash"), facts.get("interest_income"), facts.get("total_receivables"),
              json.dumps({"source_provider": "yahoo_finance", "details": details})))
    cur.execute("""
        INSERT INTO shariah_screenings (symbol_id, rule_set_id, rule_version_id, screening_date, status,
            debt_ratio, debt_ratio_pass, interest_income_ratio, interest_income_pass, cash_deposit_ratio, cash_deposit_pass,
            receivables_ratio, receivables_pass, business_activity_pass, business_activity_notes,
            purification_per_share, purification_method, details, source, screened_at)
        VALUES (%s, 1, 1, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, NULL, 'dividend_based', %s, %s, NOW())
        ON CONFLICT (symbol_id, rule_set_id, rule_version_id, screening_date) DO UPDATE SET
            status = EXCLUDED.status, debt_ratio = EXCLUDED.debt_ratio, debt_ratio_pass = EXCLUDED.debt_ratio_pass,
            interest_income_ratio = EXCLUDED.interest_income_ratio, interest_income_pass = EXCLUDED.interest_income_pass,
            cash_deposit_ratio = EXCLUDED.cash_deposit_ratio, cash_deposit_pass = EXCLUDED.cash_deposit_pass,
            receivables_ratio = EXCLUDED.receivables_ratio, receivables_pass = EXCLUDED.receivables_pass,
            business_activity_pass = EXCLUDED.business_activity_pass, business_activity_notes = EXCLUDED.business_activity_notes,
            details = EXCLUDED.details, source = EXCLUDED.source, screened_at = NOW()
    """, (symbol_id, date.today(), ev["status"],
          ratios.get("debt"), flags.get("debt"), ratios.get("interest"), flags.get("interest"),
          ratios.get("cash"), flags.get("cash"), ratios.get("receivables"), flags.get("recv"),
          ev["business"]["pass"], ev["business"]["note"], json.dumps(details), source))
