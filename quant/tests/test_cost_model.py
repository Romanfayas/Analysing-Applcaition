"""P1.7 cost-engine tests: parameterized + seeded randomized property tests (no new dependency)."""
import random
from decimal import Decimal as D, ROUND_HALF_UP

import pytest

from src.engines.research.cost_model import (
    BrokerageModel, CostEngine, CostModelConfig, FeeSchedule, SlippageModel,
)

NO_SLIP = SlippageModel(buy_bps=D(0), sell_bps=D(0))


def eng(**fee_kw):
    slip = fee_kw.pop("slippage", SlippageModel())
    return CostEngine(CostModelConfig(fees=FeeSchedule(**fee_kw), slippage=slip))


def line(c, item, leg):
    return sum(D(str(l.rounded_inr)) for l in c.ledger if l.item == item and l.leg == leg)


def unr(c, item, leg):
    return sum(D(l.unrounded_inr) for l in c.ledger if l.item == item and l.leg == leg)


def test_stt_charged_on_both_legs():
    c = eng(slippage=NO_SLIP).calculate_trade("X", 1000, 1080, quantity=10)
    assert unr(c, "STT", "BUY") == D("10")        # 0.1% of 10,000
    assert unr(c, "STT", "SELL") == D("10.8")


def test_gst_excludes_stt_and_stamp():
    c = eng(slippage=NO_SLIP).calculate_trade("X", 1000, 1080, quantity=10)
    expect_buy = (unr(c, "Exchange transaction charge", "BUY") + unr(c, "SEBI turnover fee", "BUY")) * D("0.18")
    assert unr(c, "GST", "BUY") == expect_buy
    expect_sell = (unr(c, "Exchange transaction charge", "SELL") + unr(c, "SEBI turnover fee", "SELL") + D("13.50")) * D("0.18")
    assert unr(c, "GST", "SELL") == expect_sell


def test_sell_only_and_buy_only_items():
    c = eng().calculate_trade("X", 100, 110, quantity=50)
    assert line(c, "Stamp duty", "SELL") == 0 and line(c, "Stamp duty", "BUY") > 0
    assert line(c, "DP charge (ex-GST)", "BUY") == 0 and line(c, "DP charge (ex-GST)", "SELL") == D("13.50")


@pytest.mark.parametrize("model,flat,pct,mn,mx,turnover,expected", [
    (BrokerageModel.ZERO_DELIVERY, 0, 0, 0, None, 100000, "0"),
    (BrokerageModel.FLAT_PER_ORDER, 20, 0, 0, None, 100, "20"),
    (BrokerageModel.PERCENTAGE, 0, "0.001", 0, None, 100000, "100"),
    (BrokerageModel.PERCENTAGE, 0, "0.001", 5, None, 1000, "5"),            # minimum applies
    (BrokerageModel.PERCENTAGE, 0, "0.001", 0, 50, 100000, "50"),           # cap applies
    (BrokerageModel.MIN_FLAT_OR_PCT, 20, "0.001", 0, None, 5000, "5"),       # pct lower
    (BrokerageModel.MIN_FLAT_OR_PCT, 20, "0.001", 0, None, 500000, "20"),    # flat lower
])
def test_brokerage_modes(model, flat, pct, mn, mx, turnover, expected):
    e = eng(brokerage_model=model, brokerage_flat=D(str(flat)), brokerage_pct=D(str(pct)), brokerage_min=D(str(mn)),
            brokerage_max=None if mx is None else D(str(mx)))
    assert e._brokerage(D(str(turnover))) == D(expected)


def test_zero_brokerage_has_no_brokerage_gst_component():
    c = eng(dp_charge_ex_gst_per_sell=D(0), slippage=NO_SLIP).calculate_trade("X", 100, 100, quantity=1)
    assert line(c, "Brokerage", "BUY") == 0 and line(c, "Brokerage", "SELL") == 0


def test_rounding_half_up_boundary():
    # 0.00345%-style boundary: pick a rate/turnover producing exactly x.xx5
    e = eng(exchange_txn_rate=D("0.0000345"), slippage=NO_SLIP)
    c = e.calculate_trade("X", 1000, 1000, quantity=10)     # 10,000 * 0.0000345 = 0.345 -> 0.35 (half-up, not banker's/float)
    assert unr(c, "Exchange transaction charge", "BUY") == D("0.3450000")
    assert line(c, "Exchange transaction charge", "BUY") == D("0.35")


