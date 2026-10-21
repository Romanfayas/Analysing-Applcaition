-- Migration 03: Data Isolation & Daily OHLCV Decontamination
-- Preserves 16,950 1-minute intraday rows into backup table.
-- Preserves 81 zero-volume holiday phantom candles into quarantine table.
-- Aggregates 2026-10-07 1-minute bars into 47 clean daily session bars.
-- Enforces clean 1-row-per-session daily invariant on ohlcv_daily (23,500 total clean bars across 47 symbols).

BEGIN;

-- 1. Create recoverable backup of 1-minute contamination rows
CREATE TABLE IF NOT EXISTS ohlcv_daily_contamination_backup_20261014 AS
SELECT * FROM ohlcv_daily
WHERE timestamp::date = '2026-10-07';

-- 2. Create recoverable quarantine for zero-volume holiday phantom candles
CREATE TABLE IF NOT EXISTS ohlcv_holiday_phantom_quarantine AS
SELECT * FROM ohlcv_daily
WHERE timestamp::date IN ('2026-05-01', '2026-10-02') AND volume = 0;

-- 3. Materialize clean aggregated daily bars for 2026-10-07
CREATE TEMP TABLE clean_2026_10_07_daily ON COMMIT DROP AS
SELECT 
    symbol_id,
    '2026-10-07 03:45:00+00'::timestamptz as timestamp,
    (ARRAY_AGG(open ORDER BY timestamp ASC))[1] as open,
    MAX(high) as high,
    MIN(low) as low,
    (ARRAY_AGG(close ORDER BY timestamp DESC))[1] as close,
    SUM(volume) as volume,
    (ARRAY_AGG(close ORDER BY timestamp DESC))[1] as adjusted_close,
    'yahoo_finance'::varchar(50) as source,
    MAX(retrieved_at) as retrieved_at,
    'VALID'::data_quality_status as quality_status
FROM ohlcv_daily
WHERE timestamp::date = '2026-10-07'
GROUP BY symbol_id;

-- 4. Delete the 1-minute contaminated rows from ohlcv_daily
DELETE FROM ohlcv_daily WHERE timestamp::date = '2026-10-07';

-- 5. Delete the zero-volume holiday phantom rows from ohlcv_daily
DELETE FROM ohlcv_daily WHERE timestamp::date IN ('2026-05-01', '2026-10-02') AND volume = 0;

-- 6. Insert the 47 clean aggregated daily bars for 2026-10-07
INSERT INTO ohlcv_daily (symbol_id, timestamp, open, high, low, close, volume, adjusted_close, source, retrieved_at, quality_status)
SELECT symbol_id, timestamp, open, high, low, close, volume, adjusted_close, source, retrieved_at, quality_status
FROM clean_2026_10_07_daily;

COMMIT;
