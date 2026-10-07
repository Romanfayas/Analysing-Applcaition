# Phase 15: Monitoring & Governance

## Status: IMPLEMENTED

### Monitoring Stack
- **Prometheus**: Time-series database for scraping application metrics.
- **Grafana**: Dashboard visualization for metrics.

### Key Monitored Areas
- Market Data Freshness
- Provider Health and API Health
- Database and Redis latency
- Signal Generation Rates
- Paper Orders, Fills, P&L, Slippage, and Transaction Costs
- Fallbacks and Exceptions

### Alerts
Alerts are routed via `TelegramNotifier` and application logs for:
- `STALE_MARKET_DATA`
- `MARKET_DATA_DISCONNECTED`
- `DATABASE_FAILURE`
- `STRATEGY_HASH_MISMATCH`
- `UNSAFE_LIVE_CONFIG`

### Configuration Drift
At startup, the Python Quant Engine explicitly verifies the Strategy V2 parameters (80 / 45 / 2.0) and generates a `STRATEGY_V2_HASH`. If this hash is modified or the parameters drift, the application raises a fatal error and fails to start.
