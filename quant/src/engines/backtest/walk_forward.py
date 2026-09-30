from typing import List, Tuple
from datetime import datetime
from pydantic import BaseModel
from src.engines.backtest.engine import BacktestEngine, BacktestConfig, DailyData, BacktestResult

class WalkForwardWindow(BaseModel):
    train_start: str
    train_end: str
    test_start: str
    test_end: str
    test_result: BacktestResult

class WalkForwardResult(BaseModel):
    total_oos_return_percent: float
    average_oos_win_rate: float
    average_oos_max_drawdown: float
    windows: List[WalkForwardWindow]

class WalkForwardEngine:
    """
    Performs walk-forward optimization/validation to prevent overfitting.
    Splits historical data into sliding Train and Test windows.
    """
    def __init__(self, config: BacktestConfig):
        self.config = config

    def _parse_date(self, date_str: str) -> datetime:
        return datetime.strptime(date_str, "%Y-%m-%d")

    def split_data(self, data: dict[str, List[DailyData]], train_days: int, test_days: int, step_days: int) -> List[Tuple[dict[str, List[DailyData]], dict[str, List[DailyData]]]]:
        if not data:
            return []
            
        # 1. Gather all global dates
        all_dates = set()
        for sym, bars in data.items():
            for bar in bars:
                all_dates.add(bar.timestamp)
                
        sorted_dates = sorted(list(all_dates))
        if len(sorted_dates) == 0:
            return []

        windows = []
        start_idx = 0
        
        while start_idx + train_days + test_days <= len(sorted_dates):
            train_dates_set = set(sorted_dates[start_idx : start_idx + train_days])
            test_dates_set = set(sorted_dates[start_idx + train_days : start_idx + train_days + test_days])
            
            train_data = {}
            test_data = {}
            
            for sym, bars in data.items():
                t_bars = [b for b in bars if b.timestamp in train_dates_set]
                o_bars = [b for b in bars if b.timestamp in test_dates_set]
                if t_bars: train_data[sym] = t_bars
                if o_bars: test_data[sym] = o_bars
                
            windows.append((train_data, test_data))
            start_idx += step_days

        return windows

    def run(self, data: dict[str, List[DailyData]], train_days: int = 504, test_days: int = 252, step_days: int = 252, buy_threshold: float = 80.0, sell_threshold: float = 45.0) -> WalkForwardResult:
        """
        Runs the strategy over sliding windows.
        train_days: typically 2 years (~504 trading days)
        test_days: typically 1 year (~252 trading days)
        step_days: typically 1 year (~252 trading days)
        """
        windows_data = self.split_data(data, train_days, test_days, step_days)
        results = []
        
        cumulative_oos_return = 1.0
        total_win_rate = 0.0
        total_drawdown = 0.0
        valid_windows = 0
        
        for train, test in windows_data:
            # Note: In a full optimization system, we would optimize parameters 
            # (buy_threshold, sell_threshold, etc.) over `train` data here.
            # For this phase, we are simply validating the robustness of the 
            # *current* parameters over the OOS `test` windows.
            
            engine = BacktestEngine(config=self.config)
            test_result = engine.run(test, buy_threshold, sell_threshold)
            
            # Find earliest/latest dates across all assets for the window record
            t_start, t_end = "9999", "0000"
            for sym, bars in train.items():
                if bars[0].timestamp < t_start: t_start = bars[0].timestamp
                if bars[-1].timestamp > t_end: t_end = bars[-1].timestamp
                
            o_start, o_end = "9999", "0000"
            for sym, bars in test.items():
                if bars[0].timestamp < o_start: o_start = bars[0].timestamp
                if bars[-1].timestamp > o_end: o_end = bars[-1].timestamp
            
            w_res = WalkForwardWindow(
                train_start=t_start,
                train_end=t_end,
                test_start=o_start,
                test_end=o_end,
                test_result=test_result
            )
            results.append(w_res)
            
            cumulative_oos_return *= (1 + test_result.total_return_percent / 100.0)
            total_win_rate += test_result.win_rate_percent
            total_drawdown += test_result.max_drawdown_percent
            valid_windows += 1

        total_return_pct = (cumulative_oos_return - 1.0) * 100.0
        avg_win_rate = total_win_rate / valid_windows if valid_windows > 0 else 0.0
        avg_dd = total_drawdown / valid_windows if valid_windows > 0 else 0.0

        return WalkForwardResult(
            total_oos_return_percent=round(total_return_pct, 2),
            average_oos_win_rate=round(avg_win_rate, 2),
            average_oos_max_drawdown=round(avg_dd, 2),
            windows=results
        )
