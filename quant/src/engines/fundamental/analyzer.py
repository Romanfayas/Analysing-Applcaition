"""
Fundamental Analysis Engine

Calculates fundamental metrics from financial statements.
All calculations are deterministic and documented with formulas.

Key metrics:
- Profitability: ROE, ROCE, ROA, Net Margin, Operating Margin
- Growth: Revenue Growth, EPS Growth, PAT Growth
- Efficiency: Asset Turnover, Inventory Turnover, Receivables Turnover
- Leverage: Debt/Equity, Interest Coverage, Current Ratio
- Valuation: PE, PB, PS, EV/EBITDA, Dividend Yield, PEG
- Quality: Piotroski F-Score, Altman Z-Score
"""

from dataclasses import dataclass, field
from typing import Optional
import numpy as np


@dataclass
class FinancialData:
    """Input financial data from balance sheet, P&L, and cash flow."""
    # Balance Sheet
    total_assets: Optional[float] = None
    total_equity: Optional[float] = None
    total_debt: Optional[float] = None
    current_assets: Optional[float] = None
    current_liabilities: Optional[float] = None
    cash_and_equivalents: Optional[float] = None
    inventory: Optional[float] = None
    receivables: Optional[float] = None
    fixed_assets: Optional[float] = None
    total_shares_outstanding: Optional[float] = None

    # Profit & Loss
    revenue: Optional[float] = None
    operating_profit: Optional[float] = None
    net_profit: Optional[float] = None
    ebitda: Optional[float] = None
    interest_expense: Optional[float] = None
    depreciation: Optional[float] = None
    tax_expense: Optional[float] = None
    eps: Optional[float] = None
    dividend_per_share: Optional[float] = None

    # Cash Flow
    operating_cash_flow: Optional[float] = None
    capex: Optional[float] = None
    free_cash_flow: Optional[float] = None

    # Market Data
    market_cap: Optional[float] = None
    current_price: Optional[float] = None
    enterprise_value: Optional[float] = None

    # Previous Period (for growth calculations)
    prev_revenue: Optional[float] = None
    prev_net_profit: Optional[float] = None
    prev_eps: Optional[float] = None
    prev_total_assets: Optional[float] = None
    prev_current_assets: Optional[float] = None
    prev_current_liabilities: Optional[float] = None
    prev_total_debt: Optional[float] = None
    prev_total_equity: Optional[float] = None
    prev_operating_cash_flow: Optional[float] = None
    prev_gross_margin: Optional[float] = None
    prev_asset_turnover: Optional[float] = None
    prev_shares_outstanding: Optional[float] = None

    # Projections (for PEG)
    eps_growth_estimate: Optional[float] = None


@dataclass
class MetricResult:
    """A calculated fundamental metric."""
    name: str
    value: Optional[float]
    category: str  # profitability, growth, efficiency, leverage, valuation, quality
    rating: Optional[str] = None  # EXCELLENT, GOOD, FAIR, POOR
    formula: str = ""
    note: str = ""


