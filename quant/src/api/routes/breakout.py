"""
Breakout Research API Routes

Provides endpoints for scanning and analyzing cash-equity breakout setups.
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import Optional, List, Dict, Any

from src.engines.technical.breakout import BreakoutEngine, BreakoutResult

router = APIRouter()


class BreakoutAnalyzeRequest(BaseModel):
    symbol: str
    open: List[float]
    high: List[float]
    low: List[float]
    close: List[float]
    volume: List[int]
    shariah_status: Optional[str] = "UNKNOWN"
    benchmark_closes: Optional[List[float]] = None
    data_quality: Optional[str] = "LAST_AVAILABLE"


class StockScanItem(BaseModel):
    symbol: str
    sector: Optional[str] = ""
    open: List[float]
    high: List[float]
    low: List[float]
    close: List[float]
    volume: List[int]
    shariah_status: Optional[str] = "UNKNOWN"
    data_quality: Optional[str] = "LAST_AVAILABLE"


class BreakoutScanRequest(BaseModel):
    stocks: Optional[List[StockScanItem]] = []
    benchmark_closes: Optional[List[float]] = None


@router.post("/analyze")
async def analyze_breakout(req: BreakoutAnalyzeRequest):
    """Analyze a single stock for breakout conditions."""
    try:
        engine = BreakoutEngine(
            symbol=req.symbol,
            open_prices=req.open,
            high_prices=req.high,
            low_prices=req.low,
            close_prices=req.close,
            volumes=req.volume,
            shariah_status=req.shariah_status or "UNKNOWN",
            benchmark_closes=req.benchmark_closes,
            data_quality=req.data_quality or "LAST_AVAILABLE",
        )
        res = engine.analyze()
        return res.__dict__
    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/scan")
async def scan_universe(req: BreakoutScanRequest):
    """Scan an entire stock universe for breakout setups and market breadth."""
    try:
        results = []
        sector_scores: Dict[str, List[float]] = {}
        stage_counts = {
            "CONFIRMED_BREAKOUT": 0,
            "BREAKOUT_CANDIDATE": 0,
            "WATCHLIST": 0,
            "FAILED_BREAKOUT": 0,
            "UNAVAILABLE": 0,
        }

        for s in (req.stocks or []):
            engine = BreakoutEngine(
                symbol=s.symbol,
                open_prices=s.open,
                high_prices=s.high,
                low_prices=s.low,
                close_prices=s.close,
                volumes=s.volume,
                shariah_status=s.shariah_status or "UNKNOWN",
                benchmark_closes=req.benchmark_closes,
                data_quality=s.data_quality or "LAST_AVAILABLE",
            )
            res = engine.analyze()
            res_dict = res.__dict__
            res_dict["sector"] = s.sector
            results.append(res_dict)

            stage_counts[res.stage] = stage_counts.get(res.stage, 0) + 1

            if s.sector:
                if s.sector not in sector_scores:
                    sector_scores[s.sector] = []
                sector_scores[s.sector].append(res.overall_score)

        # Sort results: Confirmed breakouts & candidates first, then by score descending
        stage_order = {
            "CONFIRMED_BREAKOUT": 0,
            "BREAKOUT_CANDIDATE": 1,
            "WATCHLIST": 2,
            "FAILED_BREAKOUT": 3,
            "UNAVAILABLE": 4,
        }
        results.sort(key=lambda x: (stage_order.get(x["stage"], 5), -x["overall_score"]))

        # Compute sector strength heatmap
        sector_heatmap = []
        for sec, scores in sector_scores.items():
            avg_score = round(sum(scores) / len(scores), 1) if scores else 0.0
            sector_heatmap.append({
                "sector": sec,
                "average_score": avg_score,
                "count": len(scores),
                "strength": "STRONG" if avg_score >= 60 else ("MODERATE" if avg_score >= 40 else "WEAK"),
            })
        sector_heatmap.sort(key=lambda x: -x["average_score"])

        # Market breadth proxy
        total_valid = sum(stage_counts.values()) - stage_counts["UNAVAILABLE"]
        bullish_breadth = (
            round((stage_counts["CONFIRMED_BREAKOUT"] + stage_counts["BREAKOUT_CANDIDATE"]) / max(total_valid, 1) * 100, 1)
            if total_valid > 0
            else 0.0
        )

        setup_counts = {
            "APPROACHING_RESISTANCE": stage_counts.get("WATCHLIST", 0),
            "CONSOLIDATION": stage_counts.get("BREAKOUT_CANDIDATE", 0),
            "CONFIRMED_BREAKOUT": stage_counts.get("CONFIRMED_BREAKOUT", 0),
            "FAILED_BREAKOUT": stage_counts.get("FAILED_BREAKOUT", 0),
        }

        return {
            "total_screened": len(req.stocks),
            "stage_counts": stage_counts,
            "setup_counts": setup_counts,
            "bullish_breadth_percent": bullish_breadth,
            "sector_heatmap": sector_heatmap,
            "score_disclaimer": "The ranking score is a multi-factor structural evidence score, NOT a calibrated probability of profit.",
            "market_depth_disclosure": "Volume demand proxy is derived from bar volume distribution. Actual Level 2 order-book market depth is unavailable without a licensed exchange feed.",
            "candidates": results,
        }

    except Exception as e:
        raise HTTPException(status_code=400, detail=str(e))
