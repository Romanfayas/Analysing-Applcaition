"""
Pre-registered evaluation statistics (P1.7). Implements docs/P1.7_PREREGISTERED_EVALUATION_SPEC.md.
Pure functions; no parameter is tuned anywhere in this module.
"""
import math
import random
from dataclasses import dataclass
from datetime import date, timedelta
from typing import Any, Dict, List, Optional, Sequence, Tuple

from scipy import stats as sps

BOOTSTRAP_SEED = 20261014
BOOTSTRAP_RESAMPLES = 10_000
ALPHA = 0.05
MIN_INDEPENDENT_TRADES = 100          # operational gate, NOT a significance claim
MIN_TRADES_PER_REGIME = 20
MIN_REGIMES = 2


def wilson_interval(k: int, n: int, alpha: float = ALPHA) -> Optional[Tuple[float, float]]:
    if n == 0:
        return None
    z = sps.norm.ppf(1 - alpha / 2)
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return (max(0.0, c - h), min(1.0, c + h))


def mean_ci(values: Sequence[float], alpha: float = ALPHA) -> Dict[str, Any]:
    """t-interval and seeded percentile bootstrap for the mean. None when n < 2 (no variance estimate)."""
    n = len(values)
    if n == 0:
        return {"n": 0, "mean": None, "t_ci": None, "bootstrap_ci": None, "sd": None}
    m = sum(values) / n
    if n < 2:
        return {"n": n, "mean": m, "t_ci": None, "bootstrap_ci": None, "sd": None}
    sd = math.sqrt(sum((v - m) ** 2 for v in values) / (n - 1))
    half = sps.t.ppf(1 - alpha / 2, n - 1) * sd / math.sqrt(n)
    rng = random.Random(BOOTSTRAP_SEED)
    means = sorted(sum(rng.choice(values) for _ in range(n)) / n for _ in range(BOOTSTRAP_RESAMPLES))
    lo, hi = means[int(alpha / 2 * BOOTSTRAP_RESAMPLES)], means[int((1 - alpha / 2) * BOOTSTRAP_RESAMPLES) - 1]
    return {"n": n, "mean": m, "sd": sd, "t_ci": (m - half, m + half), "bootstrap_ci": (lo, hi)}


@dataclass
class ClosedTrade:
    entry: date
    exit: date
    net_pnl_inr: float
    notional_inr: float
    # optional daily marks while open: {session_date: close/entry_fill - 1} (gross) for MTM drawdown
    marks: Optional[Dict[date, float]] = None


def equity_curve(trades: List[ClosedTrade], initial_capital: float, mtm: bool = True) -> List[Tuple[date, float]]:
    """Chronological portfolio equity. Realised: net P&L booked on the exit date. If `mtm` and marks are provided, open positions
    contribute notional * gross mark (costs are booked only at exit, so MTM slightly flatters open-trade equity)."""
    if not trades:
        return []
    start = min(t.entry for t in trades)
    end = max(t.exit for t in trades)
    out, d = [(start - timedelta(days=1), initial_capital)], start   # opening equity before the first trade
    while d <= end:
        if d.weekday() < 5 or any(t.entry == d or t.exit == d for t in trades):
            realised = sum(t.net_pnl_inr for t in trades if t.exit <= d)
            unreal = 0.0
            if mtm:
                for t in trades:
                    if t.entry <= d < t.exit and t.marks:
                        prior = [k for k in t.marks if k <= d]
                        if prior:
                            unreal += t.notional_inr * t.marks[max(prior)]
            out.append((d, initial_capital + realised + unreal))
        d += timedelta(days=1)
    return out


def max_drawdown(curve: List[Tuple[date, float]]) -> Optional[Dict[str, Any]]:
    """Peak-to-trough on chronological equity (NOT a sum of trade returns)."""
    if not curve:
        return None
    peak, peak_d, worst, w = curve[0][1], curve[0][0], 0.0, (curve[0][0], curve[0][0])
    for d, v in curve:
        if v > peak:
            peak, peak_d = v, d
        dd = (v - peak) / peak
        if dd < worst:
            worst, w = dd, (peak_d, d)
    return {"max_drawdown_pct": round(worst * 100, 4), "peak_date": w[0].isoformat(), "trough_date": w[1].isoformat()}


def reconcile_to_ledger(curve: List[Tuple[date, float]], trades: List[ClosedTrade], initial_capital: float) -> Dict[str, Any]:
    """After the last exit the equity must equal initial + sum of itemized-ledger net P&L (all trades closed)."""
    if not curve:
        return {"reconciled": True, "difference_inr": 0.0}
    diff = curve[-1][1] - (initial_capital + sum(t.net_pnl_inr for t in trades))
    return {"reconciled": abs(diff) < 0.005, "difference_inr": round(diff, 6)}


def verdict(excess_returns: Sequence[float], regime_counts: Dict[str, int], n_completed_trades: int, n_pending: int) -> Dict[str, Any]:
    """excess_returns: per INDEPENDENT trade, strategy net return minus matched equal-weight universe return."""
    n = len(excess_returns)
    ci = mean_ci(excess_returns)
    reasons: List[str] = []
    regimes_ok = sum(1 for c in regime_counts.values() if c >= MIN_TRADES_PER_REGIME)
    if n < MIN_INDEPENDENT_TRADES:
        reasons.append(f"independent trades {n} < operational gate {MIN_INDEPENDENT_TRADES}")
    if regimes_ok < MIN_REGIMES:
        reasons.append(f"only {regimes_ok} regimes have >= {MIN_TRADES_PER_REGIME} trades (need {MIN_REGIMES})")
    if reasons:
        state = "INSUFFICIENT_EVIDENCE"
    else:
        lo, hi = ci["t_ci"]
        blo, bhi = ci["bootstrap_ci"]
        if hi < 0 and bhi < 0:
            state = "FAILED_VALIDATION"
            reasons.append("upper 95% CI bound of mean excess net return is below zero")
        elif lo > 0 and blo > 0:
            state = "PASSED_PREREGISTERED_CRITERIA"
            reasons.append("lower 95% CI bound (t and bootstrap) above zero; this is not a statement of profitability, robustness or live-readiness")
        else:
            state = "INCONCLUSIVE"
            reasons.append("95% CI for mean excess net return includes zero")
    return {"state": state, "reasons": reasons, "primary_metric": "mean (strategy net return - matched equal-weight Tier-1 universe return) per independent trade",
            "ci": ci, "independent_trades": n, "completed_trades": n_completed_trades, "pending": n_pending,
            "regime_counts": regime_counts, "live_trading_approved": False}
