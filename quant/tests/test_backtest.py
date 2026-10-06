"""
Unit tests for the Backtesting Engine.
"""

import pytest
from src.engines.backtest.engine import BacktestEngine, BacktestConfig, DailyData


@pytest.fixture
def sample_data():
    return [
        DailyData(timestamp="2024-01-01", open=100, high=105, low=95, close=100, volume=1000, signal_score=50),
        DailyData(timestamp="2024-01-02", open=100, high=105, low=95, close=100, volume=1000, signal_score=85), # Buy signal
        DailyData(timestamp="2024-01-03", open=100, high=115, low=100, close=110, volume=1000, signal_score=80),
        DailyData(timestamp="2024-01-04", open=110, high=125, low=105, close=120, volume=1000, signal_score=35), # Sell signal
        DailyData(timestamp="2024-01-05", open=120, high=125, low=115, close=120, volume=1000, signal_score=30),
    ]


class TestBacktestEngine:
    def test_basic_trade_execution(self, sample_data):
        """Test a complete buy and sell cycle."""
        config = BacktestConfig(initial_capital=10000, slippage_percent=0, transaction_fee_percent=0)
        engine = BacktestEngine(config)
        
        result = engine.run({"TEST": sample_data}, buy_threshold=80, sell_threshold=45)
        
        # 1 Trade completed
        assert result.total_trades == 1
        trade = result.trades[0]
        
        # Buy on 01-03 open (110)
        assert trade.entry_date == "2024-01-03"
        assert trade.entry_price == 100.0
        
        # Sell on 01-05 open (120)
        assert trade.exit_date == "2024-01-05"
        assert trade.exit_price == 120.0
        
        # Profit
        assert trade.pnl > 0
        assert result.win_rate_percent == 100.0
        assert result.final_capital > 10000

    def test_slippage_and_fees(self, sample_data):
        """Slippage and fees should reduce profitability."""
        config_no_fees = BacktestConfig(initial_capital=10000, slippage_percent=0, transaction_fee_percent=0)
        config_with_fees = BacktestConfig(initial_capital=10000, slippage_percent=1.0, transaction_fee_percent=1.0)
        
        engine_no_fees = BacktestEngine(config_no_fees)
        engine_with_fees = BacktestEngine(config_with_fees)
        
        res_no = engine_no_fees.run({"TEST": sample_data}, 80, 45)
        res_with = engine_with_fees.run({"TEST": sample_data}, 80, 45)
        
        assert res_with.final_capital < res_no.final_capital
        assert res_with.trades[0].fees_paid > 0
        assert res_with.trades[0].entry_price > 100.0 # Entry slippage pushes price up
        assert res_with.trades[0].exit_price < 120.0  # Exit slippage pushes price down

    def test_no_margin_allowed(self):
        """Capital cannot drop below zero."""
        config = BacktestConfig(initial_capital=50) # Very low capital
        engine = BacktestEngine(config)
        
        data = [
            DailyData(timestamp="2024-01-01", open=100, high=100, low=100, close=100, volume=100, signal_score=90),
            DailyData(timestamp="2024-01-02", open=100, high=100, low=100, close=100, volume=100, signal_score=30)
        ]
        
        result = engine.run({"TEST": data}, 80, 45)
        
        # Capital 50 is not enough to buy 1 share at 100 + slippage
        assert result.total_trades == 0
        assert result.final_capital == 50

    def test_force_close_at_end(self, sample_data):
        """An open position should be closed on the last day of the backtest."""
        # Never sell
        config = BacktestConfig(initial_capital=10000, slippage_percent=0, transaction_fee_percent=0)
        engine = BacktestEngine(config)
        
        result = engine.run({"TEST": sample_data}, buy_threshold=80, sell_threshold=-1) # Sell threshold impossible to hit
        
        assert result.total_trades == 1
        trade = result.trades[0]
        
        # Exit date must be the last day (2024-01-05)
        assert trade.exit_date == "2024-01-05"
