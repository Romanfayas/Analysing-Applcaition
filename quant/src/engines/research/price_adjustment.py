"""
HalalEquity P1.8.8 - Point-in-Time Corporate Action & Price Adjustment Engine

Implements mathematically rigorous, fail-closed price reconstruction across:
1. Cash Dividends (Capital Gain vs Total Return index series)
2. Stock Splits & Consolidations (Price down-scaling, Volume up-scaling)
3. Bonus Issues (Dilution scaling, Volume up-scaling)
4. Rights Issues (TERP theoretical ex-rights dilution)
5. Demergers (Cost apportionment & strict pre-listing isolation)
6. Symbol Changes (Ticker rename metadata tracking with factor 1.0)

Invariants Enforced:
- Raw OHLCV bars remain immutable.
- Consistent backward-adjustment multiplying Open, High, Low, Close by exact same scalar factor.
- Volume is scaled for splits and bonus issues (preserving notional turnover P * V),
  but strictly NOT scaled for cash dividends.
- Point-in-time safety: Actions known after as_of_date are never incorporated prematurely.
- Pre-listing dates for successor entities (TMPV, TMCV) reject synthetic historical splicing.
- Complete audit provenance: Factor source reference, revision number, and verification status.
"""

from dataclasses import dataclass, field
from datetime import date, datetime, timezone
from decimal import Decimal, ROUND_HALF_UP
from enum import Enum
from typing import Dict, List, Optional, Tuple, Any, Set
import numpy as np
import pandas as pd


class ActionType(str, Enum):
    CASH_DIVIDEND = "CASH_DIVIDEND"
    STOCK_SPLIT = "STOCK_SPLIT"
    BONUS_ISSUE = "BONUS_ISSUE"
    RIGHTS_ISSUE = "RIGHTS_ISSUE"
    DEMERGER = "DEMERGER"
    SYMBOL_CHANGE = "SYMBOL_CHANGE"


class FactorCategory(str, Enum):
    SHARE_TRANSFORMATION = "SHARE_TRANSFORMATION"  # Splits, reverse splits, bonus issues
    CASH_DISTRIBUTION = "CASH_DISTRIBUTION"        # Cash dividends (nominal vs total return)
    DILUTION_OFFERING = "DILUTION_OFFERING"        # Rights issues (TERP)
    TAX_COST_ALLOCATION = "TAX_COST_ALLOCATION"    # Section 49(2C) demerger cost allocation (NOT price factor)
    PRICE_ADJUSTMENT = "PRICE_ADJUSTMENT"          # Market-derived price adjustments
    SYMBOLOGY_LINEAGE = "SYMBOLOGY_LINEAGE"        # Ticker renames, successor tracking
    UNSPECIFIED = "UNSPECIFIED"


class DisputeStatus(str, Enum):
    UNDISPUTED = "UNDISPUTED"
    DISPUTED_METADATA_DISCREPANCY = "DISPUTED_METADATA_DISCREPANCY"
    DISPUTED_NON_PRICE_FACTOR = "DISPUTED_NON_PRICE_FACTOR"


class AdjustmentMode(str, Enum):
    PRICE_RETURN = "PRICE_RETURN"       # Split & bonus adjusted; dividends unadjusted (standard for nominal price levels & stops)
    TOTAL_RETURN = "TOTAL_RETURN"       # Split, bonus & cash dividend adjusted (wealth performance & backtest comparison)
    RAW_UNADJUSTED = "RAW_UNADJUSTED"   # Strictly unadjusted raw exchange prices


