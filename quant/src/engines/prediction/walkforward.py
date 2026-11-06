"""Walk-forward validation: for every test day, train only on strictly earlier days.

Reports probabilistic skill and net-of-cost trading results against baselines, with day-block
bootstrap confidence intervals, and decides whether a horizon has a demonstrated edge.
"""
from dataclasses import dataclass
from typing import Any, Dict, List

import numpy as np
import pandas as pd

from . import model as M
from .config import (
    EDGE_BOOTSTRAP_RESAMPLES,
    EDGE_CONFIDENCE,
    EDGE_MIN_TEST_DAYS,
    EDGE_MIN_TRADES,
    FEATURES,
    GATE_VERSION,
    MIN_TEST_DAYS,
    MIN_TRAIN_DAYS,
)

EPS = 1e-12
REGIME_DAY_MOVE = 0.0025  # |universe open-to-close| above 0.25% counts as an up/down market day


def _logloss_rows(p: np.ndarray, y: np.ndarray) -> np.ndarray:
    return -np.log(np.clip(p[np.arange(len(y)), y], EPS, 1.0))


def _brier_rows(p: np.ndarray, y: np.ndarray) -> np.ndarray:
    onehot = np.eye(p.shape[1])[y]
    return ((p - onehot) ** 2).sum(axis=1)


def _block_bootstrap_ci(values_by_day: List[np.ndarray], stat, seed: int = 0) -> Dict[str, float]:
    """CI of a statistic over rows, resampling whole days (rows within a day are correlated)."""
    days = [v for v in values_by_day if len(v)]
    if not days:
        return {"estimate": float("nan"), "low": float("nan"), "high": float("nan")}
    rng = np.random.default_rng(seed)
    est = float(stat(np.concatenate(days)))
    boots = []
    for _ in range(EDGE_BOOTSTRAP_RESAMPLES):
        pick = rng.integers(0, len(days), len(days))
        boots.append(stat(np.concatenate([days[i] for i in pick])))
    a = (1 - EDGE_CONFIDENCE) / 2
    return {"estimate": est, "low": float(np.quantile(boots, a)), "high": float(np.quantile(boots, 1 - a))}


@dataclass
class WalkForwardResult:
    oof: pd.DataFrame          # out-of-sample predictions with outcomes
    metrics: Dict[str, Any]
    verdict: str               # ACTIONABLE | NO_DEMONSTRATED_EDGE | DATA_INSUFFICIENT


def entry_rule(p: np.ndarray, class_means: np.ndarray, cost: float) -> np.ndarray:
    """Long-only entry: expected gross move beats the round-trip cost and UP is likelier than DOWN."""
    exp_ret = p @ class_means
    return (exp_ret > cost) & (p[:, 2] > p[:, 0])


