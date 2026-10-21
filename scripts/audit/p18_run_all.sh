#!/bin/sh
# HalalEquity P1.8 - Operational Paper Pipeline & Data Readiness Verification
set -eu

echo "======================================================================"
echo "HALALEQUITY P1.8: VERIFICATION SUITE"
echo "======================================================================"

PG="docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X"

echo "\n### 1. SQL data-isolation and session audit"
$PG -c "
SELECT 
    (SELECT count(*) FROM ohlcv_daily) as daily_clean_bars,
    (SELECT count(DISTINCT timestamp) FROM ohlcv_daily) as daily_sessions,
    (SELECT max(timestamp) FROM ohlcv_daily) as latest_bar,
    (SELECT count(*) FROM ohlcv_daily_quarantine_p17) as quarantined_p17,
    (SELECT count(*) FROM ohlcv_holiday_phantom_quarantine) as quarantined_p16,
    (SELECT count(*) FROM ohlcv_daily_contamination_backup_20261014) as backup_1min,
    (SELECT count(*) FROM paper_observations) as total_observations;
"

echo "\n### 2. Operational EOD Ingestion Check"
docker exec halal_equity_worker /app/bin/worker --run-eod

echo "\n### 3. Transaction Cost Model Verification (COST_MODEL_2026_Q4_V2)"
docker cp scripts/audit/p17_cost_reconciliation.py halal_equity_quant:/tmp/p17_cost.py
docker exec halal_equity_quant python3 /tmp/p17_cost.py

echo "\n### 4. Strategy Governance & Hash Lock Verification"
curl -s http://localhost:8000/api/v1/governance/strategy | grep -E "runtime_hash|pinned_hash|matches_pinned"

echo "\n### 5. Quant Engine Pytest Suite (172 tests)"
docker exec halal_equity_quant pytest -q

echo "\n### 6. Go Backend Test Suites"
(cd backend && go test ./internal/database ./internal/handlers ./internal/providers/marketdata/yahoofinance ./internal/services/market ./internal/workers/ingestion)

echo "\n### 7. Next.js Frontend Production Build"
(cd frontend && npm run build)

echo "\n### 8. Container Execution-Safety Flags"
echo "Backend: $(docker exec halal_equity_backend env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"
echo "Worker:  $(docker exec halal_equity_worker env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"
echo "Quant:   $(docker exec halal_equity_quant env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"

echo "\n### 9. Paper Pipeline End-to-End Runner"
docker exec halal_equity_quant python3 /app/p18_pipeline_runner.py

echo "\n======================================================================"
echo "ALL P1.8 OPERATIONAL VERIFICATION STEPS COMPLETED SUCCESSFULLY."
echo "======================================================================"
