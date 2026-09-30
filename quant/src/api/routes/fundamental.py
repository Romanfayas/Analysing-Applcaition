"""
Fundamental Analysis API Routes
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import Optional

from src.engines.fundamental.analyzer import FundamentalAnalyzer, FinancialData

router = APIRouter()


class FundamentalRequest(BaseModel):
    total_assets: Optional[float] = None
    total_equity: Optional[float] = None
    total_debt: Optional[float] = None
    current_assets: Optional[float] = None
    current_liabilities: Optional[float] = None
    cash_and_equivalents: Optional[float] = None
    inventory: Optional[float] = None
    receivables: Optional[float] = None
    fixed_assets: Optional[float] = None
    total_shares_outstanding: Optional[float] = None
    revenue: Optional[float] = None
    operating_profit: Optional[float] = None
    net_profit: Optional[float] = None
    ebitda: Optional[float] = None
    interest_expense: Optional[float] = None
    depreciation: Optional[float] = None
    tax_expense: Optional[float] = None
    eps: Optional[float] = None
    dividend_per_share: Optional[float] = None
    operating_cash_flow: Optional[float] = None
    capex: Optional[float] = None
    free_cash_flow: Optional[float] = None
    market_cap: Optional[float] = None
    current_price: Optional[float] = None
    enterprise_value: Optional[float] = None
    prev_revenue: Optional[float] = None
    prev_net_profit: Optional[float] = None
    prev_eps: Optional[float] = None
    prev_total_assets: Optional[float] = None
    prev_current_assets: Optional[float] = None
    prev_current_liabilities: Optional[float] = None
    prev_total_debt: Optional[float] = None
    prev_total_equity: Optional[float] = None
    prev_operating_cash_flow: Optional[float] = None
    prev_gross_margin: Optional[float] = None
    prev_asset_turnover: Optional[float] = None
    prev_shares_outstanding: Optional[float] = None
    eps_growth_estimate: Optional[float] = None


@router.post("/metrics")
async def calculate_fundamental_metrics(request: FundamentalRequest):
    """Calculate fundamental metrics from financial statements."""
    try:
        analyzer = FundamentalAnalyzer()
        data = FinancialData(**request.dict())
        metrics = analyzer.analyze(data)
        score = analyzer.calculate_score(metrics)
        
        return {
            "score": score,
            "metrics": [
                {
                    "name": m.name,
                    "value": m.value,
                    "category": m.category,
                    "rating": m.rating,
                    "formula": m.formula,
                    "note": m.note
                } for m in metrics
            ]
        }
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))
