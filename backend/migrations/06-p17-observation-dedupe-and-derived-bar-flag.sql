-- Migration 06 (P1.7)
BEGIN;
-- One candidate per (symbol, setup, rule version, IST decision session): resubmission in the same session cannot create a second row.
CREATE UNIQUE INDEX IF NOT EXISTS uq_paper_obs_session
  ON paper_observations (symbol_id, setup_type, rule_version, (((candidate_timestamp AT TIME ZONE 'Asia/Kolkata'))::date));

-- 2026-10-07 daily bars were aggregated from 1-minute rows covering 09:15-15:15 only (~360 of 375 minutes; volume differs from the
-- intraday table). They are derived, not exchange EOD bars: flag SUSPECT until the EOD job replaces them (upsert resets to VALID).
UPDATE ohlcv_daily SET quality_status = 'SUSPECT'
 WHERE (timestamp AT TIME ZONE 'Asia/Kolkata')::date = '2026-10-07' AND quality_status = 'VALID';
-- the old constant 28 bps default was an unsupported placeholder; estimates are now NULL unless computed from the cost ledger
ALTER TABLE paper_observations ALTER COLUMN estimated_costs_bps DROP NOT NULL, ALTER COLUMN estimated_costs_bps DROP DEFAULT;
COMMIT;
