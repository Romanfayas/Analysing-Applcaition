# ARCHITECTURE AUDIT

## 1. System Topology

The platform follows a decoupled, microservices-style architecture:

1. **Frontend (Next.js 16)**
   - App Router based.
   - Tailwind CSS v4 styling.
   - Component-driven (e.g., `StatCard`, `PredictionBarChart`).

2. **Backend API (Go)**
   - REST API using `chi` router.
   - Orchestrates data flow between the DB, Frontend, and Quant Engine.
   - Handles Authentication (JWT), Caching (Redis), and Scheduling (Workers).

3. **Quantitative Engine (Python/FastAPI)**
   - Stateless microservice strictly for mathematical computation.
   - Exposes endpoints (`/indicators`, `/screen`, `/calculate`).
   - Purely deterministic; no database connections.

4. **Database (PostgreSQL + TimescaleDB)**
   - Stores user data, configurations (Shariah rules), and all historical market data.
   - TimescaleDB used for hypertable chunking of OHLCV time-series data.

## 2. Component Evaluation

| Component | Status | Verification |
|-----------|--------|--------------|
| **Go API** | Partially Implemented | `backend/cmd/api/main.go` exists. Handlers are wired, but logic is often mocked. |
| **Go Workers** | Implemented | `backend/cmd/worker/main.go` and `ingestion/worker.go` exist. |
| **Python Engine** | Implemented | `quant/src/api/app.py` runs FastAPI. Engines are highly modularized. |
| **TimescaleDB** | Implemented | `backend/migrations/init.sql` explicitly creates hypertables for `ohlcv_daily`. |
| **Redis Cache** | Implemented | `backend/internal/cache/redis.go` exists. |
| **Prometheus/Grafana** | Implemented | Docker Compose setup includes metrics endpoints. |

## 3. Structural Strengths
- **Decoupling**: The Python engine being stateless is an excellent architectural choice. It allows heavy pandas/numpy calculations to scale independently of the Go API.
- **Data Modeling**: The database schema is extremely robust, supporting deep fundamental data, candlestick contexts, and versioned Shariah rules.

## 4. Structural Weaknesses
- **API Boundaries**: The contract between the Frontend and Go API is broken; the frontend uses hardcoded state.
- **Engine Context**: The Go backend does not currently construct the complex JSON payloads required by the Python engine (e.g., Backtest handler mocks the payload).
