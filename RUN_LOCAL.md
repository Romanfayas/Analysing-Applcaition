# HalalEquity — Local Execution & Operations Runbook

This guide provides deterministic, step-by-step instructions to run, verify, and operate the complete HalalEquity platform locally.

---

## 1. Prerequisites

Ensure the following tools are installed on your host machine:

- **Docker Desktop** (version 24.0+ with Docker Compose v2)
- **Node.js** (v20+ and `npm`)
- **Go** (v1.22+)
- **Python** (v3.11+)

---

## 2. Quickstart: Launching the Application Stack

The entire multi-tier system (PostgreSQL/TimescaleDB, Redis, Go Backend API, Python Quant Engine, Ingestion Worker, Next.js Frontend, Prometheus, Grafana) is managed via Docker Compose.

```bash
# 1. Navigate to the project root directory
cd "/Users/fayas/Downloads/ICO-Application/analysis application"

# 2. Build and start all services in detached mode
docker compose up -d --build
```

### Port Allocations

| Container Name | Service | Host Port | Internal Port | Description |
| :--- | :--- | :--- | :--- | :--- |
| `halal_equity_frontend` | Next.js Frontend | **3000** | 3000 | Primary User Interface |
| `halal_equity_backend` | Go Backend API | **8081** | 8080 | Core API, Auth, Ledger |
| `halal_equity_quant` | Python Quant Engine | **8000** | 8000 | Screener & Signals |
| `halal_equity_postgres` | TimescaleDB | **5432** | 5432 | Primary Relational Store |
| `halal_equity_redis` | Redis Cache | **6379** | 6379 | Volatile Cache & Pub/Sub |
| `halal_equity_grafana` | Grafana | **3001** | 3000 | Operational Dashboards |
| `halal_equity_prometheus`| Prometheus | **9090** | 9090 | Metrics Scraper |

---

## 2.1 Signing in (P1.9.20)

The UI requires an account. Open <http://localhost:3000/login> and choose **Create one** (self-service sign-up is on
by default in development; set `ALLOW_REGISTRATION=false` to turn it off, and it is off by default when
`APP_ENV` is not `development`). Passwords need at least 10 characters. The session lives in an `httpOnly`
cookie; the browser never sees the token.

Set a real signing key before using the stack anywhere other than this machine. Compose otherwise falls back to a
**publicly known** default:

```bash
echo "JWT_SECRET=$(openssl rand -hex 32)" >> .env   # then: docker compose up -d --build backend worker frontend
```

Telegram credentials (`TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`) are server-side environment variables only; they are
never stored by the app or accepted through the UI.

> **Port 5432 trap (macOS):** a Homebrew/host Postgres may own `127.0.0.1:5432` and shadow the container, giving
> `role "halal_user" does not exist` on `localhost`. Reach the container through your LAN address, or use
> `docker exec halal_equity_postgres psql -U halal_user -d halal_equity`.

---

## 3. Automated End-to-End Verification

Verify the entire system end-to-end with the automated verification script:

```bash
python3 scripts/verify_e2e_delivery.py
```

Expected output:
```
======================================================================
HALALEQUITY END-TO-END DELIVERY VERIFICATION
======================================================================
[PASS] Backend Health: Code 200, status=healthy
[PASS] Quant Engine Health: Code 200, status=healthy
[PASS] Market Data Health: Code 200
[PASS] Stock Universe List (via Proxy): Found 51 active securities
[PASS] Market Overview Route: TotalEquities=51, Sectors=9
[PASS] TCS Stock Detail: Price=₹2520.3999, Shariah=DATA_INSUFFICIENT
[PASS] Shariah Screening Universe: Retrieved 130 screenings
[PASS] IPO Pipeline Ingestion: Retrieved 8 IPO records
[PASS] Quant Breakout Signals: Retrieved 0 signals
[PASS] Portfolio Mark-to-Market: Cash=₹1000000, Equity=₹1000000
[PASS] Shariah Gate Enforcement (BUY Blocked for DATA_INSUFFICIENT): Result=...
[PASS] Short Selling Restriction Enforcement: Result=...
[PASS] Paper Trading Session Safety State: PaperTrading=True, LiveExecution=False
[PASS] Risk Position Sizing Calculator: SuggestedShares=150, R:R=3, RiskPerShare=₹100
[PASS] Portfolio Risk Metrics: ExecutionMode=PAPER_TRADING, MaxHeat=10%, MaxRiskTrade=2%
[PASS] Data Quality Dashboard: Datasets=4
[PASS] Data Quality Quarantine Issues List: Retrieved 25 quarantine records
[PASS] Strategy V2 Research Gate & Parameter Verification: Status=RESEARCH_BLOCKED, QuarantinedBars=23132...
======================================================================
VERIFICATION SUMMARY: 18/18 checks passed.
======================================================================
```

P1.9.20 end-to-end check through the real login flow (creates one throwaway `e2e-*@example.test` account; set
`E2E_EMAIL`/`E2E_PASSWORD` to reuse an account instead):
```bash
python3 scripts/verify_p1920_e2e.py     # 52 checks
```

To exercise all 11 live interactive application workflows and security negative inputs:
```bash
python3 scripts/test_workflows_live.py
```

---

## 4. Running Backend & Quant Regression Tests

### 4.1 Go Backend Integration Tests
```bash
scripts/run_go_tests.sh ./...        # discovers the container's address (see the port 5432 note above)
```
The DB-backed tests use the isolated `halal_equity_test` database; the few that exercise the real runtime role
(`getAppTestDB`) connect to `halal_equity`, create rows with unique names, and clean up after themselves.
*Expected: 100% of tests pass, specifically:*
- `TestPaperTradingExactThreeStepLifecycle`: Reproduces the exact 3-step lifecycle (BUY 100 @ 1,000; SELL 40 @ 1,200; SELL 60 @ 1,100), logging each statutory charge, cost basis, realized P&L, and zero discrepancy against initial capital.
- `TestIsTradingHolidayNSECalendar2024to2026`: Validates the official exchange holiday calendar (`NSE_EQ_CALENDAR_2024-10_to_2026-12_V1`).
- `TestConcurrentOrdersAtomicity`: Proves atomic row-locking prevents overspending across concurrent orders.
- `TestP1813DataReadinessAndAdjustedPriceAudit`: Strictly verifies the 23,132 quarantined bar invariant.

### 4.2 Python Quant Engine Pytest Suite
```bash
docker exec halal_equity_quant pytest -v
```
*Expected: 381 passed, 1 skipped in ~70 seconds.*

### 4.3 Frontend Production Build Audit
```bash
cd frontend
npm run build
```
*Expected: Next.js 16 compiles cleanly in ~1.5s with Turbopack.*

---

## 5. Non-Negotiable Operational Safety Constraints

Before operating or testing the platform, ensure environment safety variables remain strictly locked:

```bash
PAPER_TRADING_ENABLED=true
LIVE_ORDER_EXECUTION=false
BROKER_TRADING_ENABLED=false
```

- **Shariah Gate:** BUY requires certified `PASS` status. Equities with `DATA_INSUFFICIENT` or `FAIL` are blocked fail-closed.
- **Short Selling:** Strictly blocked. The engine enforces Long-Only cash delivery trades.
- **Backtesting Gate:** Strictly gated under `RESEARCH_BLOCKED` while 23,132 historical daily bars remain quarantined.
- **Frozen Strategy V2 Parameters:** BUY=80.0, SELL=45.0, ATR=2.0 (Canonical SHA-256: `92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780`).
