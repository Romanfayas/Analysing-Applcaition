#!/bin/sh
# HalalEquity P1.8.4 - Complete Verification & Audit Suite
set -eu

echo "======================================================================"
echo "HALALEQUITY P1.8.4: VERIFICATION SUITE"
echo "======================================================================"

PG="docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X"

echo "\n### 1. SQL data-isolation, financial provenance, and durable ingestion run audit"
$PG -c "
SELECT 
    (SELECT count(*) FROM ohlcv_daily) as daily_clean_bars,
    (SELECT count(DISTINCT timestamp) FROM ohlcv_daily) as daily_sessions,
    (SELECT max(timestamp) FROM ohlcv_daily) as latest_bar,
    (SELECT count(*) FROM financial_statements WHERE provenance IS NOT NULL) as fin_statements_with_provenance,
    (SELECT count(*) FROM shariah_screenings WHERE screening_date = CURRENT_DATE AND status = 'DATA_INSUFFICIENT') as shariah_data_insufficient,
    (SELECT count(*) FROM shariah_screenings WHERE screening_date = CURRENT_DATE AND status = 'FAIL') as shariah_fail,
    (SELECT count(*) FROM shariah_screenings WHERE screening_date = CURRENT_DATE AND status = 'PASS') as shariah_pass,
    (SELECT count(*) FROM eod_ingestion_runs) as eod_runs_logged,
    (SELECT count(*) FROM paper_observations WHERE status = 'COMPLETED') as valid_completed_observations,
    (SELECT count(*) FROM paper_observations WHERE status = 'INVALIDATED') as invalidated_observations;
"

echo "\n### 2. Operational Durable EOD Ingestion Run"
docker exec halal_equity_worker /app/bin/worker --run-eod
$PG -c "SELECT run_id, trigger_type, provider, status, symbols_attempted, candles_stored, candles_failed, retry_count, is_exchange_verified FROM eod_ingestion_runs ORDER BY started_at DESC LIMIT 3;"

echo "\n### 3. Transaction Cost Model Verification (COST_MODEL_2026_Q4_V2)"
docker cp scripts/audit/p17_cost_reconciliation.py halal_equity_quant:/tmp/p17_cost.py
docker exec halal_equity_quant python3 /tmp/p17_cost.py

echo "\n### 4. Strategy Governance & Hash Lock Verification"
curl -s http://localhost:8000/api/v1/governance/strategy | grep -E "runtime_hash|pinned_hash|matches_pinned"

echo "\n### 5. Quant Engine Pytest Suite (198 tests)"
docker exec halal_equity_quant pytest -q

echo "\n### 6. Go Backend Test Suites (5 suites uncached)"
(cd backend && go test -count=1 ./internal/database ./internal/handlers ./internal/providers/marketdata/yahoofinance ./internal/services/market ./internal/workers/ingestion)

echo "\n### 7. Independent Market-Data Coverage Reconciliation"
docker cp scripts/audit/p184_market_coverage_reconcile.py halal_equity_quant:/app/p184_market_coverage_reconcile.py
docker exec halal_equity_quant python3 /app/p184_market_coverage_reconcile.py

echo "\n### 8. Next.js Frontend Production Build"
(cd frontend && npm run build)

echo "\n### 9. Container Execution-Safety Flags"
echo "Backend: $(docker exec halal_equity_backend env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"
echo "Worker:  $(docker exec halal_equity_worker env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"
echo "Quant:   $(docker exec halal_equity_quant env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"

echo "\n### 10. Paper Pipeline End-to-End Runner"
docker exec halal_equity_quant python3 /app/p18_pipeline_runner.py

echo "\n======================================================================"
echo "ALL P1.8.4 OPERATIONAL VERIFICATION STEPS COMPLETED SUCCESSFULLY."
echo "======================================================================"
