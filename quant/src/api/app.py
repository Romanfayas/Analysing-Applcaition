"""
Halal Equity & IPO Research Platform — Quantitative Engine API

FastAPI application serving the Python quantitative analysis engine.
All numerical calculations are performed deterministically here.
LLMs are NEVER used as the source of numerical truth.
"""

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
import hashlib
import os
import json
import sys

from src.api.routes import technical, fundamental, candlestick, quantitative, research
from src.api.routes import shariah, signal, risk, ipo, backtesting, breakout

app = FastAPI(
    title="Halal Equity Quant Engine",
    description="Deterministic quantitative analysis engine for Indian equity and IPO research",
    version="0.1.0",
)

# CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:3000", "http://localhost:8080"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Register route modules
app.include_router(technical.router, prefix="/api/v1/technical", tags=["Technical Analysis"])
app.include_router(breakout.router, prefix="/api/v1/breakout", tags=["Breakout Research"])
app.include_router(fundamental.router, prefix="/api/v1/fundamental", tags=["Fundamental Analysis"])
app.include_router(candlestick.router, prefix="/api/v1/candlestick", tags=["Candlestick Patterns"])
app.include_router(quantitative.router, prefix="/api/quantitative", tags=["Quantitative"])
app.include_router(research.router, prefix="/api/research", tags=["Research Analytics"])
app.include_router(research.router, prefix="/api/v1/research", tags=["Research Analytics"])

app.include_router(shariah.router, prefix="/api/v1/shariah", tags=["Shariah Screening"])
app.include_router(signal.router, prefix="/api/v1/signal", tags=["Signal Engine"])
app.include_router(risk.router, prefix="/api/v1/risk", tags=["Risk Engine"])
app.include_router(ipo.router, prefix="/api/v1/ipo", tags=["IPO Analysis"])
app.include_router(backtesting.router, prefix="/api/v1/backtesting", tags=["Backtesting"])


@app.get("/health")
async def health():
    from src.engines.research.strategy_config import runtime_config
    cfg = runtime_config()
    return {"status": "healthy" if not cfg["problems"] else "strategy_drift", "service": "halal-equity-quant",
            "strategy_hash": cfg["runtime_hash"], "pinned_hash": cfg["pinned_hash"], "strategy_matches_pinned": cfg["matches_pinned"]}


@app.get("/api/v1/governance/strategy")
async def strategy_governance():
    """Runtime-introspected Strategy V2 configuration, per-source values, canonical serialization and hash."""
    from src.engines.research.strategy_config import runtime_config, experimental_allowed
    return {**runtime_config(), "experimental_params_allowed": experimental_allowed()}


@app.on_event("startup")
async def verify_strategy_v2():
    # Fail closed: the hash is computed from values read out of the code that actually runs (see strategy_config).
    from src.engines.research.strategy_config import assert_frozen, StrategyDrift
    try:
        cfg = assert_frozen()
    except StrategyDrift as e:
        print(f"FATAL: Strategy V2 drift: {e}")
        sys.exit(1)
    for flag in ("LIVE_ORDER_EXECUTION", "BROKER_TRADING_ENABLED"):
        if os.getenv(flag, "false").lower() in ("1", "true", "yes", "on"):
            print(f"FATAL: {flag} must be false: live trading is disabled")
            sys.exit(1)
    print(f"[QUANT ENGINE] Strategy V2 verified from runtime sources. Hash: {cfg['runtime_hash']}")