def test_p16_gst_float_rounding_defect_is_fixed():
    # P1.6 printed 3.19 for GST on 3.195; half-up decimal gives 3.20
    c = eng(exchange_txn_rate=D("0.0000345"), slippage=NO_SLIP, dp_charge_ex_gst_per_sell=D(0)).calculate_trade("X", 2500, 2600, quantity=200)
    assert unr(c, "GST", "BUY") == D("3.195000") * 1 or abs(unr(c, "GST", "BUY") - D("3.195")) < D("1e-9")
    assert line(c, "GST", "BUY") == D("3.20")


def test_slippage_not_double_counted():
    base = eng(slippage=NO_SLIP).calculate_trade("X", 1000, 1080, quantity=10)
    withs = eng().calculate_trade("X", 1000, 1080, quantity=10)
    # net difference = modeled slippage + (fees computed on shifted fill turnover) - fees on reference turnover; never 2x slippage
    fee_base = D(base.total_costs_unrounded)
    fee_with = D(withs.total_costs_unrounded) - sum(D(l.unrounded_inr) for l in withs.ledger if l.category == "MODELED")
    slip = sum(D(l.unrounded_inr) for l in withs.ledger if l.category == "MODELED")
    assert slip == D("10.4")
    # identity: net(fill prices) - fees == gross_ref - slippage - fees
    q = D(10)
    assert q * (D(withs.fill_sell_price) - D(withs.fill_buy_price)) - fee_with == D(withs.net_pnl_unrounded)
    assert abs(fee_with - fee_base) < D("0.05")


def test_ledger_foots_and_reconciles_property():
    rnd = random.Random(1707)
    for _ in range(500):
        pb = round(rnd.uniform(5, 50000), 2)
        ps = round(pb * rnd.uniform(0.7, 1.4), 2)
        qty = rnd.choice([1, 2, 7, 10, 33, 100, 999, 25000])
        model = rnd.choice(list(BrokerageModel))
        e = eng(brokerage_model=model, brokerage_flat=D("20"), brokerage_pct=D("0.0005"), brokerage_min=D(rnd.choice([0, 1])),
                dp_charge_ex_gst_per_sell=D(rnd.choice([0, "13.50", "5.5"])),
                slippage=SlippageModel(buy_bps=D(rnd.choice([0, 5, 12])), sell_bps=D(rnd.choice([0, 5, 12]))))
        c = e.calculate_trade("X", pb, ps, quantity=qty)
        assert c.is_reconciled and c.reconciliation_discrepancy < 1e-9
        rounded_sum = sum(D(str(l.rounded_inr)) for l in c.ledger)
        assert D(str(c.total_costs)) == rounded_sum
        assert D(str(c.statutory_total)) + D(str(c.broker_total)) + D(str(c.modeled_slippage_total)) == rounded_sum
        # rounding error bounded by half a paisa per line
        assert abs(D(str(c.rounding_delta))) <= D("0.005") * len(c.ledger)
        # net(rounded domain) == gross - total costs, all to the paisa
        assert D(str(c.net_pnl)) == D(str(c.gross_pnl_reference)) - rounded_sum
        assert all(D(str(l.rounded_inr)) >= 0 for l in c.ledger)


def test_costs_monotonic_in_notional_and_fixed_charge_dominates_small_trades():
    small = eng(slippage=NO_SLIP).calculate_trade("X", 1000, 1000, quantity=10)
    large = eng(slippage=NO_SLIP).calculate_trade("X", 1000, 1000, quantity=500)
    assert large.total_costs > small.total_costs
    assert small.effective_cost_bps > large.effective_cost_bps    # fixed DP charge


def test_config_is_versioned_and_overridable():
    c = eng(dp_charge_ex_gst_per_sell=D(0)).calculate_trade("X", 100, 101, quantity=100)
    assert c.model_version == "COST_MODEL_2026_Q4_V2" and c.fee_schedule_version.startswith("INDIA_EQ")
    assert line(c, "DP charge (ex-GST)", "SELL") == 0


