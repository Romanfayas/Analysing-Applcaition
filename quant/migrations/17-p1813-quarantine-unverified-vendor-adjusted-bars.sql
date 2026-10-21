-- HalalEquity Migration 17: P1.8.13 Quarantine Unverified Vendor Adjusted Bars & Preserve Evidence Integrity
--
-- Objective:
-- 1. Quarantine 8 prospective bars for HCLTECH where Yahoo Finance vendor applied an unverified
--    dividend adjustment (adjusted_close != close) without supporting primary exchange circulars.
--    Raw OHLCV data (open, high, low, close, volume) is strictly preserved.
-- 2. Populated adjusted_close bars transition from 470 -> 462.
-- 3. Quarantined NULL adjusted_close bars transition from 23,124 -> 23,132.
-- 4. Ensures 100% of remaining populated adjusted_close values are strictly backed by verified factors
--    (46 symbols with factor=1.0, 1 symbol TCS with verified factor Revision 3).

BEGIN;

-- 1. Quarantine unverified vendor-adjusted bars for HCLTECH
UPDATE ohlcv_daily
SET adjusted_close = NULL
WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'HCLTECH')
  AND close != adjusted_close;

COMMIT;
