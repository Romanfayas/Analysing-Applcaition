# Final System Architecture

## Overview
The Halal Equity & IPO Research Platform is a quantitative analysis and paper-trading platform designed specifically for Indian equities, emphasizing Shariah-compliant screening and deterministic quantitative trading.

## Services
1. **Frontend (Next.js)**: Displays portfolio, market data (with freshness indicators), and backtest results.
2. **Backend API (Go)**: Serves REST endpoints for frontend data access. Controls user sessions and portfolio tracking.
3. **Ingestion Worker (Go)**: Periodically fetches market data (every 1 minute during active market hours).
4. **Quant Engine (Python/FastAPI)**: Performs all numerical calculations, backtesting (Strategy V2), and signal generation.
5. **PostgreSQL / TimescaleDB**: Primary data store for OHLCV ticks, signals, and paper trades.
6. **Redis**: In-memory cache for rate-limiting, session states, and queue management.
7. **Prometheus & Grafana**: System observability and health checks.

## Safety Architecture
- **Market Data**: Explicitly tracked with `received_at` and `market_timestamp` with `STALE` flagging to prevent stale signals.
- **Strategy V2**: Locked at `BUY=80 / SELL=45 / ATR=2.0`. A SHA256 hash is generated and verified at Python Quant Engine startup. Modifying these stops the container.
- **Execution**: Live trading is blocked at the environment level (`LIVE_ORDER_EXECUTION=false`). All orders route to a Paper Order Manager which evaluates idempotency keys.
- **Kill Switch**: `TRADING_ENABLED=false` bypass guarantees all new paper and live orders fail gracefully.