def test_invalid_inputs():
    with pytest.raises(ValueError):
        CostEngine().calculate_trade("X", 0, 10, quantity=1)
    with pytest.raises(ValueError):
        CostEngine().calculate_trade("X", 10, 10, quantity=0)


def test_zero_gross_trade_has_negative_net():
    c = eng().calculate_trade("X", 100, 100, quantity=10)
    assert c.net_pnl < 0


def test_breakout_backtester_cost_model_enforcement():
    import pandas as pd
    from src.engines.technical.breakout_backtest import BreakoutBacktester

    df = pd.DataFrame({
        'symbol': ['INFY'] * 60,
        'shariah_status': ['PASS'] * 60,
        'timestamp': pd.date_range('2024-01-01', periods=60, freq='D'),
        'open': [100.0] * 60,
        'high': [105.0] * 60,
        'low': [98.0] * 60,
        'close': [102.0] * 60,
        'volume': [10000] * 60,
    })

    # 1. Default without ledger flag uses legacy 47 bps
    bb_legacy = BreakoutBacktester(df)
    assert bb_legacy.is_legacy_cost is True
    assert bb_legacy.cost_model_version == "LEGACY_47BPS_PROVISIONAL"

    # 2. Explicit verified ledger
    bb_ledger = BreakoutBacktester(df, use_ledger=True)
    assert bb_ledger.is_legacy_cost is False
    assert bb_ledger.cost_model_version == "COST_MODEL_2026_Q4_V2"

    # 3. Explicit cost_model param
    bb_named = BreakoutBacktester(df, cost_model="COST_MODEL_2026_Q4_V2")
    assert bb_named.is_legacy_cost is False
    assert bb_named.use_ledger is True

    # 4. require_verified_ledger blocks silent fallback to legacy
    with pytest.raises(ValueError, match="Verified research runs must use institutional cost ledger"):
        BreakoutBacktester(df, require_verified_ledger=True)

    with pytest.raises(ValueError, match="Verified research runs must use institutional cost ledger"):
        BreakoutBacktester(df, cost_model="LEGACY_47BPS_PROVISIONAL", require_verified_ledger=True)

    # 5. require_verified_ledger succeeds when verified ledger is specified
    bb_verified = BreakoutBacktester(df, cost_model="COST_MODEL_2026_Q4_V2", require_verified_ledger=True)
    assert bb_verified.is_legacy_cost is False
    assert bb_verified.cost_model_version == "COST_MODEL_2026_Q4_V2"


def test_buy_and_sell_leg_lines():
    e = CostEngine()
    buy_lines = e.leg_lines(D("500000"), "BUY")
    sell_lines = e.leg_lines(D("520000"), "SELL")

    # Buy leg has Stamp duty > 0 and DP charge == 0
    assert any(l.item == "Stamp duty" and l.rounded_inr > 0 for l in buy_lines)
    assert any(l.item == "DP charge (ex-GST)" and l.rounded_inr == 0 for l in buy_lines)

    # Sell leg has Stamp duty == 0 and DP charge == 13.50
    assert any(l.item == "Stamp duty" and l.rounded_inr == 0 for l in sell_lines)
    assert any(l.item == "DP charge (ex-GST)" and l.rounded_inr == 13.50 for l in sell_lines)

    # Both legs have STT at 0.1%
    assert any(l.item == "STT" and l.rounded_inr == 500.00 for l in buy_lines)
    assert any(l.item == "STT" and l.rounded_inr == 520.00 for l in sell_lines)


def test_profitable_and_losing_trade_reconciliation():
    e = CostEngine()

    # Profitable trade: 100 -> 120 (+20% gross)
    c_win = e.calculate_trade("WIN", 100, 120, quantity=100)
    assert c_win.gross_pnl_reference == 2000.0
    assert c_win.net_pnl > 0
    assert c_win.net_pnl == pytest.approx(c_win.gross_pnl_reference - c_win.total_costs, abs=0.01)
    assert c_win.is_reconciled

    # Losing trade: 100 -> 80 (-20% gross)
    c_loss = e.calculate_trade("LOSS", 100, 80, quantity=100)
    assert c_loss.gross_pnl_reference == -2000.0
    assert c_loss.net_pnl < c_loss.gross_pnl_reference  # costs deepen the loss
    assert c_loss.net_pnl == pytest.approx(c_loss.gross_pnl_reference - c_loss.total_costs, abs=0.01)
    assert c_loss.is_reconciled


