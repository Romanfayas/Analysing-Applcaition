#!/bin/sh
# HalalEquity P1.8.9 - Corporate-Action Factor Validation & Safe Reconstruction Audit Suite
set -eu

echo "======================================================================"
echo "HALALEQUITY P1.8.9: FACTOR VALIDATION & DISPUTE AUDIT SUITE"
echo "======================================================================"

PG="docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X"

echo "\n### 1. Active Symbol Universe & Shariah Partition"
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

echo "\n### 2. Corporate Action Factor Ledger Revisions & Semantic Separation (WS2, WS3, WS4)"
$PG -c "
SELECT 
    c.id, s.symbol, c.action_type, c.factor_category, c.revision_number, c.ex_date,
    c.price_factor_applicable, c.adjustment_factor, c.nominal_price_factor, c.total_return_factor,
    c.tax_cost_allocation_pct, c.cum_price_reference, c.ex_price_reference,
    c.is_verified, c.verification_status, c.dispute_status
FROM corporate_action_adjustment_factors c
JOIN symbols s ON c.symbol_id = s.id
ORDER BY s.symbol, c.revision_number ASC;
"

echo "\n### 3. Append-Only Immutability Trigger Guard (WS1 & WS8)"
$PG -c "SELECT tgname, tgtype FROM pg_trigger WHERE tgname = 'trg_ca_factor_guard';"

echo "\n### 4. Active Factor Resolution (Latest Revision Per Event via CTE)"
$PG -c "
WITH latest_factors AS (
    SELECT DISTINCT ON (s.symbol, c.ex_date, c.action_type)
        c.id, s.symbol, c.action_type, c.factor_category, c.revision_number, c.ex_date,
        c.price_factor_applicable, c.nominal_price_factor, c.total_return_factor,
        c.is_verified, c.dispute_status, c.dispute_reason
    FROM corporate_action_adjustment_factors c
    JOIN symbols s ON c.symbol_id = s.id
    ORDER BY s.symbol, c.ex_date, c.action_type, c.revision_number DESC
)
SELECT symbol, action_type, revision_number, factor_category, price_factor_applicable, total_return_factor, is_verified, dispute_status
FROM latest_factors;
"

echo "\n### 5. Strategy Governance & Frozen Hash Lock"
curl -s http://localhost:8000/api/v1/governance/strategy | grep -E "runtime_hash|pinned_hash|matches_pinned"

echo "\n### 6. Quant Engine Pytest Suite (All 233 test modules)"
docker exec halal_equity_quant pytest -q

echo "\n### 7. Go Backend Test Suites (Uncached, 5 packages)"
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
echo "ALL P1.8.9 OPERATIONAL AUDIT STEPS COMPLETED SUCCESSFULLY."
echo "======================================================================"
