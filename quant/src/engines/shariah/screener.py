"""
Shariah Screening Engine

Screens stocks against configurable, versioned Shariah compliance rules.
Rules are NEVER hardcoded — loaded from database/configuration at runtime.

Screening criteria (configurable via rule sets):
- Business activity screening (prohibited sectors)
- Debt to market cap ratio
- Interest income to revenue ratio
- Cash & interest-bearing deposits to market cap ratio
- Receivables to market cap ratio
- Custom financial ratios

Possible statuses: PASS, FAIL, REVIEW_REQUIRED, DATA_INSUFFICIENT

A FAIL status is a HARD trading restriction.
The system NEVER overrides a failed screen based on technical signals.
"""

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


class ShariahStatus(Enum):
    PASS = "PASS"
    FAIL = "FAIL"
    REVIEW_REQUIRED = "REVIEW_REQUIRED"
    DATA_INSUFFICIENT = "DATA_INSUFFICIENT"


@dataclass
class ShariahRules:
    """
    Versioned Shariah screening rules loaded from configuration.
    These are NEVER hardcoded in business logic.
    """
    debt_to_market_cap_max: float = 0.33
    interest_income_to_revenue_max: float = 0.05
    cash_and_deposits_to_market_cap_max: float = 0.33
    receivables_to_market_cap_max: float = 0.49
    prohibited_business_activities: list[str] = field(default_factory=lambda: [
        "alcohol", "tobacco", "gambling", "conventional_finance",
        "conventional_insurance", "pork", "weapons",
        "adult_entertainment", "interest_based_lending",
    ])
    prohibited_revenue_threshold: float = 0.05
    purification_method: str = "dividend_based"

    @classmethod
    def from_dict(cls, data: dict) -> "ShariahRules":
        """Create rules from a dictionary (loaded from DB/config)."""
        return cls(**{k: v for k, v in data.items() if k in cls.__dataclass_fields__})


@dataclass
class RatioResult:
    """Result of a single ratio check."""
    name: str
    value: Optional[float]
    threshold: float
    passed: Optional[bool]
    note: str = ""


@dataclass
class ShariahScreeningResult:
    """Complete Shariah screening result."""
    status: ShariahStatus
    ratio_results: list[RatioResult]
    business_activity_pass: Optional[bool]
    business_activity_notes: str = ""
    purification_per_share: Optional[float] = None
    purification_method: Optional[str] = None
    details: dict = field(default_factory=dict)