def test_5_lakh_reliance_exact_paisa_reconciliation():
    e = CostEngine()
    c = e.calculate_trade("RELIANCE", 2500, 2600, quantity=200)
    assert c.gross_pnl_reference == 20000.00
    assert c.statutory_total == 1134.41
    assert c.broker_total == 13.50
    assert c.modeled_slippage_total == 510.00
    assert c.total_costs == 1657.91
    assert c.net_pnl == 18342.09
    assert c.is_reconciled
    assert c.reconciliation_discrepancy < 1e-9


def test_basis_point_denominators_explicit_reconciliation():
    """
    WS5 Requirement 2, 3:
    Explicitly define and reconcile the two distinct basis-point denominators:
    1. Entry Reference Notional (q * pb = 200 * 2500 = ₹5,00,000):
       effective_cost_bps = (1,657.91 / 5,00,000) * 10,000 = 33.1582 bps ≈ 33.16 bps.
    2. Total Round-Trip Reference Turnover (q * pb + q * ps = 5,00,000 + 5,20,000 = ₹10,20,000):
       roundtrip_turnover_bps = (1,657.91 / 10,20,000) * 10,000 = 16.2540 bps ≈ 16.25 bps.
    3. Filled Turnover (q * fill_b + q * fill_s = 500,250 + 519,740 = ₹10,19,990):
       filled_turnover_bps = (1,657.91 / 10,19,990) * 10,000 = 16.2541 bps ≈ 16.25 bps.
    """
    e = CostEngine()
    c = e.calculate_trade("RELIANCE", 2500, 2600, quantity=200)

    # 1. Entry Notional
    entry_notional = 200 * 2500.0
    assert entry_notional == 500000.0
    entry_bps = (c.total_costs / entry_notional) * 10000
    assert round(entry_bps, 2) == 33.16
    assert round(c.effective_cost_bps, 2) == 33.16

    # 2. Total Round-Trip Reference Turnover
    exit_notional = 200 * 2600.0
    assert exit_notional == 520000.0
    roundtrip_ref_turnover = entry_notional + exit_notional
    assert roundtrip_ref_turnover == 1020000.0
    roundtrip_ref_bps = (c.total_costs / roundtrip_ref_turnover) * 10000
    assert round(roundtrip_ref_bps, 2) == 16.25

    # 3. Total Filled Turnover
    total_fill_turnover = c.buy_turnover + c.sell_turnover
    assert total_fill_turnover == 1019990.00
    fill_turnover_bps = (c.total_costs / total_fill_turnover) * 10000
    assert round(fill_turnover_bps, 2) == 16.25


def test_buy_only_and_sell_only_isolated_legs():
    """
    WS5 Requirement 8:
    Verify fees and taxes on buy-only and sell-only legs independently.
    """
    e = CostEngine()

    # Buy leg of ₹5,00,000
    buy_turnover = D("500000.00")
    buy_lines = e.leg_lines(buy_turnover, "BUY")
    stt_buy = next(l for l in buy_lines if l.item == "STT")
    stamp_buy = next(l for l in buy_lines if l.item == "Stamp duty")
    dp_buy = next(l for l in buy_lines if l.item == "DP charge (ex-GST)")
    assert stt_buy.rounded_inr == 500.00
    assert stamp_buy.rounded_inr == 75.00  # 0.015%
    assert dp_buy.rounded_inr == 0.00      # DP charge never applies on BUY

    # Sell leg of ₹5,20,000
    sell_turnover = D("520000.00")
    sell_lines = e.leg_lines(sell_turnover, "SELL")
    stt_sell = next(l for l in sell_lines if l.item == "STT")
    stamp_sell = next(l for l in sell_lines if l.item == "Stamp duty")
    dp_sell = next(l for l in sell_lines if l.item == "DP charge (ex-GST)")
    gst_sell = next(l for l in sell_lines if l.item == "GST")
    assert stt_sell.rounded_inr == 520.00
    assert stamp_sell.rounded_inr == 0.00  # Stamp duty never applies on cash equity SELL
    assert dp_sell.rounded_inr == 13.50    # DP debit charge applies on SELL
    # GST on sell: 18% of (Exch txn 15.44 + SEBI 0.52 + DP 13.50 = 29.4644) -> 5.30
    assert gst_sell.rounded_inr == 5.30


