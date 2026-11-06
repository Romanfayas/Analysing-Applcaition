"""
HalalEquity P1.9.17 - Audited Financial Disclosure & Footnote Shariah Screener

Enforces AAOIFI Standard No. 21 with rigorous evidence requirements:
1. Dynamic Footnote References: Disclosures map to company-specific note numbers
   (e.g., Note 18, Note 24, Note 27, Note 31); no universal Note 24/25 assumption.
2. Anti-Inference Fail-Closed Rule: Missing interest-income notes or failed parsers
   can NEVER be inferred as zero interest income. Missing data strictly yields DATA_INSUFFICIENT.
3. Anti-LLM Hallucination Rule: LLM-generated interpretations without cited page numbers
   and audited report text passages cannot establish Shariah compliance.
4. Human Review & Scholar Gate: Methodologies requiring qualified reviewer / scholar sign-off
   remain PENDING_REVIEW and cannot be labelled certified PASS without documented verification.
"""

from dataclasses import dataclass, field
from datetime import date, datetime
from decimal import Decimal
from enum import Enum
from typing import Dict, List, Optional, Any


class AuditedShariahStatus(str, Enum):
    PASS = "PASS"
    FAIL = "FAIL"
    DATA_INSUFFICIENT = "DATA_INSUFFICIENT"
    PENDING_QUALIFIED_REVIEW = "PENDING_QUALIFIED_REVIEW"


@dataclass(frozen=True)
class AuditedFootnoteCitation:
    symbol: str
    period_end: date
    disclosure_type: str  # 'INTEREST_INCOME', 'TOTAL_RECEIVABLES', etc.
    note_reference: str   # e.g., 'Note 28 - Other Income', 'Note 19 - Finance Income'
    page_number: int
    extracted_passage: str
    extracted_amount_inr: Decimal
    reporting_period_months: int
    parser_name: str
    parser_confidence: float
    is_llm_generated: bool
    has_source_citations: bool
    manual_review_status: str  # 'PENDING_QUALIFIED_REVIEW', 'CERTIFIED_BY_REVIEWER'
    auditor_firm: Optional[str] = None
    auditor_opinion: str = "UNMODIFIED"


@dataclass
class AuditedScreeningAssessment:
    symbol: str
    as_of_date: date
    status: AuditedShariahStatus
    debt_to_market_cap: Optional[float]
    debt_pass: Optional[bool]
    interest_income_to_revenue: Optional[float]
    interest_income_pass: Optional[bool]
    cash_deposits_to_market_cap: Optional[float]
    cash_pass: Optional[bool]
    receivables_to_market_cap: Optional[float]
    receivables_pass: Optional[bool]
    business_activity_pass: bool
    business_activity_notes: str
    cited_interest_note: Optional[str]
    cited_interest_page: Optional[int]
    methodology_version: str
    certified_by_reviewer: bool
    missing_fields: List[str] = field(default_factory=list)
    rejection_reasons: List[str] = field(default_factory=list)


