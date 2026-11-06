"""Intraday round-trip costs, computed with the platform's cost engine (not assumed)."""
from decimal import Decimal as D
from functools import lru_cache

from src.engines.research.cost_model import (
    BrokerageModel,
    CostEngine,
    CostModelConfig,
    FeeSchedule,
    SlippageModel,
)

from .config import REFERENCE_ORDER_VALUE_INR

# Same-day (intraday, MIS) equity: STT on the sell leg only, lower stamp duty, no DP charge, and a
# discount-broker plan of min(Rs 20, 0.03%) per executed order.
INTRADAY_FEES = FeeSchedule(
    version="INDIA_EQ_INTRADAY_FEES_2026Q4_V1",
    stt_buy_rate=D("0"),
    stt_sell_rate=D("0.00025"),
    stamp_buy_rate=D("0.00003"),
    brokerage_model=BrokerageModel.MIN_FLAT_OR_PCT,
    brokerage_flat=D("20"),
    brokerage_pct=D("0.0003"),
    dp_charge_ex_gst_per_sell=D("0"),
    schedule_source=(
        "Public discount-broker intraday schedule (STT 0.025% sell, stamp 0.003% buy, brokerage min(Rs20, 0.03%)). "
        "Re-verify against your broker's contract note before relying on absolute values."
    ),
)


@lru_cache(maxsize=None)
def round_trip_cost_fraction(order_value: float = REFERENCE_ORDER_VALUE_INR, price: float = 100.0) -> float:
    """All-in buy+sell cost (statutory + brokerage + modeled slippage) as a fraction of notional."""
    qty = max(1, int(order_value // price))
    calc = CostEngine(CostModelConfig(fees=INTRADAY_FEES, slippage=SlippageModel())).calculate_trade(
        "REFERENCE", price, price, quantity=qty
    )
    return calc.total_costs / (qty * price)


def cost_model_summary() -> dict:
    return {
        "fee_schedule": INTRADAY_FEES.version,
        "slippage_model": SlippageModel().version,
        "reference_order_value_inr": REFERENCE_ORDER_VALUE_INR,
        "round_trip_cost_fraction": round_trip_cost_fraction(),
    }
