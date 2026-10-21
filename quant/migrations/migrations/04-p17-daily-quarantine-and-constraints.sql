-- Migration 04 (P1.7): quarantine residual invalid daily bars, null the bogus adjusted_close, add DB-level guards.
-- Pre-req: CSV backups in backups/ (verified restorable; see P1.7 report). Rollback: 04-rollback.sql
-- Rows are COPIED to ohlcv_daily_quarantine_p17 before being removed from ohlcv_daily (nothing is lost).
BEGIN;

CREATE TABLE IF NOT EXISTS ohlcv_daily_quarantine_p17 (
    LIKE ohlcv_daily,
    quarantine_reason TEXT NOT NULL,
    quarantined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (symbol_id, "timestamp")
);

-- Exchange-holiday dates (versioned calendar NSE_EQ_CALENDAR_..._V1) that still carry zero-volume phantom bars
INSERT INTO ohlcv_daily_quarantine_p17
SELECT o.*, 'HOLIDAY_PHANTOM: zero-volume flat bar on exchange holiday', NOW()
FROM ohlcv_daily o
WHERE o.volume = 0 AND o.open = o.close AND o.high = o.low
  AND (o.timestamp AT TIME ZONE 'Asia/Kolkata')::date IN ('2026-01-15','2026-05-28','2026-06-26','2026-09-14')
ON CONFLICT DO NOTHING;

-- Provider-gap bars on genuine trading sessions: invalid data, session is MISSING (not a holiday)
INSERT INTO ohlcv_daily_quarantine_p17
SELECT o.*, 'PROVIDER_GAP: zero-volume flat bar on a real trading session', NOW()
FROM ohlcv_daily o
WHERE o.volume = 0 AND o.open = o.close AND o.high = o.low
  AND (o.timestamp AT TIME ZONE 'Asia/Kolkata')::date IN ('2025-03-18','2025-09-08')
ON CONFLICT DO NOTHING;

DO $$ DECLARE n_q int; n_del int; BEGIN
  SELECT count(*) INTO n_q FROM ohlcv_daily_quarantine_p17;
  DELETE FROM ohlcv_daily o USING ohlcv_daily_quarantine_p17 q
   WHERE o.symbol_id = q.symbol_id AND o."timestamp" = q."timestamp";
  GET DIAGNOSTICS n_del = ROW_COUNT;
  IF n_q <> n_del THEN RAISE EXCEPTION 'quarantine/delete mismatch % vs %', n_q, n_del; END IF;
  RAISE NOTICE 'quarantined % rows', n_q;
END $$;

-- adjusted_close was written as 0.0000 whenever the provider returned none (never a valid adjustment).
UPDATE ohlcv_daily SET adjusted_close = NULL WHERE adjusted_close = 0;

-- DB-level guards (previously only app-level checks existed)
ALTER TABLE ohlcv_daily ADD CONSTRAINT ck_ohlcv_daily_ohlc
  CHECK (high >= low AND open > 0 AND close > 0 AND low > 0 AND high >= GREATEST(open, close) AND low <= LEAST(open, close) AND volume >= 0);
ALTER TABLE ohlcv_daily ADD CONSTRAINT ck_ohlcv_daily_adj_positive
  CHECK (adjusted_close IS NULL OR adjusted_close > 0);
-- daily bars are stamped at the NSE open, 09:15 IST = 03:45 UTC; anything else is not a daily bar
ALTER TABLE ohlcv_daily ADD CONSTRAINT ck_ohlcv_daily_session_stamp
  CHECK (("timestamp" AT TIME ZONE 'UTC')::time = TIME '03:45:00');
-- no zero-volume flat (phantom) bars
ALTER TABLE ohlcv_daily ADD CONSTRAINT ck_ohlcv_daily_no_phantom
  CHECK (NOT (volume = 0 AND open = close AND high = low));
ALTER TABLE ohlcv_intraday ADD CONSTRAINT ck_ohlcv_intraday_interval
  CHECK (interval_minutes IN (1, 5, 15, 30, 60));

COMMIT;
