import numpy as np
import pandas as pd
from typing import List, Dict, Optional
from pydantic import BaseModel

class AdvancedMetrics(BaseModel):
    cagr_percent: float
    volatility_percent: float
    sharpe_ratio: float
    sortino_ratio: float
    calmar_ratio: float
    var_95_percent: float
    cvar_95_percent: float
    alpha_percent: Optional[float] = None
    beta: Optional[float] = None

def calculate_portfolio_metrics(
    equity_curve: List[float], 
    dates: List[str],
    benchmark_returns: Optional[List[float]] = None
) -> AdvancedMetrics:
    if len(equity_curve) < 2:
        return AdvancedMetrics(
            cagr_percent=0.0, volatility_percent=0.0, sharpe_ratio=0.0, 
            sortino_ratio=0.0, calmar_ratio=0.0, var_95_percent=0.0, cvar_95_percent=0.0
        )
        
    df = pd.DataFrame({'equity': equity_curve, 'date': pd.to_datetime(dates)})
    df.set_index('date', inplace=True)
    
    # Calculate daily returns
    df['returns'] = df['equity'].pct_change().fillna(0)
    
    returns = df['returns'].values
    
    # Volatility (Annualized)
    volatility = np.std(returns) * np.sqrt(252)
    
    # CAGR
    total_return = (equity_curve[-1] / equity_curve[0]) - 1
    years = len(equity_curve) / 252.0
    cagr = ((equity_curve[-1] / equity_curve[0]) ** (1 / years) - 1) if years > 0 else total_return
    
    # Sharpe (assuming 0% risk free rate for simplicity)
    sharpe = (np.mean(returns) * 252) / volatility if volatility > 0 else 0.0
    
    # Sortino
    downside_returns = returns[returns < 0]
    downside_vol = np.std(downside_returns) * np.sqrt(252) if len(downside_returns) > 0 else 0.0
    sortino = (np.mean(returns) * 252) / downside_vol if downside_vol > 0 else 0.0
    
    # Drawdown & Calmar
    rolling_max = df['equity'].cummax()
    drawdowns = (rolling_max - df['equity']) / rolling_max
    max_dd = drawdowns.max()
    calmar = cagr / max_dd if max_dd > 0 else 0.0
    
    # VaR & CVaR (Historical 95%)
    if len(returns) > 30:
        var_95 = np.percentile(returns, 5)
        cvar_95 = np.mean(returns[returns <= var_95])
    else:
        var_95, cvar_95 = 0.0, 0.0
        
    # Alpha & Beta
    alpha, beta = None, None
    if benchmark_returns is not None and len(benchmark_returns) == len(returns):
        cov = np.cov(returns, benchmark_returns)[0][1]
        var_bench = np.var(benchmark_returns)
        if var_bench > 0:
            beta = cov / var_bench
            ann_bench_ret = np.mean(benchmark_returns) * 252
            ann_port_ret = np.mean(returns) * 252
            alpha = ann_port_ret - (beta * ann_bench_ret)
            
    return AdvancedMetrics(
        cagr_percent=round(cagr * 100, 2),
        volatility_percent=round(volatility * 100, 2),
        sharpe_ratio=round(sharpe, 2),
        sortino_ratio=round(sortino, 2),
        calmar_ratio=round(calmar, 2),
        var_95_percent=round(abs(var_95) * 100, 2),
        cvar_95_percent=round(abs(cvar_95) * 100, 2),
        alpha_percent=round(alpha * 100, 2) if alpha is not None else None,
        beta=round(beta, 2) if beta is not None else None
    )
