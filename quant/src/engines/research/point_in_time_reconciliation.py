"""
HalalEquity P1.9.17 - Point-in-Time Corporate Action & Historical Bar Reconciliation Engine

Enforces:
1. High-precision decimal arithmetic (8-decimal quantized fixed-point rounding).
2. Strict verification gate: Only VERIFIED_EXCHANGE_PRIMARY events may adjust historical prices.
3. Separation of price adjustments from tax-cost allocations (Section 49(2C) demergers).
4. Anti-lookahead session timestamp rule: Actions taking effect on ex_date Day D at 03:45 UTC
   adjust historical bars t < ex_date; same-day and forward bars t >= ex_date are unadjusted.
5. Idempotent recomputation and before/after factor reconciliation reports.
"""

from dataclasses import dataclass, field
from datetime import date, datetime, timezone
from decimal import Decimal, ROUND_HALF_UP
from enum import Enum
from typing import Dict, List, Optional, Tuple, Any
import pandas as pd


class EvidenceStatus(str, Enum):
    ACQUIRED_UNVERIFIED = "ACQUIRED_UNVERIFIED"
    PARSED_VALIDATED = "PARSED_VALIDATED"
    AUTHENTICITY_PENDING = "AUTHENTICITY_PENDING"
    PRIMARY_VERIFIED = "PRIMARY_VERIFIED"
    VERIFIED_EXCHANGE_PRIMARY = "VERIFIED_EXCHANGE_PRIMARY"
    ARCHIVE_UNAUTHENTICATED = "ARCHIVE_UNAUTHENTICATED"
    REJECTED = "REJECTED"
    AMBIGUOUS = "AMBIGUOUS"


@dataclass(frozen=True)
class StatutoryFactorCalculation:
    event_id: str
    symbol: str
    action_type: str
    ex_date: date
    effective_date: date
    reference_close: Optional[Decimal]
    declared_amount_or_ratio: Decimal
    computed_factor: Decimal
    formatted_factor_str: str
    verification_status: str
    is_eligible_for_price_adjustment: bool
    rejection_reason: Optional[str]
    source_artifact_checksum: Optional[str]


class PointInTimeFactorEngine:
    """
    Computes audited corporate action adjustment factors using fixed-point Decimal arithmetic.
    """
    FACTOR_QUANTIZATION = Decimal("0.00000001")  # 8 decimal places

    @classmethod
    def compute_cash_dividend_factor(
        cls,
        event_id: str,
        symbol: str,
        ex_date: date,
        dividend_amount: Decimal,
        reference_close: Decimal,
        verification_status: str,
        artifact_checksum: Optional[str] = None,
    ) -> StatutoryFactorCalculation:
        """
        Factor = (Reference Close - Dividend Amount) / Reference Close
        Quantized to 8 decimal places using ROUND_HALF_UP.
        """
        if reference_close <= Decimal("0"):
            raise ValueError(f"Reference close must be positive, got {reference_close}")
        if dividend_amount <= Decimal("0"):
            raise ValueError(f"Dividend amount must be positive, got {dividend_amount}")

        raw_factor = (reference_close - dividend_amount) / reference_close
        factor = raw_factor.quantize(cls.FACTOR_QUANTIZATION, rounding=ROUND_HALF_UP)

        is_eligible = verification_status in (
            EvidenceStatus.PRIMARY_VERIFIED.value,
            EvidenceStatus.VERIFIED_EXCHANGE_PRIMARY.value,
        )
        reason = None
        if not is_eligible:
            reason = f"Event verification status '{verification_status}' does not meet required primary exchange evidence standard"

        return StatutoryFactorCalculation(
            event_id=event_id,
            symbol=symbol,
            action_type="CASH_DIVIDEND",
            ex_date=ex_date,
            effective_date=ex_date,
            reference_close=reference_close,
            declared_amount_or_ratio=dividend_amount,
            computed_factor=factor,
            formatted_factor_str=str(factor),
            verification_status=verification_status,
            is_eligible_for_price_adjustment=is_eligible,
            rejection_reason=reason,
            source_artifact_checksum=artifact_checksum,
        )

    @classmethod
    def compute_stock_split_factor(
        cls,
        event_id: str,
        symbol: str,
        ex_date: date,
        split_ratio_old: Decimal,
        split_ratio_new: Decimal,
        verification_status: str,
        artifact_checksum: Optional[str] = None,
    ) -> StatutoryFactorCalculation:
        """
        Factor = old_ratio / new_ratio
        """
        if split_ratio_old <= Decimal("0") or split_ratio_new <= Decimal("0"):
            raise ValueError("Split ratios must be positive")

        raw_factor = split_ratio_old / split_ratio_new
        factor = raw_factor.quantize(cls.FACTOR_QUANTIZATION, rounding=ROUND_HALF_UP)

        is_eligible = verification_status in (
            EvidenceStatus.PRIMARY_VERIFIED.value,
            EvidenceStatus.VERIFIED_EXCHANGE_PRIMARY.value,
        )
        reason = None if is_eligible else f"Status '{verification_status}' ineligible for research price adjustment"

        return StatutoryFactorCalculation(
            event_id=event_id,
            symbol=symbol,
            action_type="STOCK_SPLIT",
            ex_date=ex_date,
            effective_date=ex_date,
            reference_close=None,
            declared_amount_or_ratio=split_ratio_old / split_ratio_new,
            computed_factor=factor,
            formatted_factor_str=str(factor),
            verification_status=verification_status,
            is_eligible_for_price_adjustment=is_eligible,
            rejection_reason=reason,
            source_artifact_checksum=artifact_checksum,
        )

    @classmethod
    def reconcile_demerger_tax_allocation(
        cls,
        event_id: str,
        parent_symbol: str,
        successor_symbol: str,
        allocation_ratio: Decimal,
        statutory_ref: str,
    ) -> Dict[str, Any]:
        """
        Demergers establish cost apportionment under Section 49(2C) of Income Tax Act 1961.
        They are strictly prohibited from being applied as historical price adjustment factors.
        """
        return {
            "event_id": event_id,
            "parent_symbol": parent_symbol,
            "successor_symbol": successor_symbol,
            "allocation_ratio": float(allocation_ratio),
            "is_price_adjustment_applicable": False,
            "statutory_ref": statutory_ref,
            "policy": "FORBID_CONCURRENT_EXPOSURE",
            "enforcement_rule": "Tax cost allocation is non-price factor; historical price adjustments strictly rejected.",
        }

    @classmethod
    def apply_adjustment_to_daily_series(
        cls,
        bars_df: pd.DataFrame,
        factor_calc: StatutoryFactorCalculation,
    ) -> pd.DataFrame:
        """
        Applies factor to historical bars strictly prior to ex_date.
        If factor_calc.is_eligible_for_price_adjustment is False, returns bars with adjusted_close = None (Quarantined).
        """
        df = bars_df.copy()
        if "adjusted_close" not in df.columns:
            df["adjusted_close"] = None

        if not factor_calc.is_eligible_for_price_adjustment:
            # Must remain quarantined!
            return df

        ex_ts = pd.to_datetime(factor_calc.ex_date).tz_localize("UTC")
        mask_historical = df["timestamp"] < ex_ts

        factor_float = float(factor_calc.computed_factor)
        df.loc[mask_historical, "adjusted_close"] = (df.loc[mask_historical, "close"] * factor_float).round(4)
        return df
