-- HalalEquity Migration 34: P1.9.24 keep the public schema free of views
--
-- An existing invariant (test_no_views_expose_intraday_as_daily) forbids any view in the public schema.
-- The vendor-vs-exchange reconciliation view therefore lives in a dedicated read-only `evidence` schema,
-- alongside the existing `ca` schema convention.

BEGIN;

DROP VIEW IF EXISTS public.v_daily_bar_reconciliation;
CREATE SCHEMA IF NOT EXISTS evidence;
REVOKE CREATE ON SCHEMA evidence FROM PUBLIC;
GRANT USAGE ON SCHEMA evidence TO halal_app_user;

-- Vendor (ohlcv_daily) vs exchange (bhavcopy) reconciliation, per stored bar.
CREATE VIEW evidence.v_daily_bar_reconciliation AS
WITH v AS (
    SELECT d.symbol_id, (d.timestamp AT TIME ZONE 'Asia/Kolkata')::date AS trade_date,
           d.open, d.high, d.low, d.close, d.volume, d.adjusted_close
    FROM ohlcv_daily d
)
SELECT s.symbol,
       COALESCE(v.symbol_id, e.symbol_id) AS symbol_id,
       COALESCE(v.trade_date, e.trade_date) AS trade_date,
       v.close  AS vendor_close,
       e.close  AS exchange_close,
       v.adjusted_close IS NULL AS vendor_quarantined,
       CASE WHEN v.close IS NOT NULL AND e.close IS NOT NULL THEN round(v.close / e.close, 6) END AS close_ratio,
       CASE
           WHEN e.symbol_id IS NULL AND f.id IS NULL THEN 'NO_EXCHANGE_FILE'
           WHEN e.symbol_id IS NULL THEN 'MISSING_EXCHANGE_BAR'
           WHEN v.symbol_id IS NULL THEN 'MISSING_VENDOR_BAR'
           WHEN abs(v.close / e.close - 1) <= 0.0005 AND abs(v.open / e.open - 1) <= 0.0005
            AND abs(v.high / e.high - 1) <= 0.0005 AND abs(v.low / e.low - 1) <= 0.0005 THEN 'NOMINAL_MATCH'
           WHEN abs((v.close / e.close) - (v.open / e.open)) <= 0.0005
            AND abs((v.close / e.close) - (v.high / e.high)) <= 0.0005 THEN 'VENDOR_ADJUSTED'
           ELSE 'MISMATCH'
       END AS classification,
       CASE WHEN v.volume IS NOT NULL AND e.volume > 0 THEN round(v.volume::numeric / e.volume, 6) END AS volume_ratio
FROM v
FULL OUTER JOIN exchange_daily_bars e ON e.symbol_id = v.symbol_id AND e.trade_date = v.trade_date
LEFT JOIN exchange_source_files f ON f.trade_date = COALESCE(v.trade_date, e.trade_date)
JOIN symbols s ON s.id = COALESCE(v.symbol_id, e.symbol_id);

GRANT SELECT ON evidence.v_daily_bar_reconciliation TO halal_app_user;

COMMIT;
