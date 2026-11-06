"""Performance statistics for a set of closed trades, with honest uncertainty (P1.9.26).

The methods follow standard practice for judging a trading record from few observations:

* every headline number carries a bias-corrected and accelerated (BCa) bootstrap confidence interval, because a mean or
  a profit factor from 12 trades is mostly noise and a bare number hides that;
* drawdown is reported as a distribution (what ordering luck alone could do to the same trades), not one lucky path;
* the cost drag (fees as a share of gross profit) is shown next to the net result, since in Indian cash equities the
  round trip itself is a large fixed hurdle;
* the record is compared with the benchmark over the same holding periods when benchmark returns are supplied;
* nothing is reported as reliable below a sample size that can support it.

Implemented independently from the textbook definitions (Efron & Tibshirani BCa; Martin's Ulcer index).
"""
from __future__ import annotations

import math
import os
from dataclasses import dataclass
from statistics import NormalDist
from typing import Callable, Dict, List, Optional, Sequence

import numpy as np

# NSE has roughly 245-250 trading sessions a year; 252 is the US convention.
TRADING_DAYS_INDIA = 248
# An ASSUMPTION, not a fetched value: set RISK_FREE_RATE_PCT to the current 91-day T-bill yield.
DEFAULT_RISK_FREE_PCT = 6.5

MIN_TRADES_FOR_ANY_INTERVAL = 5
MIN_TRADES_LOW = 10
MIN_TRADES_RELIABLE = 30

_N = NormalDist()


def risk_free_rate_pct() -> float:
    try:
        return float(os.environ.get("RISK_FREE_RATE_PCT", DEFAULT_RISK_FREE_PCT))
    except ValueError:
        return DEFAULT_RISK_FREE_PCT


# ----------------------------------------------------------------------------- statistics on arrays

def profit_factor(pnl: Sequence[float]) -> float:
    """Gross profit / gross loss. +inf when there are wins and no losses, nan when there are neither."""
    a = np.asarray(pnl, dtype=float)
    wins, losses = a[a > 0].sum(), -a[a < 0].sum()
    if losses == 0:
        return math.inf if wins > 0 else math.nan
    return float(wins / losses)


def max_drawdown_pct(pnl: Sequence[float], initial_capital: float) -> float:
    """Largest peak-to-trough fall of the cumulative realised equity curve, as a percent of the peak (>= 0)."""
    curve = initial_capital + np.cumsum(np.asarray(pnl, dtype=float))
    curve = np.concatenate(([initial_capital], curve))
    peaks = np.maximum.accumulate(curve)
    with np.errstate(divide="ignore", invalid="ignore"):
        dd = np.where(peaks > 0, (peaks - curve) / peaks, 0.0)
    return float(dd.max() * 100)


def ulcer_index(pnl: Sequence[float], initial_capital: float) -> float:
    """Root-mean-square of the percentage drawdowns along the curve: penalises depth AND duration."""
    curve = initial_capital + np.cumsum(np.asarray(pnl, dtype=float))
    curve = np.concatenate(([initial_capital], curve))
    peaks = np.maximum.accumulate(curve)
    with np.errstate(divide="ignore", invalid="ignore"):
        dd = np.where(peaks > 0, (peaks - curve) / peaks * 100, 0.0)
    return float(math.sqrt(float((dd ** 2).mean())))


def sharpe_ratio(returns: Sequence[float], periods_per_year: int = TRADING_DAYS_INDIA, rf_annual_pct: Optional[float] = None) -> float:
    """Annualised Sharpe of periodic returns (fractions) against a per-period risk-free rate."""
    r = np.asarray(returns, dtype=float)
    if len(r) < 2:
        return math.nan
    rf = (risk_free_rate_pct() if rf_annual_pct is None else rf_annual_pct) / 100.0
    excess = r - rf / periods_per_year
    sd = excess.std(ddof=1)
    return float(excess.mean() / sd * math.sqrt(periods_per_year)) if sd > 0 else math.nan


# ----------------------------------------------------------------------------- bootstrap

