#!/bin/sh
# HalalEquity P1.8.6 - Complete Verification & Research Readiness Suite
set -eu

echo "======================================================================"
echo "HALALEQUITY P1.8.6: COST-LEDGER & RESEARCH READINESS SUITE"
echo "======================================================================"

PG="docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X"

echo "\n### 1. Active Symbol Universe & Shariah Partition Reconciliation (WS4)"
$PG -c "
SELECT 
    (SELECT count(*) FROM symbols WHERE is_active = true) as total_active_symbols,
    (SELECT count(*) FROM symbols WHERE is_active = false) as inactive_symbols,
    (SELECT count(*) FROM (
        SELECT DISTINCT ON (s.id) sc.status
        FROM symbols s JOIN shariah_screenings sc ON s.id = sc.symbol_id
        WHERE s.is_active = true
        ORDER BY s.id, sc.screening_date DESC, sc.screened_at DESC, sc.id DESC
    ) t WHERE status = 'PASS') as pass_count,
    (SELECT count(*) FROM (
        SELECT DISTINCT ON (s.id) sc.status
        FROM symbols s JOIN shariah_screenings sc ON s.id = sc.symbol_id
        WHERE s.is_active = true
        ORDER BY s.id, sc.screening_date DESC, sc.screened_at DESC, sc.id DESC
    ) t WHERE status = 'FAIL') as fail_count,
    (SELECT count(*) FROM (
        SELECT DISTINCT ON (s.id) sc.status
        FROM symbols s JOIN shariah_screenings sc ON s.id = sc.symbol_id
        WHERE s.is_active = true
        ORDER BY s.id, sc.screening_date DESC, sc.screened_at DESC, sc.id DESC
    ) t WHERE status = 'DATA_INSUFFICIENT') as data_insufficient_count;
"

echo "\n### 2. Corporate Action Adjustment Factor Ledger & Immutability Trigger (WS2)"
$PG -c "
SELECT s.symbol, c.action_type, c.ex_date, c.adjustment_factor, c.source_reference, c.is_verified, c.verification_status 
FROM corporate_action_adjustment_factors c
JOIN symbols s ON c.symbol_id = s.id
ORDER BY c.ex_date DESC;
"
$PG -c "SELECT tgname, tgtype FROM pg_trigger WHERE tgname = 'trg_ca_factor_guard';"

echo "\n### 3. Transaction Cost Dual Accounting Identities & Footing Proof (WS1)"
docker cp scripts/audit/p17_cost_reconciliation.py halal_equity_quant:/tmp/p17_cost.py
docker exec halal_equity_quant python3 /tmp/p17_cost.py

echo "\n### 4. Strategy Governance & Frozen Hash Lock (WS6)"
curl -s http://localhost:8000/api/v1/governance/strategy | grep -E "runtime_hash|pinned_hash|matches_pinned"

echo "\n### 5. Quant Engine Pytest Suite (All test modules)"
docker exec halal_equity_quant pytest -q

echo "\n### 6. Go Backend Test Suites (Uncached)"
(cd backend && go test -count=1 ./internal/database ./internal/handlers ./internal/providers/marketdata/yahoofinance ./internal/services/market ./internal/workers/ingestion)

echo "\n### 7. Independent Market-Data Coverage Reconciliation (WS3)"
docker cp scripts/audit/p184_market_coverage_reconcile.py halal_equity_quant:/app/p184_market_coverage_reconcile.py
docker exec halal_equity_quant python3 /app/p184_market_coverage_reconcile.py

echo "\n### 8. Next.js Frontend Production Build"
(cd frontend && npm run build)

echo "\n### 9. Execution Safety Flags"
echo "Backend: $(docker exec halal_equity_backend env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"
echo "Worker:  $(docker exec halal_equity_worker env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"
echo "Quant:   $(docker exec halal_equity_quant env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"

echo "\n### 10. Durable EOD Runs & Idempotency Audit (WS5)"
$PG -c "
SELECT id, run_id, trigger_type, provider, started_at, status, symbols_attempted, candles_fetched, candles_stored, candles_failed, details
FROM eod_ingestion_runs
ORDER BY started_at DESC LIMIT 3;
"

echo "\n### 11. Prospective Paper Observations Sample Size"
$PG -c "
SELECT 
    status, outcome, is_complete, COUNT(*) 
FROM paper_observations 
GROUP BY status, outcome, is_complete;
"

echo "\n======================================================================"
echo "ALL P1.8.6 OPERATIONAL VERIFICATION STEPS COMPLETED SUCCESSFULLY."
echo "======================================================================"
