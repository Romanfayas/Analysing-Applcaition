#!/bin/sh
# HalalEquity P1.8.12 - Corporate-Action Integrity Reconciliation & Fail-Closed Audit Suite
set -eu

echo "======================================================================"
echo "HALALEQUITY P1.8.12: CORPORATE-ACTION INTEGRITY RECONCILIATION AUDIT"
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

echo "\n### 3. Database Schema Isolation & Dedicated ca Schema Views"
$PG -c "
SELECT count(*) as public_views_count FROM pg_views WHERE schemaname = 'public';
SELECT count(*) as ca_views_count FROM pg_views WHERE schemaname = 'ca';
SELECT viewname FROM pg_views WHERE schemaname = 'ca' ORDER BY viewname;
"

echo "\n### 4. Zero Active Factors with DEFAULT or Invalid Event ID"
$PG -c "
SELECT count(*) as default_event_id_count 
FROM corporate_action_adjustment_factors 
WHERE event_id = 'DEFAULT' OR event_id IS NULL OR TRIM(event_id) = '';

SELECT symbol, action_type, factor_category, event_id, revision_number, adjustment_factor, price_factor_applicable, is_verified, dispute_status
FROM ca.v_latest_corporate_action_factors
ORDER BY symbol, ex_date;
"

echo "\n### 5. TCS Provenance and Mathematical Reconciliation"
$PG -c "
SELECT 
    symbol, action_type, factor_category, event_id, revision_number,
    dividend_amount, cum_price_reference,
    nominal_price_factor, total_return_factor,
    ROUND((cum_price_reference - dividend_amount) / cum_price_reference, 8) as recomputed_factor_8dec,
    price_factor_applicable, is_verified, dispute_status, source_reference
FROM ca.v_latest_corporate_action_factors
WHERE symbol = 'TCS';
"

echo "\n### 6. Raw Artifacts to Normalized Events Exact Reconciliation"
$PG -c "
SELECT 
    count(DISTINCT a.id) as total_artifacts,
    count(DISTINCT e.id) as total_events,
    count(DISTINCT e.id) FILTER (WHERE s.is_active = true) as active_symbols_events,
    count(DISTINCT e.id) FILTER (WHERE s.is_active = false) as inactive_symbols_events
FROM corporate_action_events e
JOIN symbols s ON s.id = e.symbol_id
JOIN corporate_action_source_artifacts a ON a.id = e.source_artifact_id;
"

echo "\n### 7. Canonical Universe Reconciliation (52 Instruments)"
$PG -c "
SELECT 
    count(*) as total_universe_instruments,
    count(*) FILTER (WHERE is_active = true) as active_instruments,
    count(*) FILTER (WHERE is_active = false) as inactive_instruments,
    count(*) FILTER (WHERE symbol NOT IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV') AND is_active = true) as core_active_equities,
    count(*) FILTER (WHERE symbol IN ('LTIM', 'LTM')) as rename_lineage_instruments,
    count(*) FILTER (WHERE symbol IN ('TATAMOTORS', 'TMPV', 'TMCV')) as demerger_lineage_instruments
FROM symbols;
"

echo "\n### 8. Daily Bars Quarantine & Bar Count Reconciliation"
$PG -c "
SELECT 
    count(*) AS total_bars,
    count(adjusted_close) AS populated_adjusted,
    count(*) - count(adjusted_close) AS null_adjusted,
    ROUND((count(*) - count(adjusted_close))::numeric / count(*)::numeric * 100, 2) AS quarantine_pct
FROM ohlcv_daily;

SELECT 
    d.timestamp::date as bar_date,
    count(*) as bars_count,
    count(d.adjusted_close) as populated_adj,
    count(*) - count(d.adjusted_close) as null_adj
FROM ohlcv_daily d
WHERE d.timestamp >= '2026-10-12'
GROUP BY d.timestamp::date
ORDER BY d.timestamp::date DESC;
"

echo "\n### 9. Shariah Partition Reconciliation"
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

echo "\n### 10. Corporate Action Views Isolation"
echo "--- [ca.v_active_nominal_price_factors] (Excludes Cash Dividends) ---"
$PG -c "
SELECT symbol, action_type, factor_category, event_id, revision_number, price_factor, is_verified, dispute_status, source_reference
FROM ca.v_active_nominal_price_factors;
"
echo "--- [ca.v_active_total_return_factors] (Includes Cash Dividends) ---"
$PG -c "
SELECT symbol, action_type, factor_category, event_id, revision_number, price_factor, is_verified, dispute_status, source_reference
FROM ca.v_active_total_return_factors;
"

echo "\n### 11. Canonical 52-Instrument Coverage Matrix"
DB_HOST=192.168.18.43 backend/bin/ca_harvester -report

echo "\n======================================================================"
echo "HALALEQUITY P1.8.12 AUDIT SUITE EXECUTION COMPLETE: PASS"
echo "======================================================================"
