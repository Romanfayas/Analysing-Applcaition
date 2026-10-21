#!/bin/sh
# HalalEquity P1.8.8 - Corporate-Action Data Integrity & Point-in-Time Reconstruction Audit Suite
set -eu

echo "======================================================================"
echo "HALALEQUITY P1.8.8: CORPORATE-ACTION DATA INTEGRITY AUDIT SUITE"
echo "======================================================================"

PG="docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X"

echo "\n### 1. Active Symbol Universe & Shariah Partition Reconciliation"
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

echo "\n### 2. Database Baseline: Daily Bar Adjustment Coverage & Incomplete History Audit"
$PG -c "
SELECT 
    count(*) as total_daily_records,
    count(distinct symbol_id) as distinct_symbols,
    count(distinct timestamp::date) as distinct_sessions,
    count(adjusted_close) as populated_adj_close,
    count(*) - count(adjusted_close) as null_adj_close,
    round(100.0 * count(adjusted_close) / count(*), 2) as pct_populated,
    round(100.0 * (count(*) - count(adjusted_close)) / count(*), 2) as pct_null
FROM ohlcv_daily;
"

echo "\n### 3. Corporate Action Adjustment Factor Ledger & Immutability Trigger"
$PG -c "
SELECT s.symbol, c.action_type, c.ex_date, c.record_date, c.adjustment_factor, c.source_reference, c.revision_number, c.is_verified, c.verification_status 
FROM corporate_action_adjustment_factors c
JOIN symbols s ON c.symbol_id = s.id
ORDER BY c.ex_date DESC;
"
$PG -c "SELECT tgname, tgtype FROM pg_trigger WHERE tgname = 'trg_ca_factor_guard';"

echo "\n### 4. Quality Status & Quarantine Classifications"
$PG -c "
SELECT quality_status, count(*) as bar_count
FROM ohlcv_daily
GROUP BY quality_status;
"

echo "\n### 5. Strategy Governance & Frozen Hash Lock"
curl -s http://localhost:8000/api/v1/governance/strategy | grep -E "runtime_hash|pinned_hash|matches_pinned"

echo "\n### 6. Quant Engine Pytest Suite (All 228 test modules)"
docker exec halal_equity_quant pytest -q

echo "\n### 7. Go Backend Test Suites (Uncached, including reconciliation invariant)"
(cd backend && go test -count=1 ./internal/database ./internal/handlers ./internal/providers/marketdata/yahoofinance ./internal/services/market ./internal/workers/ingestion)

echo "\n### 8. Next.js Frontend Production Build"
(cd frontend && npm run build)

echo "\n### 9. Execution Safety Flags"
echo "Backend: $(docker exec halal_equity_backend env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"
echo "Worker:  $(docker exec halal_equity_worker env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"
echo "Quant:   $(docker exec halal_equity_quant env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED' | tr '\n' ' ')"

echo "\n### 10. Prospective Paper Observations Sample Size"
$PG -c "
SELECT 
    status, outcome, is_complete, COUNT(*) 
FROM paper_observations 
GROUP BY status, outcome, is_complete;
"

echo "\n======================================================================"
echo "ALL P1.8.8 OPERATIONAL AUDIT STEPS COMPLETED SUCCESSFULLY."
echo "======================================================================"
