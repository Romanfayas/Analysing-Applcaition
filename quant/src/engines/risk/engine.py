"""
Risk Engine — Position Sizing & Risk Management

Calculates:
- Position size based on capital, risk%, entry, and stop loss
- Risk/reward ratios
- Portfolio-level risk metrics (VaR, max drawdown)
- Stop loss suggestions (ATR-based, percentage-based)

CONSTRAINT: Never recommends a position larger than available capital.
"""

from dataclasses import dataclass
from typing import Optional
import numpy as np


@dataclass
class PositionSizeResult:
    """Result of position size calculation."""
    capital: float
    risk_percentage: float
    max_risk_amount: float
    entry_price: float
    stop_loss: float
    target_price: Optional[float]
    risk_per_share: float
    position_size: int  # Rounded down to whole shares
    position_value: float
    risk_reward_ratio: Optional[float]
    capital_utilization_pct: float
    note: str = ""


@dataclass
class RiskMetrics:
    """Portfolio/stock risk metrics."""
    volatility_daily: Optional[float] = None
    volatility_annual: Optional[float] = None
    max_drawdown: Optional[float] = None
    max_drawdown_duration_days: Optional[int] = None
    var_95: Optional[float] = None
    var_99: Optional[float] = None
    cvar_95: Optional[float] = None
    beta: Optional[float] = None
    sharpe_ratio: Optional[float] = None
    sortino_ratio: Optional[float] = None
    calmar_ratio: Optional[float] = None