class ShariahScreener:
    """
    Screens stocks against Shariah compliance rules.

    Rules are loaded from configuration — the screener does NOT contain
    hardcoded thresholds. All thresholds come from ShariahRules.

    Usage:
        rules = ShariahRules.from_dict(db_config)
        screener = ShariahScreener(rules)
        result = screener.screen(financial_data, market_cap)
    """

    def __init__(self, rules: ShariahRules):
        self.rules = rules

    def screen(
        self,
        total_debt: Optional[float],
        market_cap: Optional[float],
        interest_income: Optional[float],
        revenue: Optional[float],
        cash_and_deposits: Optional[float],
        receivables: Optional[float],
        business_activities: Optional[list[str]] = None,
        dividend_per_share: Optional[float] = None,
        interest_income_per_share: Optional[float] = None,
    ) -> ShariahScreeningResult:
        """
        Screen a stock against all Shariah criteria.

        Returns DATA_INSUFFICIENT if critical data is missing.
        Returns FAIL if any ratio exceeds its threshold.
        Returns PASS if all ratios are within thresholds.
        """
        ratio_results = []
        has_insufficient = False

        # 1. Debt to Market Cap
        debt_result = self._check_ratio(
            name="debt_to_market_cap",
            numerator=total_debt,
            denominator=market_cap,
            max_threshold=self.rules.debt_to_market_cap_max,
        )
        ratio_results.append(debt_result)
        if debt_result.passed is None:
            has_insufficient = True

        # 2. Interest Income to Revenue
        interest_result = self._check_ratio(
            name="interest_income_to_revenue",
            numerator=interest_income,
            denominator=revenue,
            max_threshold=self.rules.interest_income_to_revenue_max,
        )
        ratio_results.append(interest_result)
        if interest_result.passed is None:
            has_insufficient = True

        # 3. Cash & Deposits to Market Cap
        cash_result = self._check_ratio(
            name="cash_deposits_to_market_cap",
            numerator=cash_and_deposits,
            denominator=market_cap,
            max_threshold=self.rules.cash_and_deposits_to_market_cap_max,
        )
        ratio_results.append(cash_result)
        if cash_result.passed is None:
            has_insufficient = True

        # 4. Receivables to Market Cap
        recv_result = self._check_ratio(
            name="receivables_to_market_cap",
            numerator=receivables,
            denominator=market_cap,
            max_threshold=self.rules.receivables_to_market_cap_max,
        )
        ratio_results.append(recv_result)
        if recv_result.passed is None:
            has_insufficient = True

        # 5. Business Activity Screening
        biz_pass = None
        biz_notes = ""
        if business_activities is not None:
            prohibited_found = [
                act for act in business_activities
                if act.lower() in [p.lower() for p in self.rules.prohibited_business_activities]
            ]
            if prohibited_found:
                biz_pass = False
                biz_notes = f"Prohibited activities found: {', '.join(prohibited_found)}"
            else:
                biz_pass = True
                biz_notes = "No prohibited business activities detected"
        else:
            biz_notes = "Business activity data not available"
            has_insufficient = True

        # Determine overall status
        all_passed = all(r.passed is True for r in ratio_results) and biz_pass is True
        any_failed = any(r.passed is False for r in ratio_results) or biz_pass is False

        if any_failed:
            status = ShariahStatus.FAIL
        elif has_insufficient:
            status = ShariahStatus.DATA_INSUFFICIENT
        elif all_passed:
            status = ShariahStatus.PASS
        else:
            status = ShariahStatus.REVIEW_REQUIRED

        # Calculate purification if applicable
        purification = None
        purification_method = None
        if status == ShariahStatus.PASS and self.rules.purification_method == "dividend_based":
            purification = self._calculate_purification_dividend(
                dividend_per_share, interest_income, revenue
            )
            purification_method = "dividend_based"

        return ShariahScreeningResult(
            status=status,
            ratio_results=ratio_results,
            business_activity_pass=biz_pass,
            business_activity_notes=biz_notes,
            purification_per_share=purification,
            purification_method=purification_method,
            details={
                "rule_thresholds": {
                    "debt_to_market_cap_max": self.rules.debt_to_market_cap_max,
                    "interest_income_to_revenue_max": self.rules.interest_income_to_revenue_max,
                    "cash_deposits_to_market_cap_max": self.rules.cash_and_deposits_to_market_cap_max,
                    "receivables_to_market_cap_max": self.rules.receivables_to_market_cap_max,
                },
            },
        )

    def _check_ratio(
        self,
        name: str,
        numerator: Optional[float],
        denominator: Optional[float],
        max_threshold: float,
    ) -> RatioResult:
        """Check a single financial ratio against its threshold."""
        if numerator is None or denominator is None:
            return RatioResult(
                name=name,
                value=None,
                threshold=max_threshold,
                passed=None,
                note="Insufficient data to calculate ratio",
            )

        if denominator == 0:
            return RatioResult(
                name=name,
                value=None,
                threshold=max_threshold,
                passed=None,
                note="Denominator is zero, cannot calculate ratio",
            )

        ratio = numerator / denominator

        passed = ratio <= max_threshold
        note = f"{'PASS' if passed else 'FAIL'}: {ratio:.4f} {'<=' if passed else '>'} {max_threshold}"

        return RatioResult(
            name=name,
            value=round(ratio, 6),
            threshold=max_threshold,
            passed=passed,
            note=note,
        )

    def _calculate_purification_dividend(
        self,
        dividend_per_share: Optional[float],
        interest_income: Optional[float],
        revenue: Optional[float],
    ) -> Optional[float]:
        """
        Calculate purification amount per share using dividend-based method.

        Purification = Dividend per Share × (Interest Income / Revenue)
        """
        if dividend_per_share is None or interest_income is None or revenue is None:
            return None
        if revenue == 0:
            return None

        interest_ratio = interest_income / revenue
        return round(dividend_per_share * interest_ratio, 6)
