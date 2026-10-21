"""
Indian equity cash-delivery transaction-cost engine (P1.7, COST_MODEL_2026_Q4_V2).

Replaces the P1.6 engine. Key corrections and design rules:

* STT on delivery is levied on BOTH legs (0.1% buy + 0.1% sell). P1.6 charged the sell leg only.
* All arithmetic is Decimal and unrounded; rounding happens once per (line item, leg) to the paisa
  (ROUND_HALF_UP, the contract-note convention) and totals are sums of the rounded lines, so a ledger
  always foots exactly. The unrounded total and the rounding delta are reported alongside.
* Three clearly separated cost categories:
    STATUTORY  - STT, stamp duty, exchange transaction charge, SEBI turnover fee, GST on the latter two (and on broker fees)
    BROKER     - brokerage, DP (depository) charge and the GST on it. Broker/plan specific -> configuration, not law.
    MODELED    - slippage / market impact. An assumption, not an observed fill.
* Slippage is applied ONCE, to the fill prices. Fees are computed on the filled turnover. Net P&L is
  qty*(fill_sell-fill_buy) - fees, which equals gross(reference) - slippage - fees. Nothing is deducted twice.
* Every rate is in a versioned, overridable FeeSchedule / SlippageModel. Nothing here is a claim about
  what any particular broker charges today; see `schedule_source`.
"""
from dataclasses import dataclass, field
from decimal import Decimal, ROUND_HALF_UP, ROUND_FLOOR
from enum import Enum
from typing import Any, Dict, List, Optional

D = Decimal
PAISA = D("0.01")
MODEL_VERSION = "COST_MODEL_2026_Q4_V2"


def _r(x: Decimal) -> Decimal:
    return x.quantize(PAISA, rounding=ROUND_HALF_UP)


def _d(x) -> Decimal:
    return x if isinstance(x, Decimal) else D(str(x))


class BrokerageModel(str, Enum):
    ZERO_DELIVERY = "ZERO_DELIVERY"        # no delivery brokerage
    FLAT_PER_ORDER = "FLAT_PER_ORDER"      # fixed amount per executed order
    PERCENTAGE = "PERCENTAGE"              # pct of leg turnover (optional min/max)
    MIN_FLAT_OR_PCT = "MIN_FLAT_OR_PCT"    # lower of flat amount and pct of turnover (common discount-broker plan)


class Category(str, Enum):
    STATUTORY = "STATUTORY"
    BROKER = "BROKER"
    MODELED = "MODELED"


@dataclass(frozen=True)
class FeeSchedule:
    version: str = "INDIA_EQ_DELIVERY_FEES_2026Q4_V2"
    stt_buy_rate: Decimal = D("0.001")
    stt_sell_rate: Decimal = D("0.001")
    exchange_txn_rate: Decimal = D("0.0000297")      # NSE cash. Published rates have moved: 0.00345% -> 0.00322% -> 0.00307% -> 0.00297%
    sebi_rate: Decimal = D("0.000001")               # Rs 10 per crore
    stamp_buy_rate: Decimal = D("0.00015")           # 0.015% buy side only, uniform across states
    gst_rate: Decimal = D("0.18")                    # on brokerage + exchange txn + SEBI fee + DP charge. NOT on STT / stamp duty
    brokerage_model: BrokerageModel = BrokerageModel.ZERO_DELIVERY
    brokerage_flat: Decimal = D("0")
    brokerage_pct: Decimal = D("0")
    brokerage_min: Decimal = D("0")
    brokerage_max: Optional[Decimal] = None
    dp_charge_ex_gst_per_sell: Decimal = D("13.50")  # broker/DP-plan specific; charged once per scrip per sell day; GST added on top
    schedule_source: str = (
        "Rates compiled 2026-10-14 from public broker fee pages (zerodha.com/charges, upstox help-center). "
        "NOT verified against the exchange/SEBI circulars or against the contracted broker's schedule: re-verify before relying on absolute values."
    )


@dataclass(frozen=True)
class SlippageModel:
    version: str = "SLIPPAGE_ASSUMED_5BPS_PER_SIDE_V1"
    buy_bps: Decimal = D("5")
    sell_bps: Decimal = D("5")
    disclosure: str = "Assumed market-impact / execution slippage. A simulation parameter, NOT an observed fill."


@dataclass
class CostModelConfig:
    """Back-compat name; groups fee schedule + slippage model under one version."""
    fees: FeeSchedule = field(default_factory=FeeSchedule)
    slippage: SlippageModel = field(default_factory=SlippageModel)
    version: str = MODEL_VERSION


@dataclass
class LedgerLine:
    item: str
    category: str
    leg: str                 # BUY | SELL
    rate_or_rule: str
    base_inr: str            # fee base (Decimal as string, unrounded)
    unrounded_inr: str       # Decimal as string
    rounded_inr: float       # paisa-rounded, what is summed into totals
    note: str = ""


