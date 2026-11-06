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
        cash_and_deposits = 0.0
        if request.cash_and_equivalents is not None:
            cash_and_deposits += request.cash_and_equivalents
        if request.interest_bearing_deposits is not None:
            cash_and_deposits += request.interest_bearing_deposits

        result = screener.screen(
            total_debt=request.total_debt,
            market_cap=request.market_cap,
            interest_income=request.interest_income,
            revenue=request.total_revenue,
            cash_and_deposits=cash_and_deposits if request.cash_and_equivalents is not None or request.interest_bearing_deposits is not None else None,
            receivables=request.total_receivables,
        )
        
        # Format the response from RatioResults
        resp = {
            "status": result.status.value,
            "debt_ratio": None,
            "debt_ratio_pass": None,
            "interest_income_ratio": None,
            "interest_income_pass": None,
            "cash_deposit_ratio": None,
            "cash_deposit_pass": None,
            "receivables_ratio": None,
            "receivables_pass": None,
            "business_activity_pass": result.business_activity_pass,
            "purification_per_share": result.purification_per_share,
            "disclaimer": ""
        }
        
        for r in result.ratio_results:
            if r.name == "Debt/MarketCap":
                resp["debt_ratio"] = r.value
                resp["debt_ratio_pass"] = r.passed
            elif r.name == "InterestIncome/Revenue":
                resp["interest_income_ratio"] = r.value
                resp["interest_income_pass"] = r.passed
            elif r.name == "Cash+Deposits/MarketCap":
                resp["cash_deposit_ratio"] = r.value
                resp["cash_deposit_pass"] = r.passed
            elif r.name == "Receivables/MarketCap":
                resp["receivables_ratio"] = r.value
                resp["receivables_pass"] = r.passed
                
        return resp
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/screen-symbol/{symbol}")
async def screen_symbol(symbol: str):
    """Fetch reported figures for one stored symbol and store a fresh screening (used when a stock is added)."""
    from src.engines.fundamental.ingest_verified_fundamentals import screen_symbols

    ticker = symbol.strip().upper()
    try:
        result = await __import__("asyncio").get_running_loop().run_in_executor(None, screen_symbols, [ticker])
    except Exception as e:  # noqa: BLE001
        raise HTTPException(status_code=500, detail=f"screening failed: {e}")
    if ticker not in result:
        raise HTTPException(status_code=404, detail=f"{ticker} is not an active stored symbol")
    return {"symbol": ticker, "status": result[ticker]}