def test_partial_exit_accounting_linearity():
    """
    WS5 Requirement 8:
    Verify cost calculations for partial exits (e.g. 50% exit at 2600, 50% exit at 2700).
    Statutory turnover fees scale linearly with leg volume, while per-event DP charges apply per sell.
    """
    e = CostEngine()
    # Scenario A: Full exit of 200 shares at 2600
    c_full = e.calculate_trade("X", 2500, 2600, quantity=200)

    # Scenario B: Two separate partial exits of 100 shares each at 2600
    c_part1 = e.calculate_trade("X", 2500, 2600, quantity=100)
    c_part2 = e.calculate_trade("X", 2500, 2600, quantity=100)

    # Sum of variable costs (STT, stamp, exch txn, sebi) must foot linearly
    stt_part_total = (c_part1.total_for("STT") + c_part2.total_for("STT"))
    assert stt_part_total == pytest.approx(c_full.total_for("STT"), abs=0.02)

    # Fixed DP charge applies on each separate sell event (2 * 13.50 = 27.00 vs 13.50)
    dp_part_total = (c_part1.total_for("DP charge (ex-GST)") + c_part2.total_for("DP charge (ex-GST)"))
    assert dp_part_total == 27.00
    assert c_full.total_for("DP charge (ex-GST)") == 13.50


# ==============================================================================
# HalalEquity P1.8.6 WS1: Explicit Dual Accounting Identities & Regression Tests
# ==============================================================================

def test_p186_explicit_dual_pnl_identities_and_no_double_count():
    """
    P1.8.6 WS1 Reference Example:
    Quantity: 200 shares
    Reference entry: ₹2,500
    Reference exit: ₹2,600
    Entry fill: ₹2,501.25 (5 bps buy slippage)
    Exit fill: ₹2,598.70 (5 bps sell slippage)
    Modeled entry slippage: ₹250.00
    Modeled exit slippage: ₹260.00
    Total modeled slippage: ₹510.00
    Reported statutory fees: ₹1,134.41
    Reported broker/DP fees: ₹13.50
    Reported total friction: ₹1,657.91

    Enforces two explicit, separately tested identities:
    Identity 1 (Reference-price basis):
      net_pnl = reference_pnl - slippage_cost - statutory_fees - broker_fees
      20,000.00 - 510.00 - 1,134.41 - 13.50 = 18,342.09

    Identity 2 (Fill-price basis):
      net_pnl = quantity * (exit_fill - entry_fill) - statutory_fees - broker_fees
      200 * (2,598.70 - 2,501.25) - 1,134.41 - 13.50
      = 19,490.00 - 1,147.91 = 18,342.09

    Asserts that both identities yield exactly ₹18,342.09 and match c.net_pnl.
    """
    e = CostEngine()
    c = e.calculate_trade("RELIANCE", 2500, 2600, quantity=200)

    # 1. Exact price points and slippage components
    assert c.reference_buy_price == 2500.00
    assert c.reference_sell_price == 2600.00
    assert float(c.fill_buy_price) == 2501.25
    assert float(c.fill_sell_price) == 2598.70
    assert c.modeled_slippage_total == 510.00
    assert c.statutory_total == 1134.41
    assert c.broker_total == 13.50
    assert c.total_costs == 1657.91
    assert c.fees_only_total == 1147.91

    # 2. Identity 1: Reference-price basis
    ref_pnl = c.quantity * (c.reference_sell_price - c.reference_buy_price)  # 20,000.00
    assert ref_pnl == 20000.00
    id1_net = ref_pnl - c.modeled_slippage_total - c.statutory_total - c.broker_total
    assert round(id1_net, 2) == 18342.09
    assert c.net_pnl_reference_basis == 18342.09

    # 3. Identity 2: Fill-price basis (Slippage is NOT subtracted again)
    fill_pnl = c.quantity * (float(c.fill_sell_price) - float(c.fill_buy_price))  # 19,490.00
    assert round(fill_pnl, 2) == 19490.00
    assert c.fill_gross_pnl == 19490.00
    id2_net = fill_pnl - c.statutory_total - c.broker_total
    assert round(id2_net, 2) == 18342.09
    assert c.net_pnl_fill_basis == 18342.09

    # 4. Identity Equivalence & Double-Count Guard
    assert c.net_pnl == 18342.09
    assert c.net_pnl_reference_basis == c.net_pnl_fill_basis
    assert c.is_double_count_free is True