@dataclass
class TradeCostCalculation:
    model_version: str
    fee_schedule_version: str
    slippage_model_version: str
    symbol: str
    quantity: int
    reference_buy_price: float
    reference_sell_price: float
    fill_buy_price: str
    fill_sell_price: str
    holding_days: int
    buy_turnover: float          # at fill prices, paisa-rounded
    sell_turnover: float
    gross_pnl_reference: float   # qty*(ref_sell-ref_buy), before any cost
    statutory_total: float
    broker_total: float
    modeled_slippage_total: float
    total_costs: float           # sum of rounded ledger lines
    total_costs_unrounded: str
    rounding_delta: float        # total_costs - unrounded total
    net_pnl: float               # gross_pnl_reference - total_costs  (rounded domain)
    net_pnl_unrounded: str
    gross_return_pct: float
    net_return_pct: float
    effective_cost_bps: float    # on reference buy notional
    is_reconciled: bool
    reconciliation_discrepancy: float
    ledger: List[LedgerLine] = field(default_factory=list)

    def to_dict(self) -> Dict[str, Any]:
        from dataclasses import asdict
        return asdict(self)

    def total_for(self, item: str) -> float:
        return float(sum(D(str(l.rounded_inr)) for l in self.ledger if l.item == item))

    @property
    def total_costs_rounded(self) -> float:
        """Paisa-rounded total friction vs reference prices (statutory + broker + slippage)."""
        return self.total_costs

    @property
    def fees_only_total(self) -> float:
        """Actual cash fees paid (statutory + broker), excluding modeled execution slippage."""
        return float(_r(D(str(self.statutory_total)) + D(str(self.broker_total))))

    @property
    def fill_gross_pnl(self) -> float:
        """Gross P&L at filled prices: quantity * (fill_sell - fill_buy). Already reflects slippage."""
        return float(_r(D(str(self.quantity)) * (D(self.fill_sell_price) - D(self.fill_buy_price))))

    @property
    def net_pnl_reference_basis(self) -> float:
        """Identity 1 (Reference-Price Basis):
        net_pnl = reference_pnl - slippage_cost - statutory_fees - broker_fees."""
        ref_pnl = D(str(self.gross_pnl_reference))
        slip = D(str(self.modeled_slippage_total))
        stat = D(str(self.statutory_total))
        brok = D(str(self.broker_total))
        return float(_r(ref_pnl - slip - stat - brok))

    @property
    def net_pnl_fill_basis(self) -> float:
        """Identity 2 (Fill-Price Basis):
        net_pnl = quantity * (exit_fill - entry_fill) - statutory_fees - broker_fees.
        Note: Slippage is NEVER subtracted again because exit_fill and entry_fill already incorporate it."""
        fill_pnl = D(str(self.quantity)) * (D(self.fill_sell_price) - D(self.fill_buy_price))
        stat = D(str(self.statutory_total))
        brok = D(str(self.broker_total))
        return float(_r(fill_pnl - stat - brok))

    @property
    def entry_notional_bps(self) -> float:
        """Friction in basis points relative to Entry Reference Notional (q * reference_buy_price)."""
        notional = D(str(self.quantity)) * D(str(self.reference_buy_price))
        return float(D(str(self.total_costs)) / notional * D("10000"))

    @property
    def roundtrip_turnover_bps(self) -> float:
        """Friction in basis points relative to Total Round-Trip Reference Turnover (q * pb + q * ps)."""
        turnover = D(str(self.quantity)) * (D(str(self.reference_buy_price)) + D(str(self.reference_sell_price)))
        return float(D(str(self.total_costs)) / turnover * D("10000"))

    @property
    def is_double_count_free(self) -> bool:
        """Verifies both identities foot identically to the paisa and slippage is not deducted twice."""
        ref_basis = D(str(self.net_pnl_reference_basis))
        fill_basis = D(str(self.net_pnl_fill_basis))
        net = D(str(self.net_pnl))
        return abs(ref_basis - fill_basis) < D("0.02") and abs(net - ref_basis) < D("0.02")