@dataclass(frozen=True)
class CorporateActionEvent:
    symbol: str
    action_type: ActionType
    ex_date: date
    record_date: Optional[date]
    effective_date: date
    adjustment_factor: float
    cumulative_total_factor: float
    source_reference: str
    revision_number: int = 1
    is_verified: bool = True
    verification_status: str = "VERIFIED_EXCHANGE_PRIMARY"
    split_ratio_old: Optional[float] = None
    split_ratio_new: Optional[float] = None
    cash_dividend_per_share: Optional[float] = None
    factor_category: FactorCategory = FactorCategory.UNSPECIFIED
    price_factor_applicable: bool = False  # Strictly fail-closed default!
    nominal_price_factor: float = 1.0
    total_return_factor: float = 1.0
    volume_adjustment_factor: float = 1.0
    tax_cost_allocation_pct: Optional[float] = None
    cum_price_reference: Optional[float] = None
    ex_price_reference: Optional[float] = None
    dispute_status: DisputeStatus = DisputeStatus.UNDISPUTED
    dispute_reason: Optional[str] = None
    event_id: str = "DEFAULT"
    sha256_checksum: Optional[str] = None
    issuing_authority: Optional[str] = None
    source_artifact_id: Optional[int] = None
    notes: Optional[str] = None
    details: Dict[str, Any] = field(default_factory=dict)

    def is_valid_price_factor(self) -> bool:
        """
        Fail-closed check: returns True only if factor is explicitly classified in an
        allowed price category, marked price_factor_applicable=True, verified, undisputed,
        and strictly positive.
        """
        valid_categories = {
            FactorCategory.SHARE_TRANSFORMATION,
            FactorCategory.CASH_DISTRIBUTION,
            FactorCategory.DILUTION_OFFERING,
            FactorCategory.PRICE_ADJUSTMENT,
        }
        return (
            self.price_factor_applicable
            and self.is_verified
            and self.dispute_status == DisputeStatus.UNDISPUTED
            and self.factor_category in valid_categories
            and self.nominal_price_factor > 0
            and self.total_return_factor > 0
            and self.volume_adjustment_factor > 0
        )

    def is_dilution_action(self) -> bool:
        """Splits, bonus issues, and rights issues alter share count and require volume adjustment."""
        return self.action_type in (ActionType.STOCK_SPLIT, ActionType.BONUS_ISSUE, ActionType.RIGHTS_ISSUE)

    def is_dividend_action(self) -> bool:
        """Cash dividends affect total return but do not alter share count (volume unscaled)."""
        return self.action_type == ActionType.CASH_DIVIDEND

    def is_symbology_only(self) -> bool:
        """Symbol renames do not alter economic price series (factor strictly 1.0)."""
        return self.action_type == ActionType.SYMBOL_CHANGE

    def is_pure_tax_allocation(self) -> bool:
        """Demerger cost apportionment is for tax basis calculations, not price series reconstruction."""
        return self.factor_category == FactorCategory.TAX_COST_ALLOCATION or not self.price_factor_applicable


@dataclass
class AdjustmentFactorChain:
    symbol: str
    events: List[CorporateActionEvent]
    as_of_date: date
    mode: AdjustmentMode

    def get_factor_on_date(self, d: date) -> Tuple[float, float]:
        """
        Computes backward adjustment factor for date d.
        Returns (price_factor, volume_factor).
        Prices prior to ex_date are multiplied by price_factor.
        Volumes prior to ex_date are divided by volume_factor (scaled by 1 / volume_factor).
        """
        price_factor = 1.0
        volume_scale = 1.0

        # Filter strictly verified, undisputed, price-applicable events chronologically
        sorted_events = sorted(
            [
                e for e in self.events
                if e.ex_date <= self.as_of_date
                and e.is_valid_price_factor()
            ],
            key=lambda x: (x.ex_date, x.revision_number)
        )

        for e in sorted_events:
            if d < e.ex_date:
                if self.mode == AdjustmentMode.TOTAL_RETURN:
                    # Total return uses total_return_factor (splits, bonuses, and cash dividends)
                    factor_to_apply = e.total_return_factor if e.total_return_factor != 1.0 else e.adjustment_factor
                    price_factor *= factor_to_apply
                elif self.mode == AdjustmentMode.PRICE_RETURN:
                    # Nominal price return: cash dividends MUST NEVER alter nominal price series (strictly 1.0)
                    if e.is_dividend_action():
                        factor_to_apply = 1.0
                    elif e.nominal_price_factor > 0 and e.nominal_price_factor != 1.0:
                        factor_to_apply = e.nominal_price_factor
                    elif e.is_dilution_action():
                        factor_to_apply = e.adjustment_factor
                    else:
                        factor_to_apply = 1.0
                    price_factor *= factor_to_apply

                # Volume is scaled ONLY by volume_adjustment_factor (or for dilution actions)
                v_fac = e.volume_adjustment_factor if e.volume_adjustment_factor != 1.0 else (
                    e.adjustment_factor if e.is_dilution_action() else 1.0
                )
                volume_scale *= v_fac

        return price_factor, volume_scale


