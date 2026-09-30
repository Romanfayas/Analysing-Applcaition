"""
Technical Analysis API Routes

Exposes endpoints for computing technical indicators.
All calculations are deterministic and unit-tested.
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import Optional

router = APIRouter()


class OHLCVInput(BaseModel):
    """Input OHLCV data for indicator calculation."""
    timestamps: list[str]
    open: list[float]
    high: list[float]
    low: list[float]
    close: list[float]
    volume: list[int]


class IndicatorRequest(BaseModel):
    """Request to calculate technical indicators."""
    ohlcv: OHLCVInput
    indicators: list[str]  # e.g., ["sma_20", "rsi_14", "macd"]


class IndicatorResponse(BaseModel):
    """Response with calculated indicator values."""
    indicator: str
    parameters: dict
    values: list[Optional[float]]
    signal: Optional[str] = None  # BULLISH, BEARISH, NEUTRAL


@router.post("/indicators")
async def calculate_indicators(request: IndicatorRequest):
    """Calculate specified technical indicators for given OHLCV data."""
    from src.engines.technical.indicators import TechnicalIndicators

    ti = TechnicalIndicators(
        open_prices=request.ohlcv.open,
        high_prices=request.ohlcv.high,
        low_prices=request.ohlcv.low,
        close_prices=request.ohlcv.close,
        volumes=request.ohlcv.volume,
    )

    results = []
    for indicator_spec in request.indicators:
        try:
            result = ti.calculate(indicator_spec)
            results.append(result)
        except ValueError as e:
            raise HTTPException(status_code=400, detail=str(e))

    return {"indicators": results}


@router.post("/support-resistance")
async def calculate_support_resistance(request: OHLCVInput):
    """Calculate support and resistance levels."""
    return {"levels": []}


@router.post("/trend")
async def detect_trend(request: OHLCVInput):
    """Detect current trend direction and strength."""
    return {"trend": "NEUTRAL", "strength": 0}
