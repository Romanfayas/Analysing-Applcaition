"""
Candlestick Pattern Detection API Routes
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import List

from src.engines.candlestick.patterns import CandlestickEngine

router = APIRouter()


class OHLCVInput(BaseModel):
    open: List[float]
    high: List[float]
    low: List[float]
    close: List[float]
    volume: List[int]


@router.post("/detect")
async def detect_patterns(request: OHLCVInput):
    """Detect candlestick patterns in OHLCV data."""
    try:
        engine = CandlestickEngine(
            open_prices=request.open,
            high_prices=request.high,
            low_prices=request.low,
            close_prices=request.close,
            volumes=request.volume,
        )
        patterns = engine.detect_all()
        
        return {
            "patterns": [
                {
                    "name": p.name,
                    "pattern_type": p.pattern_type,
                    "confidence": p.confidence,
                    "index": p.index,
                    "is_bullish": p.is_bullish,
                    "is_bearish": p.is_bearish,
                } for p in patterns
            ]
        }
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))
