-- Rollback for migration 04 (P1.7). adjusted_close=0 values are restored from backups/p17_pre_ohlcv_daily.csv only if needed (they were invalid).
BEGIN;
ALTER TABLE ohlcv_daily DROP CONSTRAINT IF EXISTS ck_ohlcv_daily_ohlc, DROP CONSTRAINT IF EXISTS ck_ohlcv_daily_adj_positive,
  DROP CONSTRAINT IF EXISTS ck_ohlcv_daily_session_stamp, DROP CONSTRAINT IF EXISTS ck_ohlcv_daily_no_phantom;
ALTER TABLE ohlcv_intraday DROP CONSTRAINT IF EXISTS ck_ohlcv_intraday_interval;
INSERT INTO ohlcv_daily (symbol_id,"timestamp",open,high,low,close,volume,adjusted_close,source,retrieved_at,quality_status)
SELECT symbol_id,"timestamp",open,high,low,close,volume,adjusted_close,source,retrieved_at,quality_status FROM ohlcv_daily_quarantine_p17 ON CONFLICT DO NOTHING;
COMMIT;