class PriceAdjustmentEngine:
    """
    Core quantitative engine for point-in-time price reconstruction and audit.
    """

    @staticmethod
    def calculate_split_factor(old_shares: float, new_shares: float) -> float:
        """
        Stock split factor = old_shares / new_shares.
        Example: 2-for-1 split (1 old share becomes 2 new shares) -> 1 / 2 = 0.50.
        """
        if old_shares <= 0 or new_shares <= 0:
            raise ValueError(f"Invalid split ratio: old={old_shares}, new={new_shares}; both must be positive")
        return float(Decimal(str(old_shares)) / Decimal(str(new_shares)))

    @staticmethod
    def calculate_bonus_factor(bonus_shares: float, existing_shares: float) -> float:
        """
        Bonus issue factor = existing_shares / (existing_shares + bonus_shares).
        Example: 1:1 bonus issue (1 bonus for every 1 held) -> 1 / (1 + 1) = 0.50.
        """
        if bonus_shares <= 0 or existing_shares <= 0:
            raise ValueError(f"Invalid bonus ratio: bonus={bonus_shares}, existing={existing_shares}")
        total = Decimal(str(existing_shares)) + Decimal(str(bonus_shares))
        return float(Decimal(str(existing_shares)) / total)

    @staticmethod
    def calculate_cash_dividend_factor(cum_dividend_close: float, dividend_amount: float) -> float:
        """
        Cash dividend factor for Total Return Index = (P_cum - D) / P_cum.
        Example: ₹12 dividend on ₹2,269.25 stock -> (2269.25 - 12) / 2269.25 = 0.99471197.
        """
        if cum_dividend_close <= 0:
            raise ValueError(f"Invalid cum-dividend close price: {cum_dividend_close}; must be positive")
        if dividend_amount <= 0:
            raise ValueError(f"Invalid dividend amount: {dividend_amount}; must be positive")
        if dividend_amount >= cum_dividend_close:
            raise ValueError(f"Dividend amount {dividend_amount} >= cum close {cum_dividend_close}; economic return invalid")

        p_cum = Decimal(str(cum_dividend_close))
        d_amt = Decimal(str(dividend_amount))
        factor = (p_cum - d_amt) / p_cum
        return float(factor.quantize(Decimal("0.00000001"), rounding=ROUND_HALF_UP))

    @staticmethod
    def calculate_rights_issue_factor(
        cum_price: float,
        subscription_price: float,
        new_ratio: float,
        old_ratio: float,
        economic_rational_exercise: bool = True,
    ) -> float:
        """
        Theoretical Ex-Rights Price (TERP) and dilution factor calculation.

        Mathematical Formulation:
            TERP = (N_old * P_cum + N_new * P_sub) / (N_old + N_new)
            factor = TERP / P_cum

        Regime Analysis:
        1. Discount / In-the-money (P_sub < P_cum):
           Rights are economically valuable (Intrinsic Value = (P_cum - P_sub) / (N_old/N_new + 1) > 0).
           New shares are issued at a discount, diluting existing shares.
           TERP < P_cum, resulting in factor < 1.0 (backward prices scaled down, volume scaled up).

        2. At-the-market (P_sub == P_cum):
           TERP = P_cum, resulting in factor = 1.00000000 under both models.
           Capital is raised at the prevailing market price without per-share dilution.

        3. Premium / Out-of-the-money (P_sub > P_cum):
           - Standard Corporate Actions & Exchange Rule (economic_rational_exercise=True):
             Rational investors will NOT exercise rights at P_sub when shares are obtainable cheaper
             in the secondary market at P_cum. Value of the right is zero (V_right = max(0, P_cum - P_sub) = 0).
             The rights issue lapses with 0% take-up. No new capital enters and no new shares are issued.
             Under official exchange methodology (NSE circulars, S&P Dow Jones Corporate Actions),
             no market adjustment is made: factor is strictly 1.00000000.
           - Unconditional Mathematical Formula (economic_rational_exercise=False):
             Unconditional mechanical substitution into the TERP equation yields TERP > P_cum
             and factor > 1.00000000.
        """
        if cum_price <= 0 or subscription_price <= 0 or new_ratio <= 0 or old_ratio <= 0:
            raise ValueError("All rights parameters must be positive")

        p_cum = Decimal(str(cum_price))
        p_sub = Decimal(str(subscription_price))
        n_new = Decimal(str(new_ratio))
        n_old = Decimal(str(old_ratio))

        if economic_rational_exercise and p_sub >= p_cum:
            # Out-of-the-money or at-the-money rights: no dilution occurs
            return 1.0

        terp = (n_old * p_cum + n_new * p_sub) / (n_old + n_new)
        factor = terp / p_cum
        return float(factor.quantize(Decimal("0.00000001"), rounding=ROUND_HALF_UP))

    @classmethod
    def reconstruct_series(
        cls,
        df: pd.DataFrame,
        events: List[CorporateActionEvent],
        mode: AdjustmentMode = AdjustmentMode.PRICE_RETURN,
        as_of_date: Optional[date] = None
    ) -> pd.DataFrame:
        """
        Applies point-in-time corporate action adjustments to a raw OHLCV DataFrame.
        Enforces strict consistency:
        - Open, High, Low, Close are multiplied by exact same scalar price factor.
        - Volume is scaled for splits and bonuses, NOT for cash dividends.
        - Returns a clean DataFrame with both raw and adjusted columns.
        """
        if df.empty:
            return df.copy()

        required_cols = {"timestamp", "open", "high", "low", "close", "volume"}
        missing_cols = required_cols - set(df.columns)
        if missing_cols:
            raise ValueError(f"Missing required OHLCV columns: {missing_cols}")

        out = df.copy()
        out["timestamp"] = pd.to_datetime(out["timestamp"])
        out["date"] = out["timestamp"].dt.date
        as_of = as_of_date or out["date"].max()

        symbol = str(out["symbol"].iloc[0]) if "symbol" in out.columns else "UNKNOWN"
        chain = AdjustmentFactorChain(symbol=symbol, events=events, as_of_date=as_of, mode=mode)

        price_factors = []
        volume_scales = []

        for d in out["date"]:
            p_fac, v_fac = chain.get_factor_on_date(d)
            price_factors.append(p_fac)
            volume_scales.append(v_fac)

        p_arr = np.array(price_factors)
        v_arr = np.array(volume_scales)

        # Store explicit adjustment provenance columns
        out["adjustment_mode"] = mode.value
        out["price_factor"] = p_arr
        out["volume_factor"] = v_arr

        # Retain raw unadjusted columns for full audit recovery
        if "raw_open" not in out.columns:
            out["raw_open"] = out["open"]
            out["raw_high"] = out["high"]
            out["raw_low"] = out["low"]
            out["raw_close"] = out["close"]
            out["raw_volume"] = out["volume"]

        # Apply backward adjustments consistently
        out["open"] = np.round(out["raw_open"].values * p_arr, 4)
        out["high"] = np.round(out["raw_high"].values * p_arr, 4)
        out["low"] = np.round(out["raw_low"].values * p_arr, 4)
        out["close"] = np.round(out["raw_close"].values * p_arr, 4)

        # Volume adjustment: scaled by 1 / v_fac (for splits and bonuses only)
        # For dividends, v_arr is 1.0, preserving raw volume
        out["volume"] = np.round(out["raw_volume"].values / v_arr).astype(int)

        # Populate adjusted_close column
        out["adjusted_close"] = out["close"]

        # Invariant checks
        cls.validate_ohlc_invariants(out)

        return out

    @staticmethod
    def validate_ohlc_invariants(df: pd.DataFrame):
        """Verifies mathematical validity of OHLC and Volume values after adjustment."""
        if (df["high"] < df["low"]).any():
            raise ValueError("Adjusted High < Low detected; adjustment corrupted bar geometry")
        if (df["open"] <= 0).any() or (df["close"] <= 0).any():
            raise ValueError("Non-positive adjusted price detected")
        if (df["volume"] < 0).any():
            raise ValueError("Negative adjusted volume detected")

    @staticmethod
    def check_lineage_and_pre_listing(
        symbol: str,
        requested_start_date: date,
        listing_date: Optional[date] = None
    ) -> Tuple[bool, str]:
        """
        Guards against pre-listing synthetic history fabrication for successor companies (e.g. TMCV, TMPV).
        """
        # Known corporate action lineage definitions
        lineage_map = {
            "TMPV": {
                "listing_date": date(2025, 10, 24),
                "parent": "TATAMOTORS",
                "notes": "Demerged successor entity (Passenger Vehicles). No pre-listing market data exists."
            },
            "TMCV": {
                "listing_date": date(2025, 10, 24),
                "parent": "TATAMOTORS",
                "notes": "Demerged successor entity (Commercial Vehicles). No pre-listing market data exists."
            },
            "LTM": {
                "listing_date": date(2026, 2, 27),
                "parent": "LTIM",
                "notes": "Ticker rename successor from LTIM. 1:1 nominal continuity."
            }
        }

        if symbol in lineage_map:
            l_info = lineage_map[symbol]
            effective_listing = listing_date or l_info["listing_date"]
            if requested_start_date < effective_listing:
                return False, (
                    f"PRE_LISTING_UNTRADABLE: {symbol} was listed on {effective_listing.isoformat()}. "
                    f"Historical price series starting on {requested_start_date.isoformat()} is invalid. "
                    f"Fabricating or splicing raw predecessor ({l_info['parent']}) prices violates data integrity."
                )

        return True, "VALID_TIMELINE"


