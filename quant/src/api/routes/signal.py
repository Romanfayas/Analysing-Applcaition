"""
Signal Engine API Routes

Calculates composite weighted signals combining all engine dimensions.
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import Optional, List

from src.engines.signal.engine import SignalEngine, SignalWeights

router = APIRouter()


class SignalRequest(BaseModel):
    fundamental_score: Optional[float] = None
    technical_score: Optional[float] = None
    candlestick_score: Optional[float] = None
    momentum_score: Optional[float] = None
    volume_score: Optional[float] = None
    valuation_score: Optional[float] = None
    market_regime_score: Optional[float] = None
    risk_score: Optional[float] = None
    quality_score: Optional[float] = None
    shariah_status: str = "PASS"
    fundamental_factors: Optional[List[str]] = None
    technical_factors: Optional[List[str]] = None
    conflicting_factors: Optional[List[str]] = None


@router.post("/calculate")
async def calculate_signal(request: SignalRequest):
    """Calculate composite weighted signal for a stock."""
    try:
        engine = SignalEngine()
        result = engine.calculate(
            fundamental_score=request.fundamental_score,
            technical_score=request.technical_score,
            candlestick_score=request.candlestick_score,
            momentum_score=request.momentum_score,
            volume_score=request.volume_score,
            valuation_score=request.valuation_score,
            market_regime_score=request.market_regime_score,
            risk_score=request.risk_score,
            quality_score=request.quality_score,
            shariah_status=request.shariah_status,
            fundamental_factors=request.fundamental_factors,
            technical_factors=request.technical_factors,
            conflicting=request.conflicting_factors,
        )
        
        return {
            "signal": result.signal.value,
            "overall_score": result.overall_score,
            "dimensions": [
                {
                    "name": d.name,
                    "score": d.score,
                    "weight": d.weight
                } for d in result.dimensions
            ],
            "shariah_status": result.shariah_status,
            "shariah_override": result.shariah_override,
            "explanation": result.explanation,
            "supporting_factors": result.supporting_factors,
            "conflicting_factors": result.conflicting_factors,
            "confidence": result.confidence,
            "disclaimer": result.disclaimer
        }
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))
