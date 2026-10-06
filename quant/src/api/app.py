"""
Halal Equity & IPO Research Platform — Quantitative Engine API

FastAPI application serving the Python quantitative analysis engine.
All numerical calculations are performed deterministically here.
LLMs are NEVER used as the source of numerical truth.
"""

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
import hashlib
import json
import sys

from src.api.routes import technical, fundamental, candlestick, quantitative, research
from src.api.routes import shariah, signal, risk, ipo, backtesting

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
app.include_router(fundamental.router, prefix="/api/v1/fundamental", tags=["Fundamental Analysis"])
app.include_router(candlestick.router, prefix="/api/v1/candlestick", tags=["Candlestick Patterns"])
app.include_router(quantitative.router, prefix="/api/quantitative", tags=["Quantitative"])
app.include_router(research.router, prefix="/api/research", tags=["Research Analytics"])
app.include_router(shariah.router, prefix="/api/v1/shariah", tags=["Shariah Screening"])
app.include_router(signal.router, prefix="/api/v1/signal", tags=["Signal Engine"])
app.include_router(risk.router, prefix="/api/v1/risk", tags=["Risk Engine"])
app.include_router(ipo.router, prefix="/api/v1/ipo", tags=["IPO Analysis"])
app.include_router(backtesting.router, prefix="/api/v1/backtesting", tags=["Backtesting"])


@app.get("/health")
async def health():
    return {"status": "healthy", "service": "halal-equity-quant", "strategy_hash": STRATEGY_V2_HASH}

STRATEGY_V2_CONFIG = {
    "BUY_THRESHOLD": 80.0,
    "SELL_THRESHOLD": 45.0,
    "ATR_MULTIPLIER": 2.0
}

STRATEGY_V2_HASH = hashlib.sha256(json.dumps(STRATEGY_V2_CONFIG, sort_keys=True).encode()).hexdigest()

@app.on_event("startup")
async def verify_strategy_v2():
    # Enforce Strategy V2 configuration frozen status
    if STRATEGY_V2_CONFIG["BUY_THRESHOLD"] != 80.0:
        print("FATAL: Strategy V2 BUY_THRESHOLD must be 80.0")
        sys.exit(1)
    if STRATEGY_V2_CONFIG["SELL_THRESHOLD"] != 45.0:
        print("FATAL: Strategy V2 SELL_THRESHOLD must be 45.0")
        sys.exit(1)
    if STRATEGY_V2_CONFIG["ATR_MULTIPLIER"] != 2.0:
        print("FATAL: Strategy V2 ATR_MULTIPLIER must be 2.0")
        sys.exit(1)
    print(f"[QUANT ENGINE] Strategy V2 Verified. Hash: {STRATEGY_V2_HASH}")