def run(ds: pd.DataFrame, cost: float) -> WalkForwardResult:
    data = ds[ds["label"] >= 0].sort_values(["date", "bar_start", "symbol_id"])
    days = sorted(data["date"].unique())
    n_test_days = len(days) - MIN_TRAIN_DAYS
    base = {"days_available": len(days), "min_train_days": MIN_TRAIN_DAYS, "min_test_days": MIN_TEST_DAYS,
            "round_trip_cost": cost, "labelled_rows": int(len(data))}
    if n_test_days < MIN_TEST_DAYS:
        base["reason"] = f"need {MIN_TRAIN_DAYS + MIN_TEST_DAYS} trading days of intraday data, have {len(days)}"
        return WalkForwardResult(pd.DataFrame(), base, "DATA_INSUFFICIENT")

    parts = []
    for d in days[MIN_TRAIN_DAYS:]:
        tr, te = data[data["date"] < d], data[data["date"] == d]
        ytr = tr["label"].to_numpy(int)
        mdl = M.fit(tr[FEATURES].to_numpy(float), ytr)
        p = M.predict_full(mdl, te[FEATURES].to_numpy(float))
        clim = M.class_frequencies(ytr)
        means = M.class_mean_returns(ytr, tr["fwd"].to_numpy(float))
        part = te[["date", "bar_start", "bar_end", "symbol_id", "symbol", "close", "fwd", "label", "ret_3"]].copy()
        part[["p_down", "p_neutral", "p_up"]] = p
        part[["c_down", "c_neutral", "c_up"]] = np.tile(clim, (len(te), 1))
        part["exp_ret"] = p @ means
        part["signal"] = entry_rule(p, means, cost)
        parts.append(part)
    oof = pd.concat(parts, ignore_index=True)

    y = oof["label"].to_numpy(int)
    P = oof[["p_down", "p_neutral", "p_up"]].to_numpy()
    C = oof[["c_down", "c_neutral", "c_up"]].to_numpy()
    oof["ll_model"] = _logloss_rows(P, y)
    oof["ll_clim"] = _logloss_rows(C, y)
    simple = np.expm1(oof["fwd"].to_numpy())
    oof["net"] = simple - cost

    by_day = lambda col, mask=None: [g[col].to_numpy() if mask is None else g.loc[mask(g), col].to_numpy()
                                     for _, g in oof.groupby("date")]
    oof["ll_gain"] = oof["ll_clim"] - oof["ll_model"]
    skill = _block_bootstrap_ci(by_day("ll_gain"), np.mean)
    model_trades = _block_bootstrap_ci(by_day("net", lambda g: g["signal"]), np.mean)
    momentum = _block_bootstrap_ci(by_day("net", lambda g: g["ret_3"] > 0), np.mean)
    always_long = _block_bootstrap_ci(by_day("net"), np.mean)

    n_trades = int(oof["signal"].sum())
    trades = oof[oof["signal"]]
    # calibration of P(UP): predicted vs realised frequency by decile
    cal = []
    if len(oof) >= 100:
        q = pd.qcut(oof["p_up"], 10, duplicates="drop")
        for _, g in oof.groupby(q, observed=True):
            cal.append({"p_up_mean": float(g["p_up"].mean()), "up_rate": float((g["label"] == 2).mean()), "n": int(len(g))})
    ece = float(sum(abs(c["p_up_mean"] - c["up_rate"]) * c["n"] for c in cal) / max(1, len(oof))) if cal else None

    metrics = base | {
        "test_days": [str(d) for d in days[MIN_TRAIN_DAYS:]],
        "oos_rows": int(len(oof)),
        "class_share_oos": M.class_frequencies(y).round(4).tolist(),
        "log_loss": {"model": float(oof["ll_model"].mean()), "climatology": float(oof["ll_clim"].mean())},
        "log_loss_gain_vs_climatology": skill,
        "brier": {"model": float(_brier_rows(P, y).mean()), "climatology": float(_brier_rows(C, y).mean())},
        "accuracy": {"model": float((P.argmax(1) == y).mean()), "climatology": float((C.argmax(1) == y).mean())},
        "calibration_p_up": cal,
        "expected_calibration_error_p_up": ece,
        "trading": {
            "model_signals": n_trades,
            "model_net_return_per_trade": model_trades,
            "model_hit_rate_net": float((trades["net"] > 0).mean()) if n_trades else None,
            "baseline_momentum_net_return_per_trade": momentum,
            "baseline_always_long_net_return_per_trade": always_long,
        },
    }

    # Stability: how results vary by test day and by the market's direction that day. Regimes are labelled
    # after the fact (for reporting only); nothing here feeds the model.
    # The day's market move = universe mean of each symbol's open-to-last-bar return. It is the same for every
    # horizon (a horizon-specific average would label nearly every 5-minute day "flat").
    last_bar = ds.sort_values("bar_start").groupby(["date", "symbol_id"]).tail(1)
    day_market = last_bar.groupby("date")["ret_from_open"].mean()
    regime_of = {d: ("market_up_day" if r > REGIME_DAY_MOVE else "market_down_day" if r < -REGIME_DAY_MOVE else "market_flat_day")
                 for d, r in day_market.items()}
    oof["regime"] = oof["date"].map(regime_of)
    per_day = []
    for d, g in oof.groupby("date"):
        tr = g[g["signal"]]
        per_day.append({"date": str(d), "regime": regime_of[d], "rows": int(len(g)), "signals": int(len(tr)),
                        "mean_net_return": float(tr["net"].mean()) if len(tr) else None,
                        "log_loss_gain": float(g["ll_gain"].mean())})
    regimes = {}
    for reg, g in oof.groupby("regime"):
        tr = g[g["signal"]]
        regimes[reg] = {"days": int(g["date"].nunique()), "signals": int(len(tr)),
                        "mean_net_return": float(tr["net"].mean()) if len(tr) else None,
                        "log_loss_gain": float(g["ll_gain"].mean())}
    metrics["per_day"] = per_day
    metrics["regimes"] = regimes
    metrics["gate_version"] = GATE_VERSION
    metrics["positive_net_days"] = sum(1 for p in per_day if p["mean_net_return"] is not None and p["mean_net_return"] > 0)

    checks = {
        "enough_independent_test_days": n_test_days >= EDGE_MIN_TEST_DAYS,
        "enough_trades": n_trades >= EDGE_MIN_TRADES,
        "probabilistic_skill_ci_above_zero": bool(skill["low"] > 0),
        "net_return_ci_above_zero": bool(model_trades["low"] > 0) if n_trades else False,
    }
    metrics["edge_checks"] = checks
    verdict = "ACTIONABLE" if all(checks.values()) else "NO_DEMONSTRATED_EDGE"
    return WalkForwardResult(oof, metrics, verdict)
