# Halal Equity & IPO Quantitative Research Platform

A full-stack, Shariah-conscious Indian equity and IPO quantitative research platform.

## Architecture

| Component | Technology | Port |
|-----------|-----------|------|
| Frontend | Next.js + TypeScript + Tailwind CSS | 3000 |
| Backend API | Go (chi router) | 8080 |
| Quant Engine | Python (FastAPI) | 8000 |
| Database | PostgreSQL 16 + TimescaleDB | 5432 |
| Cache | Redis 7 | 6379 |

## Important Principles

- **Shariah-conscious**: Configurable, versioned Shariah screening rules
- **No prohibited instruments**: No short selling, futures, options, CFDs, leverage, margin trading
- **Deterministic calculations**: All financial calculations in Python, never via LLM
- **No fabricated data**: Missing values are `null`, never silently replaced with zero
- **Predictions ≠ certainty**: All predictions include confidence scores and disclaimers
- **Data provenance**: Every data point records source, timestamp, and quality status

## Quick Start

### Prerequisites
- Docker & Docker Compose
- Node.js 20+ (for local frontend dev)
- Go 1.22+ (for local backend dev)
- Python 3.11+ (for local quant dev)

### Setup

```bash
# 1. Clone and configure
cp .env.example .env
# Edit .env with your values (API keys, secrets, etc.)

# 2. Start all services
docker compose up -d

# 3. Access
# Frontend: http://localhost:3000
# Backend API: http://localhost:8080/health
# Quant Engine: http://localhost:8000/health
```

### Local Development

```bash
# Frontend
cd frontend
npm install
npm run dev

# Backend
cd backend
go mod download
go run ./cmd/api

# Quant Engine
cd quant
pip install -r requirements.txt
uvicorn src.api.app:app --reload --port 8000

# Run tests
cd quant && pytest tests/ -v
cd backend && go test ./...
```

## Project Structure

```
├── docker-compose.yml       # All services
├── .env.example             # Configuration template
├── backend/                 # Go REST API + WebSocket + Workers
│   ├── cmd/api/             # API server entry point
│   ├── cmd/worker/          # Background worker entry point
│   ├── internal/            # Business logic
│   └── migrations/          # Database schema
├── quant/                   # Python quantitative engine
│   ├── src/engines/         # Technical, fundamental, Shariah, etc.
│   └── tests/               # Unit tests with known-value fixtures
├── frontend/                # Next.js dashboard
│   └── src/app/             # App Router pages
└── docs/                    # Architecture documentation
```

## API Endpoints

### Authentication
- `POST /api/v1/auth/register` — Create user account
- `POST /api/v1/auth/login` — Login and get JWT token

### Stocks
- `GET /api/v1/stocks` — List stocks
- `GET /api/v1/stocks/{symbol}` — Stock details
- `GET /api/v1/stocks/{symbol}/candles` — OHLCV data
- `GET /api/v1/stocks/{symbol}/fundamentals` — Financial analysis
- `GET /api/v1/stocks/{symbol}/technicals` — Technical indicators
- `GET /api/v1/stocks/{symbol}/shariah` — Shariah screening status
- `GET /api/v1/stocks/{symbol}/signal` — Composite signal
- `GET /api/v1/stocks/{symbol}/risk` — Risk calculations

### IPOs
- `GET /api/v1/ipos` — List IPOs
- `GET /api/v1/ipos/{id}` — IPO details
- `GET /api/v1/ipos/{id}/prediction` — Listing prediction
- `GET /api/v1/ipos/{id}/decision` — APPLY/WATCH/AVOID

### Other
- `GET /api/v1/signals` — Active signals
- `GET /api/v1/shariah/screenings` — All screenings
- `POST /api/v1/backtesting/run` — Run backtest
- `POST /api/v1/risk/calculate` — Position sizing

## Testing

```bash
# Python quant engine tests
cd quant
pytest tests/test_indicators.py -v    # Technical indicators
pytest tests/test_shariah.py -v       # Shariah screening
pytest tests/test_candlestick.py -v   # Candlestick patterns

# Go backend tests
cd backend
go test ./... -v
```

## License

Private — All rights reserved.
