"""
IPO Analysis API Routes
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import Optional

from src.engines.ipo.predictor import IPOPredictor, IPOData, IPODecision

router = APIRouter()


class IPOAnalyzeRequest(BaseModel):
    issue_price: float
    gmp: Optional[float] = None
    qib_subscription: Optional[float] = None
    nii_subscription: Optional[float] = None
    retail_subscription: Optional[float] = None
    issue_size_crores: Optional[float] = None
    shariah_status: str = "PASS"


@router.post("/analyze")
async def analyze_ipo(request: IPOAnalyzeRequest):
    """Predict IPO listing price and provide APPLY/WATCH/AVOID decision."""
    try:
        predictor = IPOPredictor()
        data = IPOData(**request.dict())
        result = predictor.predict(data)
        
        return {
            "decision": result.decision.value,
            "base_listing_price": result.base_listing_price,
            "bull_listing_price": result.bull_listing_price,
            "bear_listing_price": result.bear_listing_price,
            "expected_gain_percent": result.expected_gain_percent,
            "shariah_override": result.shariah_override,
            "explanation": result.explanation
        }
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
