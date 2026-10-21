#!/bin/sh
# HalalEquity P1.8.10 - Legacy Factor Isolation, Formula Corrections & Fail-Closed Validation Audit Suite
set -eu

echo "======================================================================"
echo "HALALEQUITY P1.8.10: FACTOR CONSUMER & FORMULA HARDENING AUDIT SUITE"
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

echo "\n### 2. Corporate Action Factor Ledger All Historical Revisions"
$PG -c "
SELECT 
    c.id, s.symbol, c.action_type, c.factor_category, c.event_id, c.revision_number, c.ex_date,
    c.price_factor_applicable, c.adjustment_factor, c.nominal_price_factor, c.total_return_factor,
    c.volume_adjustment_factor, c.tax_cost_allocation_pct, c.cum_price_reference, c.ex_price_reference,
    c.is_verified, c.dispute_status
FROM corporate_action_adjustment_factors c
JOIN symbols s ON c.symbol_id = s.id
ORDER BY s.symbol, c.revision_number ASC;
"

echo "\n### 3. Dedicated ca Schema Deterministic Views"
echo "\n--- [ca.v_latest_corporate_action_factors] ---"
$PG -c "
SELECT symbol, action_type, factor_category, event_id, revision_number, price_factor_applicable, total_return_factor, is_verified, dispute_status
FROM ca.v_latest_corporate_action_factors
ORDER BY symbol;
"

echo "\n--- [ca.v_active_price_adjustment_factors] (Strict Price Factor Filter) ---"
$PG -c "
SELECT symbol, action_type, factor_category, event_id, revision_number, price_factor_applicable, nominal_price_factor, total_return_factor, volume_adjustment_factor, is_verified, dispute_status
FROM ca.v_active_price_adjustment_factors
ORDER BY symbol;
"

echo "\n--- [ca.v_tax_cost_allocations] (Statutory Section 49(2C) Tax Apportionment) ---"
$PG -c "
SELECT symbol, action_type, factor_category, revision_number, tax_cost_allocation_pct, details
FROM ca.v_tax_cost_allocations
ORDER BY symbol;
"

echo "\n### 4. Database Trigger Guards (Append-Only + Fail-Closed Ingestion)"
$PG -c "SELECT tgname, tgtype, proname FROM pg_trigger t JOIN pg_proc p ON t.tgfoid = p.oid WHERE tgname IN ('trg_ca_factor_guard', 'trg_ca_factor_ingest_guard');"

echo "\n### 5. Strategy Governance & Canonical Frozen Hash Lock"
curl -s http://localhost:8000/api/v1/governance/strategy | grep -E "runtime_hash|pinned_hash|matches_pinned"

echo "\n### 6. Quant Engine Pytest Suite (All 241 test modules)"
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
echo "ALL P1.8.10 OPERATIONAL AUDIT STEPS COMPLETED SUCCESSFULLY."
echo "======================================================================"
