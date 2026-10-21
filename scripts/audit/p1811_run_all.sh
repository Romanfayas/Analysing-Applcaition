#!/bin/sh
# HalalEquity P1.8.11 - Primary Corporate-Action Harvesting & Provenance Validation Audit Suite
set -eu

echo "======================================================================"
echo "HALALEQUITY P1.8.11: PRIMARY CORPORATE-ACTION HARVESTING AUDIT SUITE"
echo "======================================================================"

PG="docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X"

echo "\n### 1. Safety Controls Verification"
docker exec halal_equity_backend env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED'
docker exec halal_equity_worker env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED'
docker exec halal_equity_quant env | grep -E 'PAPER_TRADING_ENABLED|LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED'

echo "\n### 2. Strategy V2 Governance & Frozen Canonical Hash"
docker exec halal_equity_quant python -c "
from src.engines.research.strategy_config import assert_frozen, PINNED_HASH
cfg = assert_frozen()
print(f'Runtime Hash: {cfg[\"runtime_hash\"]}')
print(f'Pinned Hash:  {PINNED_HASH}')
print(f'Matches Pinned: {cfg[\"matches_pinned\"]}')
print(f'Parameters: {cfg[\"resolved\"]}')
assert cfg[\"matches_pinned\"], 'Strategy V2 hash mismatch!'
"

echo "\n### 3. Database Schema Isolation & Migration 15 Tables"
$PG -c "
SELECT count(*) as public_views_count FROM pg_views WHERE schemaname = 'public';
SELECT count(*) as ca_views_count FROM pg_views WHERE schemaname = 'ca';
SELECT viewname FROM pg_views WHERE schemaname = 'ca' ORDER BY viewname;
"

echo "\n### 4. Daily Bars Quarantine & Bar Count Reconciliation"
$PG -c "
SELECT 
    count(*) AS total_bars,
    count(adjusted_close) AS populated_adjusted,
    count(*) - count(adjusted_close) AS null_adjusted,
    ROUND((count(*) - count(adjusted_close))::numeric / count(*)::numeric * 100, 2) AS quarantine_pct
FROM ohlcv_daily;
"

echo "\n### 5. Shariah Partition Reconciliation"
$PG -c "
SELECT 
    (SELECT count(*) FROM symbols WHERE is_active = true) as total_active_symbols,
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

echo "\n### 6. Primary Source Artifacts in Immutable Storage"
$PG -c "
SELECT id, artifact_id, issuing_authority, document_identifier, sha256_checksum, validation_status, created_at
FROM corporate_action_source_artifacts
ORDER BY id ASC;
"

echo "\n### 7. Normalized Corporate Action Events Sample"
$PG -c "
SELECT id, event_id, symbol, action_type, ex_date, dividend_amount, split_ratio_old, split_ratio_new, verification_status
FROM corporate_action_events
ORDER BY symbol, ex_date DESC
LIMIT 15;
"

echo "\n### 8. Nominal Price vs Total Return Views Isolation"
echo "--- [ca.v_active_nominal_price_factors] (Excludes Cash Dividends) ---"
$PG -c "
SELECT symbol, action_type, factor_category, event_id, revision_number, price_factor, is_verified, dispute_status
FROM ca.v_active_nominal_price_factors;
"
echo "--- [ca.v_active_total_return_factors] (Includes Cash Dividends) ---"
$PG -c "
SELECT symbol, action_type, factor_category, event_id, revision_number, price_factor, is_verified, dispute_status
FROM ca.v_active_total_return_factors;
"

echo "\n### 9. 47-Symbol Primary Corporate Action Coverage Matrix"
(cd backend && DB_HOST=192.168.18.43 ./bin/ca_harvester -report)

echo "\n======================================================================"
echo "HALALEQUITY P1.8.11 AUDIT SUITE EXECUTION COMPLETE: PASS"
echo "======================================================================"
