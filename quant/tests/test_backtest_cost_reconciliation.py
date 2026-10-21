"""Portfolio-level reconciliation: sum of trade P&L (net of ledger fees) must equal change in capital."""
import pytest
from src.engines.backtest.engine import BacktestEngine, BacktestConfig, DailyData


def bars():
    rows = [(100, 100, 50), (100, 100, 85), (100, 110, 80), (110, 120, 35), (120, 120, 30), (120, 118, 20)]
    return [DailyData(timestamp=f"2024-01-0{i+1}", open=o, high=max(o, c) + 3, low=min(o, c) - 3, close=c, volume=1000, signal_score=s)
            for i, (o, c, s) in enumerate(rows)]


@pytest.mark.parametrize("slip", [0.0, 0.05, 0.1])
def test_sum_trade_pnl_equals_capital_change(slip):
    r = BacktestEngine(BacktestConfig(initial_capital=100000, slippage_percent=slip)).run({"T": bars()}, 80, 45)
    assert r.total_trades >= 1
    assert round(sum(t.pnl for t in r.trades), 2) == pytest.approx(r.final_capital - r.initial_capital, abs=0.01 * len(r.trades) + 0.011)
    for t in r.trades:
        assert t.fees_paid > 0
        # fees recomputed from the shared ledger equal what the trade booked
        e = BacktestEngine(BacktestConfig())
        assert t.fees_paid == pytest.approx(e.calculate_fees(t.shares * t.entry_price, True) + e.calculate_fees(t.shares * t.exit_price, False), abs=0.011)


def test_engine_fees_match_cost_engine_for_same_trade():
    from src.engines.research.cost_model import CostEngine, CostModelConfig, SlippageModel
    from decimal import Decimal as D
    e = BacktestEngine(BacktestConfig())
    buy, sell = 100 * 1000.0, 100 * 1080.0
    ce = CostEngine(CostModelConfig(slippage=SlippageModel(buy_bps=D(0), sell_bps=D(0), ))).calculate_trade("X", 1000, 1080, quantity=100)
    assert e.calculate_fees(buy, True) + e.calculate_fees(sell, False) == pytest.approx(ce.statutory_total + ce.broker_total, abs=0.011)