def test_p186_slippage_double_count_regression_detection():
    """
    P1.8.6 WS1 Regression Test:
    Fails if slippage is erroneously subtracted from fill-price gross P&L.
    Fill prices (2,501.25 and 2,598.70) ALREADY incorporate the ₹510 slippage cost.
    If a flawed implementation computes:
      fill_gross_pnl - slippage_cost - statutory_fees - broker_fees
    it yields ₹17,832.09, which differs by exactly ₹510 (the double-counted slippage).
    """
    e = CostEngine()
    c = e.calculate_trade("RELIANCE", 2500, 2600, quantity=200)

    # Flawed calculation that double-counts slippage
    flawed_double_count_net = c.fill_gross_pnl - c.modeled_slippage_total - c.fees_only_total
    assert round(flawed_double_count_net, 2) == 17832.09

    # True net P&L must NOT equal the double-counted value
    assert c.net_pnl != flawed_double_count_net
    assert round(c.net_pnl - flawed_double_count_net, 2) == 510.00
    assert round(c.net_pnl - flawed_double_count_net, 2) == c.modeled_slippage_total


def test_p186_adverse_price_movement_loss_identities():
    """
    WS1 Requirement: Verify adverse price movement (losing trade).
    Trade: Buy 200 @ ₹2,500; Sell 200 @ ₹2,400 (-₹20,000 reference gross).
    Entry fill: ₹2,501.25 (slip = +₹250.00 adverse)
    Exit fill: ₹2,398.80 (slip = +₹240.00 adverse)
    Total slippage: ₹490.00
    Fill gross P&L: 200 * (2,398.80 - 2,501.25) = -₹20,490.00
    Both identities must foot to the exact same expanded loss.
    """
    e = CostEngine()
    c = e.calculate_trade("RELIANCE", 2500, 2400, quantity=200)

    assert c.gross_pnl_reference == -20000.00
    assert c.modeled_slippage_total == 490.00  # 250 buy + 240 sell
    assert c.fill_gross_pnl == -20490.00

    # Identity 1: -20,000 - 490 - fees
    assert c.net_pnl_reference_basis == pytest.approx(c.net_pnl, abs=0.01)
    # Identity 2: -20,490 - fees
    assert c.net_pnl_fill_basis == pytest.approx(c.net_pnl, abs=0.01)
    assert c.is_double_count_free is True
    assert c.net_pnl < -20000.00  # Losses deepened by costs and slippage


def test_p186_zero_slippage_identity_equivalence():
    """
    When slippage is zero, fill prices equal reference prices.
    Identity 1 and Identity 2 trivially coincide line-for-line.
    """
    e = CostEngine(CostModelConfig(slippage=SlippageModel(buy_bps=D(0), sell_bps=D(0))))
    c = e.calculate_trade("X", 1000, 1100, quantity=50)

    assert c.modeled_slippage_total == 0.00
    assert float(c.fill_buy_price) == 1000.00
    assert float(c.fill_sell_price) == 1100.00
    assert c.fill_gross_pnl == c.gross_pnl_reference
    assert c.net_pnl_reference_basis == c.net_pnl_fill_basis == c.net_pnl


def test_p186_zero_quantity_and_negative_prices_guard():
    """
    WS1 Requirement: Verify zero quantities and invalid prices raise ValueError.
    """
    e = CostEngine()
    with pytest.raises(ValueError, match="quantity must be >= 1"):
        e.calculate_trade("X", 2500, 2600, quantity=0)

    with pytest.raises(ValueError, match="prices must be positive"):
        e.calculate_trade("X", -2500, 2600, quantity=100)

    with pytest.raises(ValueError, match="prices must be positive"):
        e.calculate_trade("X", 2500, -2600, quantity=100)