def load_verified_factors_from_db(
    conn,
    symbol: Optional[str] = None,
    only_price_applicable: bool = True,
    include_unverified: bool = False,
    mode: Optional[AdjustmentMode] = None,
) -> List[CorporateActionEvent]:
    """
    Loads corporate action adjustment factors from database views or ledger table.
    Enforces append-only revision semantics:
    1. Deterministically resolves the latest revision for each event (symbol, ex_date, action_type, event_id).
    2. Strictly filters verified, undisputed, and price-applicable factors when only_price_applicable=True.
    3. If mode == AdjustmentMode.PRICE_RETURN, strictly isolates nominal price actions (cash dividends excluded).
    """
    cur = conn.cursor()
    params = []

    # Check if view is supported, else fallback to CTE
    try:
        if only_price_applicable and not include_unverified:
            where_clause = "WHERE symbol = %s" if symbol else ""
            if symbol:
                params.append(symbol)
            query = f"""
                SELECT 
                    symbol, action_type, ex_date, record_date, effective_date,
                    adjustment_factor, cumulative_total_factor, source_reference,
                    revision_number, is_verified, verification_status,
                    split_ratio_old, split_ratio_new, cash_dividend_per_share,
                    factor_category, price_factor_applicable, nominal_price_factor,
                    total_return_factor, volume_adjustment_factor, tax_cost_allocation_pct,
                    cum_price_reference, ex_price_reference, dispute_status, dispute_reason, event_id
                FROM v_active_price_adjustment_factors
                {where_clause}
                ORDER BY symbol, ex_date ASC;
            """
            cur.execute(query, params)
            rows = cur.fetchall()
        else:
            conditions = []
            if not include_unverified:
                conditions.append("is_verified = true")
            if only_price_applicable:
                conditions.append("price_factor_applicable = true")
                conditions.append("dispute_status = 'UNDISPUTED'")
            if symbol:
                conditions.append("symbol = %s")
                params.append(symbol)
            where_clause = ("WHERE " + " AND ".join(conditions)) if conditions else ""
            query = f"""
                SELECT 
                    symbol, action_type, ex_date, record_date, effective_date,
                    adjustment_factor, cumulative_total_factor, source_reference,
                    revision_number, is_verified, verification_status,
                    split_ratio_old, split_ratio_new, cash_dividend_per_share,
                    factor_category, price_factor_applicable, nominal_price_factor,
                    total_return_factor, volume_adjustment_factor, tax_cost_allocation_pct,
                    cum_price_reference, ex_price_reference, dispute_status, dispute_reason, event_id
                FROM v_latest_corporate_action_factors
                {where_clause}
                ORDER BY symbol, ex_date ASC;
            """
            cur.execute(query, params)
            rows = cur.fetchall()
    except Exception:
        # Fallback to direct table CTE if view does not exist in mock/test environment
        if hasattr(conn, "rollback"):
            conn.rollback()
        conditions = []
        if not include_unverified:
            conditions.append("is_verified = true")
        if only_price_applicable:
            conditions.append("price_factor_applicable = true")
            conditions.append("dispute_status = 'UNDISPUTED'")
        params = []
        if symbol:
            conditions.append("symbol = %s")
            params.append(symbol)

        where = ("WHERE " + " AND ".join(conditions)) if conditions else ""

        query = f"""
            WITH latest_factors AS (
                SELECT DISTINCT ON (s.symbol, c.ex_date, c.action_type, COALESCE(c.event_id, 'DEFAULT'))
                    s.symbol, c.action_type, c.ex_date, c.record_date, c.effective_date,
                    c.adjustment_factor, c.cumulative_total_factor, c.source_reference,
                    c.revision_number, c.is_verified, c.verification_status,
                    c.split_ratio_old, c.split_ratio_new, c.cash_dividend_per_share,
                    c.factor_category, c.price_factor_applicable, c.nominal_price_factor,
                    c.total_return_factor, c.volume_adjustment_factor, c.tax_cost_allocation_pct,
                    c.cum_price_reference, c.ex_price_reference, c.dispute_status, c.dispute_reason,
                    COALESCE(c.event_id, 'DEFAULT') as event_id
                FROM corporate_action_adjustment_factors c
                JOIN symbols s ON c.symbol_id = s.id
                ORDER BY s.symbol, c.ex_date, c.action_type, COALESCE(c.event_id, 'DEFAULT'), c.revision_number DESC
            )
            SELECT 
                symbol, action_type, ex_date, record_date, effective_date,
                adjustment_factor, cumulative_total_factor, source_reference,
                revision_number, is_verified, verification_status,
                split_ratio_old, split_ratio_new, cash_dividend_per_share,
                factor_category, price_factor_applicable, nominal_price_factor,
                total_return_factor, volume_adjustment_factor, tax_cost_allocation_pct,
                cum_price_reference, ex_price_reference, dispute_status, dispute_reason, event_id
            FROM latest_factors
            {where}
            ORDER BY symbol, ex_date ASC;
        """
        cur.execute(query, params)
        rows = cur.fetchall()

    events = []
    for r in rows:
        events.append(CorporateActionEvent(
            symbol=r[0],
            action_type=ActionType(r[1]),
            ex_date=r[2],
            record_date=r[3],
            effective_date=r[4],
            adjustment_factor=float(r[5]),
            cumulative_total_factor=float(r[6]),
            source_reference=r[7],
            revision_number=int(r[8]),
            is_verified=bool(r[9]),
            verification_status=str(r[10]),
            split_ratio_old=float(r[11]) if r[11] is not None else None,
            split_ratio_new=float(r[12]) if r[12] is not None else None,
            cash_dividend_per_share=float(r[13]) if r[13] is not None else None,
            factor_category=FactorCategory(r[14]) if r[14] else FactorCategory.UNSPECIFIED,
            price_factor_applicable=bool(r[15]),
            nominal_price_factor=float(r[16]),
            total_return_factor=float(r[17]),
            volume_adjustment_factor=float(r[18]),
            tax_cost_allocation_pct=float(r[19]) if r[19] is not None else None,
            cum_price_reference=float(r[20]) if r[20] is not None else None,
            ex_price_reference=float(r[21]) if r[21] is not None else None,
            dispute_status=DisputeStatus(r[22]) if r[22] else DisputeStatus.UNDISPUTED,
            dispute_reason=str(r[23]) if r[23] is not None else None,
            event_id=str(r[24]) if len(r) > 24 and r[24] is not None else "DEFAULT",
        ))

    if mode == AdjustmentMode.PRICE_RETURN:
        events = [e for e in events if not e.is_dividend_action()]
    elif mode == AdjustmentMode.TOTAL_RETURN:
        events = [e for e in events if e.total_return_factor != 1.0 or e.is_dilution_action()]

    return events



