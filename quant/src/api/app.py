"""
Halal Equity & IPO Research Platform — Quantitative Engine API

FastAPI application serving the Python quantitative analysis engine.
All numerical calculations are performed deterministically here.
LLMs are NEVER used as the source of numerical truth.
"""

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

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
    return {"status": "healthy", "service": "halal-equity-quant"}