class FundamentalAnalyzer:
    """
    Calculates all fundamental metrics from financial data.
    Missing values are None, never silently filled with zero.
    """

    def analyze(self, data: FinancialData) -> list[MetricResult]:
        """Run all fundamental calculations and return results."""
        results = []

        # Profitability
        results.append(self._calc_roe(data))
        results.append(self._calc_roce(data))
        results.append(self._calc_roa(data))
        results.append(self._calc_net_margin(data))
        results.append(self._calc_operating_margin(data))

        # Growth
        results.append(self._calc_revenue_growth(data))
        results.append(self._calc_eps_growth(data))
        results.append(self._calc_pat_growth(data))

        # Efficiency
        results.append(self._calc_asset_turnover(data))

        # Leverage
        results.append(self._calc_debt_equity(data))
        results.append(self._calc_interest_coverage(data))
        results.append(self._calc_current_ratio(data))

        # Valuation
        results.append(self._calc_pe(data))
        results.append(self._calc_pb(data))
        results.append(self._calc_ps(data))
        results.append(self._calc_ev_ebitda(data))
        results.append(self._calc_dividend_yield(data))
        results.append(self._calc_peg(data))

        # Quality
        results.append(self._calc_piotroski(data))

        return results

    def calculate_score(self, metrics: list[MetricResult]) -> float:
        """Calculate overall fundamental quality score (0-100)."""
        scored_metrics = [m for m in metrics if m.rating is not None]
        if not scored_metrics:
            return 0.0

        rating_scores = {"EXCELLENT": 100, "GOOD": 75, "FAIR": 50, "POOR": 25}
        total = sum(rating_scores.get(m.rating, 0) for m in scored_metrics)
        return round(total / len(scored_metrics), 2)

    # ==================================================
    # Profitability Metrics
    # ==================================================

    def _calc_roe(self, d: FinancialData) -> MetricResult:
        """ROE = Net Profit / Total Equity × 100"""
        val = self._safe_ratio(d.net_profit, d.total_equity, pct=True)
        rating = self._rate(val, [(20, "EXCELLENT"), (15, "GOOD"), (10, "FAIR")])
        return MetricResult("roe", val, "profitability", rating,
                            "Net Profit / Total Equity × 100")

    def _calc_roce(self, d: FinancialData) -> MetricResult:
        """ROCE = EBIT / Capital Employed × 100"""
        ebit = None
        if d.operating_profit is not None:
            ebit = d.operating_profit
        capital_employed = None
        if d.total_assets is not None and d.current_liabilities is not None:
            capital_employed = d.total_assets - d.current_liabilities
        val = self._safe_ratio(ebit, capital_employed, pct=True)
        rating = self._rate(val, [(20, "EXCELLENT"), (15, "GOOD"), (10, "FAIR")])
        return MetricResult("roce", val, "profitability", rating,
                            "EBIT / (Total Assets - Current Liabilities) × 100")

    def _calc_roa(self, d: FinancialData) -> MetricResult:
        """ROA = Net Profit / Total Assets × 100"""
        val = self._safe_ratio(d.net_profit, d.total_assets, pct=True)
        rating = self._rate(val, [(10, "EXCELLENT"), (5, "GOOD"), (2, "FAIR")])
        return MetricResult("roa", val, "profitability", rating,
                            "Net Profit / Total Assets × 100")

    def _calc_net_margin(self, d: FinancialData) -> MetricResult:
        """Net Margin = Net Profit / Revenue × 100"""
        val = self._safe_ratio(d.net_profit, d.revenue, pct=True)
        rating = self._rate(val, [(20, "EXCELLENT"), (10, "GOOD"), (5, "FAIR")])
        return MetricResult("net_margin", val, "profitability", rating,
                            "Net Profit / Revenue × 100")

    def _calc_operating_margin(self, d: FinancialData) -> MetricResult:
        """Operating Margin = Operating Profit / Revenue × 100"""
        val = self._safe_ratio(d.operating_profit, d.revenue, pct=True)
        rating = self._rate(val, [(25, "EXCELLENT"), (15, "GOOD"), (8, "FAIR")])
        return MetricResult("operating_margin", val, "profitability", rating,
                            "Operating Profit / Revenue × 100")

    # ==================================================
    # Growth Metrics
    # ==================================================

    def _calc_revenue_growth(self, d: FinancialData) -> MetricResult:
        """Revenue Growth = (Revenue - PrevRevenue) / PrevRevenue × 100"""
        val = self._safe_growth(d.revenue, d.prev_revenue)
        rating = self._rate(val, [(20, "EXCELLENT"), (10, "GOOD"), (5, "FAIR")])
        return MetricResult("revenue_growth", val, "growth", rating,
                            "(Revenue - Prev Revenue) / Prev Revenue × 100")

    def _calc_eps_growth(self, d: FinancialData) -> MetricResult:
        """EPS Growth = (EPS - PrevEPS) / PrevEPS × 100"""
        val = self._safe_growth(d.eps, d.prev_eps)
        rating = self._rate(val, [(25, "EXCELLENT"), (15, "GOOD"), (5, "FAIR")])
        return MetricResult("eps_growth", val, "growth", rating,
                            "(EPS - Prev EPS) / Prev EPS × 100")

    def _calc_pat_growth(self, d: FinancialData) -> MetricResult:
        """PAT Growth = (Net Profit - Prev Net Profit) / Prev Net Profit × 100"""
        val = self._safe_growth(d.net_profit, d.prev_net_profit)
        rating = self._rate(val, [(20, "EXCELLENT"), (10, "GOOD"), (5, "FAIR")])
        return MetricResult("pat_growth", val, "growth", rating,
                            "(PAT - Prev PAT) / Prev PAT × 100")

    # ==================================================
    # Efficiency Metrics
    # ==================================================

    def _calc_asset_turnover(self, d: FinancialData) -> MetricResult:
        """Asset Turnover = Revenue / Total Assets"""
        val = self._safe_ratio(d.revenue, d.total_assets)
        rating = self._rate(val, [(2.0, "EXCELLENT"), (1.0, "GOOD"), (0.5, "FAIR")])
        return MetricResult("asset_turnover", val, "efficiency", rating,
                            "Revenue / Total Assets")

    # ==================================================
    # Leverage Metrics
    # ==================================================

    def _calc_debt_equity(self, d: FinancialData) -> MetricResult:
        """D/E = Total Debt / Total Equity"""
        val = self._safe_ratio(d.total_debt, d.total_equity)
        # Lower is better for D/E
        rating = None
        if val is not None:
            if val <= 0.5:
                rating = "EXCELLENT"
            elif val <= 1.0:
                rating = "GOOD"
            elif val <= 2.0:
                rating = "FAIR"
            else:
                rating = "POOR"
        return MetricResult("debt_to_equity", val, "leverage", rating,
                            "Total Debt / Total Equity")

    def _calc_interest_coverage(self, d: FinancialData) -> MetricResult:
        """Interest Coverage = EBIT / Interest Expense"""
        val = self._safe_ratio(d.operating_profit, d.interest_expense)
        rating = self._rate(val, [(5, "EXCELLENT"), (3, "GOOD"), (1.5, "FAIR")])
        return MetricResult("interest_coverage", val, "leverage", rating,
                            "Operating Profit / Interest Expense")

    def _calc_current_ratio(self, d: FinancialData) -> MetricResult:
        """Current Ratio = Current Assets / Current Liabilities"""
        val = self._safe_ratio(d.current_assets, d.current_liabilities)
        rating = None
        if val is not None:
            if 1.5 <= val <= 3.0:
                rating = "EXCELLENT"
            elif 1.0 <= val < 1.5:
                rating = "GOOD"
            elif val > 3.0:
                rating = "FAIR"  # Excess liquidity
            else:
                rating = "POOR"
        return MetricResult("current_ratio", val, "leverage", rating,
                            "Current Assets / Current Liabilities")

    # ==================================================
    # Valuation Metrics
    # ==================================================

    def _calc_pe(self, d: FinancialData) -> MetricResult:
        """PE = Market Price / EPS"""
        val = self._safe_ratio(d.current_price, d.eps)
        rating = None
        if val is not None:
            if val > 0:
                if val <= 15:
                    rating = "EXCELLENT"
                elif val <= 25:
                    rating = "GOOD"
                elif val <= 40:
                    rating = "FAIR"
                else:
                    rating = "POOR"
        return MetricResult("pe_ratio", val, "valuation", rating,
                            "Market Price / EPS")

    def _calc_pb(self, d: FinancialData) -> MetricResult:
        """PB = Market Cap / Book Value (Total Equity)"""
        val = self._safe_ratio(d.market_cap, d.total_equity)
        rating = None
        if val is not None and val > 0:
            if val <= 1.5:
                rating = "EXCELLENT"
            elif val <= 3.0:
                rating = "GOOD"
            elif val <= 5.0:
                rating = "FAIR"
            else:
                rating = "POOR"
        return MetricResult("pb_ratio", val, "valuation", rating,
                            "Market Cap / Total Equity")

    def _calc_ps(self, d: FinancialData) -> MetricResult:
        """PS = Market Cap / Revenue"""
        val = self._safe_ratio(d.market_cap, d.revenue)
        return MetricResult("ps_ratio", val, "valuation", None,
                            "Market Cap / Revenue")

    def _calc_ev_ebitda(self, d: FinancialData) -> MetricResult:
        """EV/EBITDA = Enterprise Value / EBITDA"""
        val = self._safe_ratio(d.enterprise_value, d.ebitda)
        rating = None
        if val is not None and val > 0:
            if val <= 10:
                rating = "EXCELLENT"
            elif val <= 15:
                rating = "GOOD"
            elif val <= 25:
                rating = "FAIR"
            else:
                rating = "POOR"
        return MetricResult("ev_ebitda", val, "valuation", rating,
                            "Enterprise Value / EBITDA")

    def _calc_dividend_yield(self, d: FinancialData) -> MetricResult:
        """Dividend Yield = DPS / Price × 100"""
        val = self._safe_ratio(d.dividend_per_share, d.current_price, pct=True)
        rating = self._rate(val, [(4, "EXCELLENT"), (2, "GOOD"), (1, "FAIR")])
        return MetricResult("dividend_yield", val, "valuation", rating,
                            "Dividend Per Share / Market Price × 100")

    def _calc_peg(self, d: FinancialData) -> MetricResult:
        """PEG = PE / EPS Growth Rate"""
        pe = self._safe_ratio(d.current_price, d.eps)
        growth = d.eps_growth_estimate
        if pe is None or growth is None or growth <= 0:
            return MetricResult("peg_ratio", None, "valuation", None,
                                "PE Ratio / EPS Growth Rate")
        val = round(pe / growth, 4)
        rating = None
        if val <= 1.0:
            rating = "EXCELLENT"
        elif val <= 1.5:
            rating = "GOOD"
        elif val <= 2.0:
            rating = "FAIR"
        else:
            rating = "POOR"
        return MetricResult("peg_ratio", val, "valuation", rating,
                            "PE Ratio / EPS Growth Rate")

    # ==================================================
    # Quality Metrics
    # ==================================================

    def _calc_piotroski(self, d: FinancialData) -> MetricResult:
        """
        Piotroski F-Score (0-9 points):
        Profitability (4): ROA>0, OCF>0, ΔROA>0, Accruals
        Leverage (3): ΔLeverage<0, ΔCurrent Ratio>0, No dilution
        Efficiency (2): ΔGross Margin>0, ΔAsset Turnover>0
        """
        score = 0

        # 1. ROA > 0
        if d.net_profit is not None and d.total_assets is not None:
            if d.total_assets > 0 and d.net_profit / d.total_assets > 0:
                score += 1

        # 2. Operating Cash Flow > 0
        if d.operating_cash_flow is not None and d.operating_cash_flow > 0:
            score += 1

        # 3. ΔROA > 0
        roa_curr = self._safe_ratio(d.net_profit, d.total_assets)
        roa_prev = self._safe_ratio(d.prev_net_profit, d.prev_total_assets)
        if roa_curr is not None and roa_prev is not None and roa_curr > roa_prev:
            score += 1

        # 4. Accruals (OCF > Net Profit, quality of earnings)
        if d.operating_cash_flow is not None and d.net_profit is not None:
            if d.operating_cash_flow > d.net_profit:
                score += 1

        # 5. ΔLeverage (lower is better)
        de_curr = self._safe_ratio(d.total_debt, d.total_equity)
        de_prev = self._safe_ratio(d.prev_total_debt, d.prev_total_equity)
        if de_curr is not None and de_prev is not None and de_curr < de_prev:
            score += 1

        # 6. ΔCurrent Ratio > 0
        cr_curr = self._safe_ratio(d.current_assets, d.current_liabilities)
        cr_prev = self._safe_ratio(d.prev_current_assets, d.prev_current_liabilities)
        if cr_curr is not None and cr_prev is not None and cr_curr > cr_prev:
            score += 1

        # 7. No share dilution
        if d.total_shares_outstanding is not None and d.prev_shares_outstanding is not None:
            if d.total_shares_outstanding <= d.prev_shares_outstanding:
                score += 1

        # 8. ΔGross Margin > 0 (using operating margin as proxy)
        gm_curr = self._safe_ratio(d.operating_profit, d.revenue, pct=True)
        if gm_curr is not None and d.prev_gross_margin is not None:
            if gm_curr > d.prev_gross_margin:
                score += 1

        # 9. ΔAsset Turnover > 0
        at_curr = self._safe_ratio(d.revenue, d.total_assets)
        if at_curr is not None and d.prev_asset_turnover is not None:
            if at_curr > d.prev_asset_turnover:
                score += 1

        rating = None
        if score >= 7:
            rating = "EXCELLENT"
        elif score >= 5:
            rating = "GOOD"
        elif score >= 3:
            rating = "FAIR"
        else:
            rating = "POOR"

        return MetricResult("piotroski_f_score", float(score), "quality", rating,
                            "9-point quality score (Profitability + Leverage + Efficiency)")

    # ==================================================
    # Helpers
    # ==================================================

    def _safe_ratio(
        self, numerator: Optional[float], denominator: Optional[float], pct: bool = False
    ) -> Optional[float]:
        """Safely calculate a ratio. Returns None if data is missing."""
        if numerator is None or denominator is None or denominator == 0:
            return None
        ratio = numerator / denominator
        if pct:
            ratio *= 100
        return round(ratio, 4)

    def _safe_growth(
        self, current: Optional[float], previous: Optional[float]
    ) -> Optional[float]:
        """Calculate growth rate. Returns None if data is missing."""
        if current is None or previous is None or previous == 0:
            return None
        return round(((current - previous) / abs(previous)) * 100, 4)

    def _rate(
        self, value: Optional[float], thresholds: list[tuple[float, str]]
    ) -> Optional[str]:
        """Rate a metric value against descending thresholds."""
        if value is None:
            return None
        for threshold, rating in thresholds:
            if value >= threshold:
                return rating
        return "POOR"
