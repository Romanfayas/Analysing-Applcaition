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
