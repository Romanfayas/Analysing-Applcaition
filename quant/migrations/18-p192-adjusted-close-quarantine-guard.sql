-- HalalEquity Migration 18: P1.9.2 Adjusted-Close Quarantine Integrity Guard & Explicit Source-Trust Policy
--
-- Objectives:
-- 1. Preserve source provenance for existing verified adjustments (e.g. TCS Rev 3 dividend bars).
-- 2. Enforce explicit source-trust policy on ohlcv_daily via database trigger:
--    - Quarantined NULL adjusted_close values remain strictly NULL during ordinary vendor ingestion.
--    - Existing verified non-NULL adjustments cannot be overwritten by untrusted vendor feeds.
--    - Provenance source of verified adjustments is guarded against downgrade.
--    - Vendor attempts to supply unverified adjusted_close != close on insert are quarantined to NULL.
--    - Only explicitly authorized corporate-action workflows ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE')
--      may populate or revise adjusted_close.

BEGIN;

-- 1. Restore verified provenance source for authoritative corporate-action bars
UPDATE ohlcv_daily
SET source = 'NSE_CORPORATE_ACTION'
WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS')
  AND adjusted_close IS NOT NULL
  AND adjusted_close != close;

-- 2. Define the fail-closed quarantine and provenance guard trigger function
CREATE OR REPLACE FUNCTION fn_ohlcv_daily_quarantine_guard()
RETURNS TRIGGER AS $$
BEGIN
    -- Handle UPDATEs
    IF TG_OP = 'UPDATE' THEN
        -- Invariant 1: Quarantined NULL values must remain NULL during ordinary vendor ingestion
        IF OLD.adjusted_close IS NULL AND NEW.adjusted_close IS NOT NULL THEN
            IF NEW.source NOT IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN
                NEW.adjusted_close := NULL;
            END IF;
        END IF;

        -- Invariant 2: Verified non-NULL adjustments from trusted authority cannot be overwritten by vendors
        IF OLD.adjusted_close IS NOT NULL AND OLD.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN
            IF NEW.source NOT IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN
                NEW.adjusted_close := OLD.adjusted_close;
                NEW.source := OLD.source;
            END IF;
        END IF;

        -- Invariant 3: General preservation of existing non-NULL adjusted_close against untrusted vendor drift
        IF OLD.adjusted_close IS NOT NULL AND NEW.source NOT IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN
            NEW.adjusted_close := OLD.adjusted_close;
        END IF;

        RETURN NEW;
    END IF;

    -- Handle INSERTs
    IF TG_OP = 'INSERT' THEN
        -- Invariant 4: Unverified vendor feeds cannot insert unverified adjustments (adjusted_close != close)
        IF NEW.source NOT IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN
            IF NEW.adjusted_close IS NOT NULL AND NEW.adjusted_close != NEW.close THEN
                NEW.adjusted_close := NULL;
            END IF;
        END IF;

        RETURN NEW;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 3. Install the trigger on ohlcv_daily
DROP TRIGGER IF EXISTS trg_ohlcv_daily_quarantine_guard ON ohlcv_daily;
CREATE TRIGGER trg_ohlcv_daily_quarantine_guard
BEFORE INSERT OR UPDATE ON ohlcv_daily
FOR EACH ROW
EXECUTE FUNCTION fn_ohlcv_daily_quarantine_guard();

COMMIT;