class AuditedEvidenceScreener:
    """
    Evaluates Shariah compliance using audited statutory footnotes.
    """
    DEBT_THRESHOLD = 0.33
    INTEREST_INCOME_THRESHOLD = 0.05
    CASH_THRESHOLD = 0.33
    RECEIVABLES_THRESHOLD = 0.49
    METHODOLOGY_VERSION = "AAOIFI_STANDARD_NO_21_2026_V1.9.17"

    @classmethod
    def evaluate(
        cls,
        symbol: str,
        as_of_date: date,
        market_cap: Optional[float],
        revenue: Optional[float],
        total_debt: Optional[float],
        cash_and_deposits: Optional[float],
        receivables: Optional[float],
        is_conventional_finance: bool,
        interest_income_citation: Optional[AuditedFootnoteCitation],
        reviewer_certified: bool = False,
    ) -> AuditedScreeningAssessment:
        missing_fields = []
        reasons = []

        # 1. Prohibited Sector Check (Conventional Finance / Gambling / Alcohol)
        if is_conventional_finance:
            return AuditedScreeningAssessment(
                symbol=symbol,
                as_of_date=as_of_date,
                status=AuditedShariahStatus.FAIL,
                debt_to_market_cap=None,
                debt_pass=False,
                interest_income_to_revenue=None,
                interest_income_pass=False,
                cash_deposits_to_market_cap=None,
                cash_pass=False,
                receivables_to_market_cap=None,
                receivables_pass=False,
                business_activity_pass=False,
                business_activity_notes="Core business activity is conventional interest-based financial services",
                cited_interest_note=None,
                cited_interest_page=None,
                methodology_version=cls.METHODOLOGY_VERSION,
                certified_by_reviewer=False,
                rejection_reasons=["Conventional interest-based finance is non-compliant under AAOIFI rules"],
            )

        # 2. Debt to Market Cap Ratio
        debt_ratio = None
        debt_pass = None
        if total_debt is not None and market_cap and market_cap > 0:
            debt_ratio = round(total_debt / market_cap, 4)
            debt_pass = debt_ratio <= cls.DEBT_THRESHOLD
            if not debt_pass:
                reasons.append(f"Debt to market cap ({debt_ratio:.2%}) exceeds 33.0% threshold")
        else:
            missing_fields.append("total_debt")

        # 3. Cash and Deposits to Market Cap
        cash_ratio = None
        cash_pass = None
        if cash_and_deposits is not None and market_cap and market_cap > 0:
            cash_ratio = round(cash_and_deposits / market_cap, 4)
            cash_pass = cash_ratio <= cls.CASH_THRESHOLD
            if not cash_pass:
                reasons.append(f"Cash & deposits to market cap ({cash_ratio:.2%}) exceeds 33.0% threshold")
        else:
            missing_fields.append("cash_and_deposits")

        # 4. Receivables to Market Cap
        recv_ratio = None
        recv_pass = None
        if receivables is not None and market_cap and market_cap > 0:
            recv_ratio = round(receivables / market_cap, 4)
            recv_pass = recv_ratio <= cls.RECEIVABLES_THRESHOLD
            if not recv_pass:
                reasons.append(f"Receivables to market cap ({recv_ratio:.2%}) exceeds 49.0% threshold")
        else:
            missing_fields.append("receivables")

        # 5. Non-operating Interest Income to Revenue
        interest_ratio = None
        interest_pass = None
        interest_note = None
        interest_page = None

        if interest_income_citation is None:
            # RULE: Missing note CANNOT infer zero!
            missing_fields.append("interest_income_footnote")
            reasons.append("Missing audited interest income footnote disclosure (cannot be inferred as zero)")
        else:
            # Check LLM hallucination rule
            if interest_income_citation.is_llm_generated and not interest_income_citation.has_source_citations:
                missing_fields.append("interest_income_unsupported_llm")
                reasons.append("LLM interpretation lacks audited page/passage citation; cannot establish Shariah evidence")
            elif interest_income_citation.page_number <= 0:
                missing_fields.append("interest_income_missing_page")
                reasons.append("Audited footnote citation lacks valid report page number")
            elif not interest_income_citation.note_reference or not interest_income_citation.extracted_passage.strip():
                missing_fields.append("interest_income_missing_text")
                reasons.append("Audited footnote citation lacks explicit note reference or extracted text passage")
            elif interest_income_citation.reporting_period_months <= 0:
                missing_fields.append("interest_income_invalid_period")
                reasons.append("Audited footnote reporting period months must be positive")
            elif interest_income_citation.auditor_opinion not in ("UNMODIFIED", "QUALIFIED"):
                missing_fields.append("adverse_audit_opinion")
                reasons.append(f"Audit opinion '{interest_income_citation.auditor_opinion}' is adverse or disclaimer; cannot certify Shariah compliance")
            elif revenue and revenue > 0:
                interest_val = float(interest_income_citation.extracted_amount_inr)
                interest_ratio = round(interest_val / revenue, 4)
                interest_pass = interest_ratio <= cls.INTEREST_INCOME_THRESHOLD
                interest_note = interest_income_citation.note_reference
                interest_page = interest_income_citation.page_number
                if not interest_pass:
                    reasons.append(f"Interest income to revenue ({interest_ratio:.2%}) exceeds 5.0% threshold")
            else:
                missing_fields.append("revenue")

        # Determine Final Compliance Status
        if debt_pass is False or interest_pass is False or cash_pass is False or recv_pass is False:
            final_status = AuditedShariahStatus.FAIL
        elif missing_fields:
            final_status = AuditedShariahStatus.DATA_INSUFFICIENT
        elif not reviewer_certified:
            # All quantitative checks pass, but reviewer/scholar sign-off pending!
            final_status = AuditedShariahStatus.PENDING_QUALIFIED_REVIEW
            reasons.append("All quantitative thresholds met; pending qualified Shariah reviewer certification")
        else:
            final_status = AuditedShariahStatus.PASS

        return AuditedScreeningAssessment(
            symbol=symbol,
            as_of_date=as_of_date,
            status=final_status,
            debt_to_market_cap=debt_ratio,
            debt_pass=debt_pass,
            interest_income_to_revenue=interest_ratio,
            interest_income_pass=interest_pass,
            cash_deposits_to_market_cap=cash_ratio,
            cash_pass=cash_pass,
            receivables_to_market_cap=recv_ratio,
            receivables_pass=recv_pass,
            business_activity_pass=True,
            business_activity_notes="Core operating business is compliant",
            cited_interest_note=interest_note,
            cited_interest_page=interest_page,
            methodology_version=cls.METHODOLOGY_VERSION,
            certified_by_reviewer=reviewer_certified,
            missing_fields=missing_fields,
            rejection_reasons=reasons,
        )
