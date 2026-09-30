"""
Unit tests for Risk Engine.
"""

import pytest
from src.engines.risk.engine import RiskEngine


@pytest.fixture
def engine():
    return RiskEngine()


class TestPositionSizing:
    def test_basic_position_size(self, engine):
        """Standard position size calculation."""
        result = engine.calculate_position_size(
            capital=50000,
            risk_percentage=1,
            entry_price=500,
            stop_loss=475,
        )
        # max_risk = 50000 * 0.01 = 500
        # risk_per_share = 500 - 475 = 25
        # position_size = 500 / 25 = 20
        assert result.position_size == 20
        assert result.risk_per_share == 25
        assert result.max_risk_amount == 500
        assert result.position_value == 10000

    def test_position_capped_by_capital(self, engine):
        """Position value must never exceed available capital."""
        result = engine.calculate_position_size(
            capital=5000,
            risk_percentage=10,
            entry_price=500,
            stop_loss=495,
        )
        # max_risk = 500, risk_per_share = 5, raw_position = 100
        # But 100 * 500 = 50000 > 5000 capital
        # So capped: 5000 / 500 = 10 shares
        assert result.position_size == 10
        assert result.position_value <= 5000

    def test_risk_reward_ratio(self, engine):
        """Risk/reward should be calculated when target is provided."""
        result = engine.calculate_position_size(
            capital=50000,
            risk_percentage=1,
            entry_price=500,
            stop_loss=475,
            target_price=575,
        )
        # reward = 575 - 500 = 75
        # risk = 500 - 475 = 25
        # R:R = 75 / 25 = 3.0
        assert result.risk_reward_ratio == 3.0

    def test_zero_capital_raises(self, engine):
        """Zero capital should raise ValueError."""
        with pytest.raises(ValueError):
            engine.calculate_position_size(0, 1, 500, 475)

    def test_stop_loss_above_entry_raises(self, engine):
        """Stop loss above entry should raise ValueError."""
        with pytest.raises(ValueError):
            engine.calculate_position_size(50000, 1, 500, 510)


class TestRiskMetrics:
    def test_basic_risk_metrics(self, engine):
        """Risk metrics should be calculated from returns."""
        # Simulate daily returns
        import numpy as np
        np.random.seed(42)
        returns = np.random.normal(0.001, 0.02, 252).tolist()

        result = engine.calculate_risk_metrics(returns)
        assert result.volatility_daily is not None
        assert result.volatility_annual is not None
        assert result.max_drawdown is not None
        assert result.max_drawdown <= 0  # Drawdown is negative
        assert result.var_95 is not None
        assert result.sharpe_ratio is not None

    def test_insufficient_data(self, engine):
        """Fewer than 2 returns should return empty metrics."""
        result = engine.calculate_risk_metrics([0.01])
        assert result.volatility_daily is None


class TestStopLoss:
    def test_atr_stop_loss(self, engine):
        """ATR-based stop loss calculation."""
        result = engine.suggest_stop_loss(
            entry_price=500,
            atr=20,
            method="atr",
            multiplier=2.0,
        )
        # SL = 500 - (20 * 2) = 460
        assert result["stop_loss"] == 460

    def test_percentage_stop_loss(self, engine):
        """Percentage-based stop loss calculation."""
        result = engine.suggest_stop_loss(
            entry_price=500,
            atr=20,
            method="percentage",
            percentage=5,
        )
        # SL = 500 * 0.95 = 475
        assert result["stop_loss"] == 475