class CostEngine:
    def __init__(self, config: Optional[CostModelConfig] = None):
        self.config = config or CostModelConfig()

    # ---- brokerage ----
    def _brokerage(self, turnover: Decimal) -> Decimal:
        f = self.config.fees
        m = f.brokerage_model
        if m == BrokerageModel.ZERO_DELIVERY:
            return D("0")
        if m == BrokerageModel.FLAT_PER_ORDER:
            return f.brokerage_flat
        pct = turnover * f.brokerage_pct
        if m == BrokerageModel.MIN_FLAT_OR_PCT:
            pct = min(pct, f.brokerage_flat)
        if pct < f.brokerage_min:
            pct = f.brokerage_min
        if f.brokerage_max is not None and pct > f.brokerage_max:
            pct = f.brokerage_max
        return pct

    def leg_lines(self, turnover: Decimal, leg: str) -> List[LedgerLine]:
        """Fee lines for one leg given its (unrounded) filled turnover."""
        f = self.config.fees
        buy = leg == "BUY"
        lines: List[LedgerLine] = []

        def add(item, cat, rate, base, amt, note=""):
            lines.append(LedgerLine(item, cat.value, leg, rate, str(base), str(amt), float(_r(amt)), note))

        stt_rate = f.stt_buy_rate if buy else f.stt_sell_rate
        add("STT", Category.STATUTORY, f"{stt_rate*100:f}% of turnover", turnover, turnover * stt_rate, "Delivery STT is levied on both legs; not subject to GST")
        add("Exchange transaction charge", Category.STATUTORY, f"{f.exchange_txn_rate*100:f}% of turnover", turnover, turnover * f.exchange_txn_rate, "NSE cash segment; GST applies")
        add("SEBI turnover fee", Category.STATUTORY, f"{f.sebi_rate*100:f}% of turnover (Rs10/crore)", turnover, turnover * f.sebi_rate, "GST applies")
        stamp = turnover * f.stamp_buy_rate if buy else D("0")
        add("Stamp duty", Category.STATUTORY, f"{f.stamp_buy_rate*100:f}% buy only", turnover, stamp, "Not subject to GST")
        brok = self._brokerage(turnover)
        add("Brokerage", Category.BROKER, f.brokerage_model.value, turnover, brok, "Broker/plan specific; GST applies")
        dp = f.dp_charge_ex_gst_per_sell if not buy else D("0")
        add("DP charge (ex-GST)", Category.BROKER, f"Rs{f.dp_charge_ex_gst_per_sell} per sell", D("0"), dp, "Depository participant debit charge; broker/plan specific")
        exch = turnover * f.exchange_txn_rate
        sebi = turnover * f.sebi_rate
        gst_base = exch + sebi + brok + dp
        add("GST", Category.STATUTORY, f"{f.gst_rate*100:f}% on txn charge + SEBI fee + brokerage + DP charge", gst_base,
            gst_base * f.gst_rate, "Excludes STT and stamp duty")
        return lines

    def calculate_trade(
        self,
        symbol: str,
        buy_price: float,
        sell_price: float,
        quantity: Optional[int] = None,
        target_order_value: Optional[float] = None,
        holding_days: int = 5,
    ) -> TradeCostCalculation:
        if buy_price <= 0 or sell_price <= 0:
            raise ValueError("prices must be positive")
        pb, ps = _d(buy_price), _d(sell_price)
        if quantity is None:
            quantity = int((_d(target_order_value) / pb).to_integral_value(rounding=ROUND_FLOOR)) if target_order_value else 100
        if quantity < 1:
            raise ValueError("quantity must be >= 1")
        q = D(quantity)
        s = self.config.slippage
        fill_b = pb * (1 + s.buy_bps / D(10000))
        fill_s = ps * (1 - s.sell_bps / D(10000))
        buy_turn, sell_turn = q * fill_b, q * fill_s

        ledger = self.leg_lines(buy_turn, "BUY") + self.leg_lines(sell_turn, "SELL")
        slip_b = q * (fill_b - pb)
        slip_s = q * (ps - fill_s)
        ledger.append(LedgerLine("Modeled slippage", Category.MODELED.value, "BUY", f"{s.buy_bps} bps on reference price", str(q * pb),
                                 str(slip_b), float(_r(slip_b)), s.disclosure))
        ledger.append(LedgerLine("Modeled slippage", Category.MODELED.value, "SELL", f"{s.sell_bps} bps on reference price", str(q * ps),
                                 str(slip_s), float(_r(slip_s)), s.disclosure))

        tot_r = sum((D(str(l.rounded_inr)) for l in ledger), D("0"))
        tot_u = sum((D(l.unrounded_inr) for l in ledger), D("0"))
        by_cat = lambda c: sum((D(str(l.rounded_inr)) for l in ledger if l.category == c.value), D("0"))

        gross_ref = q * (ps - pb)
        gross_r = _r(gross_ref)
        net_r = gross_r - tot_r
        net_u = gross_ref - tot_u
        # independent identity: net P&L at fill prices minus (fees only) must equal gross-ref - slippage - fees
        fees_u = tot_u - (slip_b + slip_s)
        alt_net_u = q * (fill_s - fill_b) - fees_u
        disc = abs(alt_net_u - net_u)
        notional = q * pb
        return TradeCostCalculation(
            model_version=self.config.version,
            fee_schedule_version=self.config.fees.version,
            slippage_model_version=s.version,
            symbol=symbol, quantity=quantity,
            reference_buy_price=float(pb), reference_sell_price=float(ps),
            fill_buy_price=str(fill_b), fill_sell_price=str(fill_s),
            holding_days=holding_days,
            buy_turnover=float(_r(buy_turn)), sell_turnover=float(_r(sell_turn)),
            gross_pnl_reference=float(gross_r),
            statutory_total=float(by_cat(Category.STATUTORY)), broker_total=float(by_cat(Category.BROKER)),
            modeled_slippage_total=float(by_cat(Category.MODELED)),
            total_costs=float(tot_r), total_costs_unrounded=str(tot_u), rounding_delta=float(tot_r - tot_u),
            net_pnl=float(net_r), net_pnl_unrounded=str(net_u),
            gross_return_pct=float(gross_ref / notional * 100),
            net_return_pct=float(net_r / notional * 100),
            effective_cost_bps=float(tot_r / notional * 10000),
            is_reconciled=disc < D("1e-9"),
            reconciliation_discrepancy=float(disc),
            ledger=ledger,
        )
