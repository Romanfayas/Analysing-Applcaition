"""
Backtesting API Routes
"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import List, Optional

from src.engines.backtest.engine import BacktestEngine, BacktestConfig, DailyData, CostModelConfig
from src.engines.backtest.walk_forward import WalkForwardEngine
from src.engines.signal.engine import SignalEngine

router = APIRouter()


class BacktestRequest(BaseModel):
    initial_capital: float = 100000.0
    slippage_percent: float = 0.1
    position_size_atr_multiplier: float = 2.0
    risk_per_trade_percent: float = 2.0
    buy_threshold: float = 80.0
    sell_threshold: float = 45.0
    cost_model: CostModelConfig = CostModelConfig()
    historical_data: dict[str, List[DailyData]]

class HistoricalPoint(BaseModel):
    timestamp: str
    open: float
    high: float
    low: float
    close: float
    volume: int
    technical_score: Optional[float] = None
    fundamental_score: Optional[float] = None
    revenue: Optional[float] = None
    pat: Optional[float] = None
    total_debt: Optional[float] = None
    total_equity: Optional[float] = None
    operating_cash_flow: Optional[float] = None
    fundamental_data_available: bool = False
    fundamental_pit_mode: str = "PIT_UNAVAILABLE"
    shariah_status: str = "HISTORICAL_DATA_INSUFFICIENT"
    benchmark_close: Optional[float] = None

class HistoricalBacktestRequest(BaseModel):
    initial_capital: float = 100000.0
    slippage_percent: float = 0.1
    position_size_atr_multiplier: float = 2.0
    risk_per_trade_percent: float = 2.0
    buy_threshold: float = 80.0
    sell_threshold: float = 45.0
    cost_model: CostModelConfig = CostModelConfig()
    historical_data: dict[str, List[HistoricalPoint]]


@router.post("/run")
async def run_backtest(request: BacktestRequest):
    """Run a deterministic backtest on historical data."""
    try:
        config = BacktestConfig(
            initial_capital=request.initial_capital,
            slippage_percent=request.slippage_percent,
            position_size_atr_multiplier=request.position_size_atr_multiplier,
            risk_per_trade_percent=request.risk_per_trade_percent,
            cost_model=request.cost_model
        )
        
        engine = BacktestEngine(config)
        result = engine.run(
            data=request.historical_data,
            buy_threshold=request.buy_threshold,
            sell_threshold=request.sell_threshold
        )
        
        return result.dict()
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@router.post("/run_historical")
async def run_historical_backtest(request: HistoricalBacktestRequest):
    """Run a deterministic backtest generating point-in-time signals."""
    try:
        daily_data = {}
        engine = SignalEngine()
        
        for sym, points in request.historical_data.items():
            daily_data[sym] = []
            for pt in points:
                if not pt.fundamental_data_available:
                    pt.shariah_status = "HISTORICAL_DATA_INSUFFICIENT"

                # Deterministic fundamental_score computation
                computed_fundamental_score = pt.fundamental_score
                if computed_fundamental_score is None:
                    if pt.revenue is not None and pt.pat is not None and pt.total_debt is not None and pt.total_equity is not None:
                        # Very naive example fundamental score
                        score = 50.0
                        if pt.pat > 0:
                            score += 20
                        if pt.total_equity > 0 and (pt.total_debt / pt.total_equity) < 1.0:
                            score += 30
                        computed_fundamental_score = score
                
                # Generate point-in-time signal
                sig_result = engine.calculate(
                    technical_score=pt.technical_score,
                    fundamental_score=computed_fundamental_score,
                    shariah_status=pt.shariah_status
                )
                
                daily_data[sym].append(DailyData(
                    timestamp=pt.timestamp,
                    open=pt.open,
                    high=pt.high,
                    low=pt.low,
                    close=pt.close,
                    volume=pt.volume,
                    signal_score=sig_result.overall_score
                ))

        config = BacktestConfig(
            initial_capital=request.initial_capital,
            slippage_percent=request.slippage_percent,
            position_size_atr_multiplier=request.position_size_atr_multiplier,
            risk_per_trade_percent=request.risk_per_trade_percent,
            cost_model=request.cost_model
        )
        
        b_engine = BacktestEngine(config)
        result = b_engine.run(
            data=daily_data,
            buy_threshold=request.buy_threshold,
            sell_threshold=request.sell_threshold
        )
        
        return result.dict()
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

class WalkForwardRequest(BaseModel):
    request: HistoricalBacktestRequest
    train_days: int = 504
    test_days: int = 252
    step_days: int = 252

@router.post("/run_walk_forward")
async def run_walk_forward_backtest(req: WalkForwardRequest):
    """Run a walk-forward optimization/validation."""
    try:
        daily_data = {}
        engine = SignalEngine()
        
        for sym, points in req.request.historical_data.items():
            daily_data[sym] = []
            for pt in points:
                if not pt.fundamental_data_available:
                    pt.shariah_status = "HISTORICAL_DATA_INSUFFICIENT"

                computed_fundamental_score = pt.fundamental_score
                if computed_fundamental_score is None:
                    if pt.revenue is not None and pt.pat is not None and pt.total_debt is not None and pt.total_equity is not None:
                        score = 50.0
                        if pt.pat > 0: score += 20
                        if pt.total_equity > 0 and (pt.total_debt / pt.total_equity) < 1.0: score += 30
                        computed_fundamental_score = score
                
                sig_result = engine.calculate(
                    technical_score=pt.technical_score,
                    fundamental_score=computed_fundamental_score,
                    shariah_status=pt.shariah_status
                )
                
                daily_data[sym].append(DailyData(
                    timestamp=pt.timestamp,
                    open=pt.open,
                    high=pt.high,
                    low=pt.low,
                    close=pt.close,
                    volume=pt.volume,
                    signal_score=sig_result.overall_score
                ))

        config = BacktestConfig(
            initial_capital=req.request.initial_capital,
            slippage_percent=req.request.slippage_percent,
            position_size_atr_multiplier=req.request.position_size_atr_multiplier,
            risk_per_trade_percent=req.request.risk_per_trade_percent,
            cost_model=req.request.cost_model
        )
        
        wf_engine = WalkForwardEngine(config)
        result = wf_engine.run(
            data=daily_data,
            train_days=req.train_days,
            test_days=req.test_days,
            step_days=req.step_days,
            buy_threshold=req.request.buy_threshold,
            sell_threshold=req.request.sell_threshold
        )
        
        return result.dict()
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
