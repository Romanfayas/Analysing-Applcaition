"""Quantitative analytics routes.

None of these endpoints is implemented. They answer 501 rather than returning an empty-but-successful
body, so no caller can mistake "not built" for "calculated, and nothing to report".
"""
from fastapi import APIRouter, HTTPException

router = APIRouter()


def _not_implemented(feature: str) -> HTTPException:
    return HTTPException(status_code=501, detail=f"{feature} is not implemented")


@router.post("/returns")
async def calculate_returns(data: dict):
    """Return metrics (absolute, daily, CAGR, rolling)."""
    raise _not_implemented("returns analytics")


@router.post("/risk")
async def calculate_risk_metrics(data: dict):
    """Risk metrics (volatility, VaR, CVaR, drawdown). See /api/v1/risk/metrics for the implemented subset."""
    raise _not_implemented("quantitative risk analytics")


@router.post("/ratios")
async def calculate_ratios(data: dict):
    """Risk-adjusted ratios (Sharpe, Sortino, Calmar)."""
    raise _not_implemented("risk-adjusted ratio analytics")


@router.post("/market-relationship")
async def calculate_market_relationship(data: dict):
    """Alpha, beta, correlation, tracking error (would require an index price series, which is not stored)."""
    raise _not_implemented("market-relationship analytics")
