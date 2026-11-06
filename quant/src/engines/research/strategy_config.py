"""
Strategy V2 frozen-configuration verification (P1.7).

The hash is computed from values INTROSPECTED from the code that actually executes (pydantic defaults of the
backtest API, BacktestConfig, WalkForward signature defaults, a behavioural probe of SignalEngine's STRONG_BUY
boundary, and any BUY_THRESHOLD / SELL_THRESHOLD / ATR_MULTIPLIER environment overrides), never from a literal dict
that only the hash function itself consumes.

Canonical serialization (the one that reproduces the pinned hash):
    sha256( json.dumps({"ATR_MULTIPLIER":..,"BUY_THRESHOLD":..,"SELL_THRESHOLD":..}, sort_keys=True).encode("utf-8") )
with Python's default separators (", " and ": ") and floats rendered with a trailing ".0" (80.0, 45.0, 2.0).
Equivalent bytes: {"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}
"""
import hashlib
import inspect
import json
import os
from typing import Any, Dict, List

PINNED_HASH = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
STRATEGY_V2_CONFIG_ID = "STRATEGY_V2_CORE_PARAMETERS_V1"
FROZEN = {"BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 2.0}

# P1.9.16 Forensic Specification: Composite Breakout Screener and Portfolio Risk Engine
# In P1.9.15, this 12-parameter object was erroneously cited as having hash 92ab27c2...
# We establish its distinct versioned identity and independently reproducible hashes here.
COMPOSITE_BREAKOUT_RISK_ID = "BREAKOUT_PORTFOLIO_RISK_COMPOSITE_V1"
COMPOSITE_BREAKOUT_RISK_FROZEN = {
    "atr_stop_multiplier": 2.0,
    "cost_schedule": "INDIA_EQ_DELIVERY_FEES_2026Q4_V2",
    "donchian_breakout_period": 20,
    "max_open_positions": 5,
    "max_portfolio_heat": 0.10,
    "max_single_trade_risk": 0.02,
    "min_volume_multiple": 1.5,
    "partial_profit_atr": 3.0,
    "partial_profit_size": 0.50,
    "slippage_model": "INSTITUTIONAL_LIQUIDITY_V2",
    "trailing_stop_activation_atr": 1.5,
    "volume_filter_period": 20,
}
COMPOSITE_COMPACT_HASH = "0daac86130da4bf6e0fba7b2a9e8f7ff76c309dee4702b4a0a23c33e0a42f79f"
COMPOSITE_STANDARD_HASH = "deb24826865c3eefb2a1c7126ccf64f1955904af8631859042de93e980e6b5ba"


class StrategyDrift(RuntimeError):
    pass


def canonical_json(cfg: Dict[str, float]) -> str:
    return json.dumps({k: float(v) for k, v in cfg.items()}, sort_keys=True)


def compute_hash(cfg: Dict[str, float]) -> str:
    return hashlib.sha256(canonical_json(cfg).encode("utf-8")).hexdigest()


def collect_runtime_sources() -> Dict[str, Dict[str, float]]:
    """parameter -> {source_name: value} as read from the live code paths."""
    from src.api.routes.backtesting import BacktestRequest
    from src.engines.backtest.engine import BacktestConfig
    from src.engines.backtest.walk_forward import WalkForwardEngine
    from src.engines.signal.engine import SignalEngine, Signal

    src: Dict[str, Dict[str, float]] = {"BUY_THRESHOLD": {}, "SELL_THRESHOLD": {}, "ATR_MULTIPLIER": {}}
    mf = BacktestRequest.model_fields
    src["BUY_THRESHOLD"]["BacktestRequest.default"] = float(mf["buy_threshold"].default)
    src["SELL_THRESHOLD"]["BacktestRequest.default"] = float(mf["sell_threshold"].default)
    src["ATR_MULTIPLIER"]["BacktestRequest.default"] = float(mf["position_size_atr_multiplier"].default)
    src["ATR_MULTIPLIER"]["BacktestConfig.default"] = float(BacktestConfig().position_size_atr_multiplier)
    sig = inspect.signature(WalkForwardEngine.run).parameters
    src["BUY_THRESHOLD"]["WalkForwardEngine.run.default"] = float(sig["buy_threshold"].default)
    src["SELL_THRESHOLD"]["WalkForwardEngine.run.default"] = float(sig["sell_threshold"].default)
    # behavioural probe of the signal engine's STRONG_BUY boundary (find smallest 0.5-step score that maps to STRONG_BUY)
    eng = SignalEngine.__new__(SignalEngine)
    probe = next((s / 2 for s in range(0, 201) if eng._score_to_signal(s / 2) == Signal.STRONG_BUY), None)
    if probe is not None:
        src["BUY_THRESHOLD"]["SignalEngine.STRONG_BUY_boundary"] = float(probe)
    for k in list(src):
        if os.getenv(k) not in (None, ""):
            src[k]["env"] = float(os.environ[k])
    return src


def runtime_config() -> Dict[str, Any]:
    sources = collect_runtime_sources()
    resolved: Dict[str, float] = {}
    problems: List[str] = []
    for k, vals in sources.items():
        distinct = set(vals.values())
        if len(distinct) != 1:
            problems.append(f"{k} disagrees across sources: {vals}")
        else:
            resolved[k] = distinct.pop()
    for k, v in resolved.items():
        if v != FROZEN[k]:
            problems.append(f"{k}={v} differs from frozen {FROZEN[k]}")
    h = compute_hash(resolved) if len(resolved) == 3 else None
    if h != PINNED_HASH:
        problems.append(f"runtime hash {h} != pinned {PINNED_HASH}")
    return {"resolved": resolved, "sources": sources, "canonical_json": canonical_json(resolved) if resolved else None,
            "runtime_hash": h, "pinned_hash": PINNED_HASH, "matches_pinned": h == PINNED_HASH, "problems": problems}


def assert_frozen() -> Dict[str, Any]:
    cfg = runtime_config()
    if cfg["problems"]:
        raise StrategyDrift("; ".join(cfg["problems"]))
    return cfg


def experimental_allowed() -> bool:
    return os.getenv("ALLOW_EXPERIMENTAL_PARAMS", "false").lower() == "true"


def enforce_request(buy_threshold: float, sell_threshold: float, atr_multiplier: float) -> None:
    """Raise StrategyDrift if a caller tries to run with non-frozen parameters (unless experimentation is explicitly enabled)."""
    got = {"BUY_THRESHOLD": buy_threshold, "SELL_THRESHOLD": sell_threshold, "ATR_MULTIPLIER": atr_multiplier}
    if got != FROZEN and not experimental_allowed():
        raise StrategyDrift(f"request parameters {got} differ from frozen Strategy V2 {FROZEN}; set ALLOW_EXPERIMENTAL_PARAMS=true to run labelled experiments")