def bca_interval(x: Sequence[float], stat: Callable[[np.ndarray], float], n_boot: int = 2000, confidence: float = 0.95,
                 seed: int = 42) -> Optional[Dict[str, float]]:
    """BCa bootstrap confidence interval of `stat(x)`; None when the data cannot support one.

    Bias correction z0 comes from the share of resamples below the estimate; the acceleration comes from the jackknife.
    Non-finite resampled statistics (e.g. a profit factor with no losses) are dropped, and if too few remain no
    interval is claimed.
    """
    arr = np.asarray(x, dtype=float)
    n = len(arr)
    if n < MIN_TRADES_FOR_ANY_INTERVAL:
        return None
    theta = stat(arr)
    if not np.isfinite(theta):
        return None
    rng = np.random.default_rng(seed)
    idx = rng.integers(0, n, size=(n_boot, n))
    boots = np.array([stat(arr[i]) for i in idx], dtype=float)
    boots = boots[np.isfinite(boots)]
    if len(boots) < max(200, n_boot // 4):
        return None
    prop = float((boots < theta).mean())
    prop = min(max(prop, 1.0 / (len(boots) + 1)), 1.0 - 1.0 / (len(boots) + 1))
    z0 = _N.inv_cdf(prop)
    jack = np.array([stat(np.delete(arr, i)) for i in range(n)], dtype=float)
    jack = jack[np.isfinite(jack)]
    a = 0.0
    if len(jack) >= 3:
        d = jack.mean() - jack
        den = 6.0 * (float((d ** 2).sum()) ** 1.5)
        a = float((d ** 3).sum()) / den if den > 0 else 0.0
    alpha = (1 - confidence) / 2

    def adj(z: float) -> float:
        zz = z0 + (z0 + z) / (1 - a * (z0 + z))
        return min(max(_N.cdf(zz), 0.0), 1.0)

    lo, hi = np.quantile(boots, [adj(_N.inv_cdf(alpha)), adj(_N.inv_cdf(1 - alpha))])
    return {"estimate": float(theta), "low": float(lo), "high": float(hi), "confidence": confidence, "n_boot": int(len(boots))}


def drawdown_distribution(pnl: Sequence[float], initial_capital: float, n_boot: int = 2000, seed: int = 42) -> Optional[Dict[str, float]]:
    """Max drawdown of the SAME trades in random order: how bad ordering luck alone can make it (percent)."""
    a = np.asarray(pnl, dtype=float)
    if len(a) < MIN_TRADES_FOR_ANY_INTERVAL:
        return None
    rng = np.random.default_rng(seed)
    dds = np.array([max_drawdown_pct(rng.permutation(a), initial_capital) for _ in range(n_boot)])
    return {"actual": max_drawdown_pct(a, initial_capital), "median": float(np.median(dds)),
            "p95": float(np.quantile(dds, 0.95)), "p99": float(np.quantile(dds, 0.99))}


# ----------------------------------------------------------------------------- the report

@dataclass
class ClosedTrade:
    pnl: float                       # net of all charges, INR
    invested: float                  # cost of the shares sold, INR (> 0)
    fees: float = 0.0                # all charges on the round trip that belong to this trade, INR
    holding_days: Optional[float] = None
    benchmark_return_pct: Optional[float] = None  # index return over the same holding period, if known


def _reliability(n: int) -> str:
    if n < MIN_TRADES_LOW:
        return "INSUFFICIENT"
    return "LOW" if n < MIN_TRADES_RELIABLE else "ADEQUATE"


def performance_report(trades: List[ClosedTrade], initial_capital: float, n_boot: int = 2000, seed: int = 42) -> Dict:
    n = len(trades)
    out: Dict = {
        "n_trades": n, "reliability": _reliability(n),
        "assumptions": {"trading_days_per_year": TRADING_DAYS_INDIA, "risk_free_rate_pct": risk_free_rate_pct(),
                        "risk_free_note": "An assumption; set RISK_FREE_RATE_PCT to the current 91-day T-bill yield.",
                        "bootstrap": {"method": "BCa", "n_boot": n_boot, "confidence": 0.95}},
    }
    if n == 0:
        out["message"] = "No closed trades yet: sell something you hold to get a performance record."
        return out
    pnl = np.array([t.pnl for t in trades], dtype=float)
    inv = np.array([max(t.invested, 1e-9) for t in trades], dtype=float)
    ret = pnl / inv * 100  # percent per trade, net
    wins, losses = pnl[pnl > 0], pnl[pnl < 0]
    gross_profit = float(wins.sum())
    fees = float(sum(t.fees for t in trades))
    out.update({
        "total_net_pnl": float(pnl.sum()), "total_fees": fees,
        "win_rate_pct": float((pnl > 0).mean() * 100),
        "avg_win_pct": float(ret[pnl > 0].mean()) if len(wins) else None,
        "avg_loss_pct": float(ret[pnl < 0].mean()) if len(losses) else None,
        "payoff_ratio": float(wins.mean() / -losses.mean()) if len(wins) and len(losses) else None,
        "profit_factor": None if math.isnan(profit_factor(pnl)) else (None if math.isinf(profit_factor(pnl)) else float(profit_factor(pnl))),
        "profit_factor_note": "No losing trades yet, so the profit factor is undefined." if len(losses) == 0 and len(wins) else None,
        "cost_drag_pct_of_gross_profit": float(fees / gross_profit * 100) if gross_profit > 0 else None,
        "avg_holding_days": float(np.mean([t.holding_days for t in trades if t.holding_days is not None]))
        if any(t.holding_days is not None for t in trades) else None,
        "expectancy_pct": bca_interval(ret, np.mean, n_boot, seed=seed),
        "profit_factor_ci": bca_interval(pnl, profit_factor, n_boot, seed=seed),
        "max_drawdown": drawdown_distribution(pnl, initial_capital, n_boot, seed),
        "ulcer_index": ulcer_index(pnl, initial_capital),
    })
    bench = [t.benchmark_return_pct for t in trades]
    if all(b is not None for b in bench):
        excess = ret - np.array(bench, dtype=float)
        out["excess_vs_benchmark_pct"] = bca_interval(excess, np.mean, n_boot, seed=seed)
        out["benchmark_beaten_pct_of_trades"] = float((excess > 0).mean() * 100)
    else:
        out["excess_vs_benchmark_pct"] = None
    out["verdict"] = _verdict(out)
    return out


def _verdict(r: Dict) -> str:
    n = r["n_trades"]
    if r["reliability"] == "INSUFFICIENT":
        return (f"Only {n} closed trades: too few to say anything reliable about skill. Treat every number here as an anecdote "
                f"until you have at least {MIN_TRADES_LOW} (and ideally {MIN_TRADES_RELIABLE}).")
    e = r.get("expectancy_pct")
    if e is None:
        return "The data cannot support a confidence interval yet."
    if e["low"] > 0:
        return "The average net return per trade is positive with 95% confidence on this record."
    if e["high"] < 0:
        return "The average net return per trade is negative with 95% confidence on this record."
    return ("The average net return per trade is not distinguishable from zero: the interval includes both gains and losses, "
            "so this record does not yet show skill or its absence.")
