"""
Risk Engine API Routes
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import List, Optional

from src.engines.risk.engine import RiskEngine

router = APIRouter()


class PositionSizeRequest(BaseModel):
    capital: float
    risk_percentage: float
    entry_price: float
    stop_loss: float
    target_price: Optional[float] = None


class RiskMetricsRequest(BaseModel):
    returns: List[float]


@router.post("/position_size")
async def calculate_position_size(request: PositionSizeRequest):
    """Calculate recommended position size based on risk parameters."""
    try:
        engine = RiskEngine()
        result = engine.calculate_position_size(
            capital=request.capital,
            risk_percentage=request.risk_percentage,
            entry_price=request.entry_price,
            stop_loss=request.stop_loss,
            target_price=request.target_price
        )
        
        return {
            "shares": result.position_size,
            "max_risk_amount": result.max_risk_amount,
            "position_value": result.position_value,
            "risk_per_share": result.risk_per_share,
            "risk_reward_ratio": result.risk_reward_ratio
        }
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/metrics")
async def calculate_risk_metrics(request: RiskMetricsRequest):
    """Calculate portfolio risk metrics from a series of returns."""
    try:
        engine = RiskEngine()
        result = engine.calculate_risk_metrics(request.returns)
        
        return {
            "volatility_daily": result.volatility_daily,
            "volatility_annual": result.volatility_annual,
            "max_drawdown": result.max_drawdown,
            "var_95": result.var_95,
            "sharpe_ratio": result.sharpe_ratio
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


class ClosedTradeIn(BaseModel):
    pnl: float
    invested: float
    fees: float = 0.0
    holding_days: Optional[float] = None
    benchmark_return_pct: Optional[float] = None


class PerformanceRequest(BaseModel):
    trades: List[ClosedTradeIn]
    initial_capital: float


@router.post("/performance")
async def performance_report_endpoint(request: PerformanceRequest):
    """Bootstrap-backed performance statistics for a list of closed trades (P1.9.26)."""
    from src.engines.research.performance import ClosedTrade, performance_report

    if request.initial_capital <= 0:
        raise HTTPException(status_code=400, detail="initial_capital must be positive")
    if len(request.trades) > 5000:
        raise HTTPException(status_code=400, detail="too many trades")
    try:
        trades = [ClosedTrade(pnl=t.pnl, invested=t.invested, fees=t.fees, holding_days=t.holding_days,
                              benchmark_return_pct=t.benchmark_return_pct) for t in request.trades]
        return performance_report(trades, request.initial_capital)
    except Exception as e:  # noqa: BLE001
        raise HTTPException(status_code=500, detail=str(e))