class RiskEngine:
    """
    Position sizing and risk management calculations.

    CONSTRAINT: position_value never exceeds available capital.
    """

    def calculate_position_size(
        self,
        capital: float,
        risk_percentage: float,
        entry_price: float,
        stop_loss: float,
        target_price: Optional[float] = None,
    ) -> PositionSizeResult:
        """
        Calculate optimal position size.

        Formula:
            max_risk = capital × risk_percentage / 100
            risk_per_share = |entry_price - stop_loss|
            position_size = floor(max_risk / risk_per_share)
            position_value = position_size × entry_price

        Constraint: position_value ≤ capital
        """
        if capital <= 0:
            raise ValueError("Capital must be positive")
        if risk_percentage <= 0 or risk_percentage > 100:
            raise ValueError("Risk percentage must be between 0 and 100")
        if entry_price <= 0:
            raise ValueError("Entry price must be positive")
        if stop_loss <= 0:
            raise ValueError("Stop loss must be positive")
        if stop_loss >= entry_price:
            raise ValueError("Stop loss must be below entry price for a long position")

        max_risk = capital * risk_percentage / 100
        risk_per_share = abs(entry_price - stop_loss)

        if risk_per_share == 0:
            raise ValueError("Risk per share cannot be zero")

        # Calculate raw position size
        raw_position = max_risk / risk_per_share
        position_size = int(raw_position)  # Floor to whole shares

        # CONSTRAINT: Never exceed available capital
        max_affordable = int(capital / entry_price)
        position_size = min(position_size, max_affordable)

        # Ensure at least 0 shares
        position_size = max(position_size, 0)

        position_value = position_size * entry_price
        capital_util = (position_value / capital * 100) if capital > 0 else 0

        # Risk/reward ratio
        risk_reward = None
        note = ""
        if target_price is not None and target_price > entry_price:
            reward_per_share = target_price - entry_price
            if risk_per_share > 0:
                risk_reward = round(reward_per_share / risk_per_share, 2)
                if risk_reward < 2:
                    note = "Risk/reward ratio below 2:1 — consider widening target or tightening stop loss."

        if position_size == 0:
            note = "Position size is 0 — insufficient capital or risk too tight."

        return PositionSizeResult(
            capital=capital,
            risk_percentage=risk_percentage,
            max_risk_amount=round(max_risk, 2),
            entry_price=entry_price,
            stop_loss=stop_loss,
            target_price=target_price,
            risk_per_share=round(risk_per_share, 2),
            position_size=position_size,
            position_value=round(position_value, 2),
            risk_reward_ratio=risk_reward,
            capital_utilization_pct=round(capital_util, 2),
            note=note,
        )

    def calculate_risk_metrics(
        self,
        returns: list[float],
        benchmark_returns: Optional[list[float]] = None,
        risk_free_rate: float = 0.065,  # 6.5% — India 10Y govt bond
        trading_days: int = 252,
    ) -> RiskMetrics:
        """
        Calculate comprehensive risk metrics from daily returns.

        All calculations are deterministic.
        """
        if len(returns) < 2:
            return RiskMetrics()

        r = np.array(returns, dtype=np.float64)

        # Volatility
        vol_daily = float(np.std(r, ddof=1))
        vol_annual = vol_daily * np.sqrt(trading_days)

        # Max Drawdown
        cumulative = np.cumprod(1 + r)
        running_max = np.maximum.accumulate(cumulative)
        drawdowns = (cumulative - running_max) / running_max
        max_dd = float(np.min(drawdowns))

        # Drawdown duration
        dd_duration = 0
        current_dd = 0
        for i in range(len(drawdowns)):
            if drawdowns[i] < 0:
                current_dd += 1
                dd_duration = max(dd_duration, current_dd)
            else:
                current_dd = 0

        # VaR and CVaR
        var_95 = float(np.percentile(r, 5))
        var_99 = float(np.percentile(r, 1))
        cvar_95 = float(np.mean(r[r <= var_95])) if np.any(r <= var_95) else var_95

        # Sharpe Ratio
        excess_return = np.mean(r) * trading_days - risk_free_rate
        sharpe = excess_return / vol_annual if vol_annual > 0 else None

        # Sortino Ratio (downside deviation)
        downside = r[r < 0]
        downside_std = float(np.std(downside, ddof=1)) * np.sqrt(trading_days) if len(downside) > 1 else None
        sortino = excess_return / downside_std if downside_std and downside_std > 0 else None

        # Calmar Ratio
        annual_return = np.mean(r) * trading_days
        calmar = annual_return / abs(max_dd) if max_dd != 0 else None

        # Beta (if benchmark provided)
        beta = None
        if benchmark_returns is not None and len(benchmark_returns) == len(returns):
            b = np.array(benchmark_returns, dtype=np.float64)
            cov = np.cov(r, b)
            var_b = np.var(b, ddof=1)
            if var_b > 0:
                beta = float(cov[0][1] / var_b)

        return RiskMetrics(
            volatility_daily=round(vol_daily, 6),
            volatility_annual=round(vol_annual, 6),
            max_drawdown=round(max_dd, 6),
            max_drawdown_duration_days=dd_duration,
            var_95=round(var_95, 6),
            var_99=round(var_99, 6),
            cvar_95=round(cvar_95, 6),
            beta=round(beta, 4) if beta is not None else None,
            sharpe_ratio=round(sharpe, 4) if sharpe is not None else None,
            sortino_ratio=round(sortino, 4) if sortino is not None else None,
            calmar_ratio=round(calmar, 4) if calmar is not None else None,
        )

    def suggest_stop_loss(
        self,
        entry_price: float,
        atr: float,
        method: str = "atr",
        multiplier: float = 2.0,
        percentage: float = 5.0,
    ) -> dict:
        """
        Suggest stop loss levels.

        Methods:
        - "atr": Entry - (ATR × multiplier)
        - "percentage": Entry × (1 - percentage/100)
        """
        if method == "atr":
            sl = entry_price - (atr * multiplier)
        elif method == "percentage":
            sl = entry_price * (1 - percentage / 100)
        else:
            raise ValueError(f"Unknown method: {method}")

        return {
            "entry_price": entry_price,
            "stop_loss": round(max(sl, 0), 2),
            "method": method,
            "risk_pct": round(((entry_price - sl) / entry_price) * 100, 2),
        }
