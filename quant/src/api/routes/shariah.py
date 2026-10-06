"""
Shariah Screening API Routes
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import Optional

from src.engines.shariah.screener import ShariahScreener
from src.engines.fundamental.analyzer import FinancialData

router = APIRouter()


class ShariahRequest(BaseModel):
    total_debt: Optional[float] = None
    cash_and_equivalents: Optional[float] = None
    interest_bearing_deposits: Optional[float] = None
    total_receivables: Optional[float] = None
    market_cap: Optional[float] = None
    total_assets: Optional[float] = None
    total_revenue: Optional[float] = None
    interest_income: Optional[float] = None
    non_compliant_revenue: Optional[float] = None
    shares_outstanding: Optional[float] = None
    is_financial_institution: bool = False
    is_prohibited_industry: bool = False


@router.post("/screen")
async def screen_stock(request: ShariahRequest):
    """Screen a stock against Shariah rules."""
    try:
        screener = ShariahScreener()
        data = FinancialData(**request.dict())
        result = screener.screen(data)
        
        return {
            "status": result.status.value,
            "debt_ratio": result.debt_ratio,
            "debt_ratio_pass": result.debt_ratio_pass,
            "interest_income_ratio": result.interest_income_ratio,
            "interest_income_pass": result.interest_income_pass,
            "cash_deposit_ratio": result.cash_deposit_ratio,
            "cash_deposit_pass": result.cash_deposit_pass,
            "receivables_ratio": result.receivables_ratio,
            "receivables_pass": result.receivables_pass,
            "business_activity_pass": result.business_activity_pass,
            "purification_per_share": result.purification_per_share,
            "disclaimer": result.disclaimer
        }
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))
