-- HalalEquity Migration 16: P1.8.12 Eliminate DEFAULT Event Identities & Harden Provenance Constraints
--
-- Objective:
-- 1. Eliminate 'DEFAULT' event_id across corporate_action_adjustment_factors.
-- 2. Backfill stable, deterministic event_ids for existing corporate actions:
--    - LTIM:       'NSE:LTIM:2026-02-27:SYMBOL_CHANGE:1'
--    - TATAMOTORS: 'NSE:TATAMOTORS:2025-10-24:DEMERGER:1'
--    - TCS:        'NSE:TCS:2026-10-14:CASH_DIVIDEND:1'
-- 3. Drop column default 'DEFAULT' on corporate_action_adjustment_factors.event_id.
-- 4. Add check constraint prohibiting 'DEFAULT' or empty event_id (ck_ca_factor_event_id_not_default).
-- 5. Update ca_factor_ingest_guard() to fail closed on any attempt to insert 'DEFAULT' or empty event_id.
-- 6. Refresh views in ca schema:
--    - ca.v_latest_corporate_action_factors
--    - ca.v_active_price_adjustment_factors
--    - ca.v_active_nominal_price_factors
--    - ca.v_active_total_return_factors
--    - ca.v_tax_cost_allocations
--    - ca.v_symbology_lineage

BEGIN;

-- 1. Temporarily disable the immutability guard trigger to backfill event_ids on existing rows
ALTER TABLE corporate_action_adjustment_factors DISABLE TRIGGER trg_ca_factor_guard;

-- 2. Backfill deterministic event_ids for all existing rows
UPDATE corporate_action_adjustment_factors
SET event_id = 'NSE:LTIM:2026-02-27:SYMBOL_CHANGE:1'
WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'LTIM')
  AND (event_id = 'DEFAULT' OR event_id IS NULL OR event_id = '');

UPDATE corporate_action_adjustment_factors
SET event_id = 'NSE:TATAMOTORS:2025-10-24:DEMERGER:1'
WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TATAMOTORS')
  AND (event_id = 'DEFAULT' OR event_id IS NULL OR event_id = '');

UPDATE corporate_action_adjustment_factors
SET event_id = 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1'
WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS')
  AND (event_id = 'DEFAULT' OR event_id IS NULL OR event_id = '');

-- 3. Re-enable immutability guard trigger
ALTER TABLE corporate_action_adjustment_factors ENABLE TRIGGER trg_ca_factor_guard;

-- 4. Drop default 'DEFAULT' from event_id column
ALTER TABLE corporate_action_adjustment_factors ALTER COLUMN event_id DROP DEFAULT;

-- 5. Add check constraint preventing 'DEFAULT' or empty string
ALTER TABLE corporate_action_adjustment_factors
DROP CONSTRAINT IF EXISTS ck_ca_factor_event_id_not_default;

ALTER TABLE corporate_action_adjustment_factors
ADD CONSTRAINT ck_ca_factor_event_id_not_default
CHECK (event_id <> 'DEFAULT' AND TRIM(event_id) <> '');

-- 6. Update ca_factor_ingest_guard() to strictly reject 'DEFAULT' or empty event_id
CREATE OR REPLACE FUNCTION ca_factor_ingest_guard()
RETURNS TRIGGER AS $$
BEGIN
    -- Ensure factor_category is not empty
    IF NEW.factor_category IS NULL OR TRIM(NEW.factor_category) = '' THEN
        NEW.factor_category := 'UNSPECIFIED';
    END IF;

    -- Reject DEFAULT or blank event_id (fail-closed)
    IF NEW.event_id IS NULL OR TRIM(NEW.event_id) = '' OR NEW.event_id = 'DEFAULT' THEN
        RAISE EXCEPTION 'Ingestion rejected: valid deterministic event_id is required; DEFAULT or blank is prohibited (got %)', NEW.event_id;
    END IF;

    -- Non-price categories must fail closed
    IF NEW.factor_category IN ('UNSPECIFIED', 'TAX_COST_ALLOCATION', 'SYMBOLOGY_LINEAGE') THEN
        IF NEW.price_factor_applicable = TRUE THEN
            RAISE EXCEPTION 'Ingestion rejected: factor_category % cannot have price_factor_applicable=true (fail-closed)', NEW.factor_category;
        END IF;
    END IF;

    -- Disputed factors must fail closed
    IF NEW.dispute_status IS NOT NULL AND NEW.dispute_status <> 'UNDISPUTED' THEN
        IF NEW.price_factor_applicable = TRUE THEN
            RAISE EXCEPTION 'Ingestion rejected: disputed factor status (%) cannot have price_factor_applicable=true', NEW.dispute_status;
        END IF;
    END IF;

    -- Unverified factors must fail closed
    IF NEW.is_verified = FALSE AND NEW.price_factor_applicable = TRUE THEN
        RAISE EXCEPTION 'Ingestion rejected: unverified factor cannot have price_factor_applicable=true';
    END IF;

    -- Numerical validity check
    IF NEW.price_factor_applicable = TRUE THEN
        IF NEW.nominal_price_factor <= 0 OR NEW.total_return_factor <= 0 OR NEW.volume_adjustment_factor <= 0 THEN
            RAISE EXCEPTION 'Ingestion rejected: price/volume factors must be positive (nominal=%, total=%, volume=%)',
                NEW.nominal_price_factor, NEW.total_return_factor, NEW.volume_adjustment_factor;
        END IF;
    END IF;

    -- Revision sequence check
    IF NEW.revision_number < 1 THEN
        RAISE EXCEPTION 'Ingestion rejected: revision_number must be >= 1 (got %)', NEW.revision_number;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 7. Refresh ca schema views
CREATE OR REPLACE VIEW ca.v_latest_corporate_action_factors AS
SELECT DISTINCT ON (c.symbol_id, c.ex_date, c.action_type, c.event_id)
    c.id,
    c.symbol_id,
    s.symbol,
    c.action_type,
    c.factor_category,
    c.event_id,
    c.revision_number,
    c.ex_date,
    c.record_date,
    c.effective_date,
    c.split_ratio_old,
    c.split_ratio_new,
    c.dividend_amount,
    c.cash_dividend_per_share,
    c.adjustment_factor,
    c.nominal_price_factor,
    c.total_return_factor,
    c.volume_adjustment_factor,
    c.price_factor_applicable,
    c.tax_cost_allocation_pct,
    c.cum_price_reference,
    c.ex_price_reference,
    c.is_verified,
    c.verification_status,
    c.dispute_status,
    c.dispute_reason,
    c.source_reference,
    c.retrieval_timestamp,
    c.details,
    c.created_at,
    c.updated_at
FROM corporate_action_adjustment_factors c
JOIN symbols s ON s.id = c.symbol_id
ORDER BY c.symbol_id, c.ex_date, c.action_type, c.event_id, c.revision_number DESC;

CREATE OR REPLACE VIEW ca.v_active_price_adjustment_factors AS
SELECT *
FROM ca.v_latest_corporate_action_factors
WHERE price_factor_applicable = TRUE
  AND is_verified = TRUE
  AND dispute_status = 'UNDISPUTED'
  AND factor_category IN ('SHARE_TRANSFORMATION', 'CASH_DISTRIBUTION', 'DILUTION_OFFERING', 'PRICE_ADJUSTMENT');

CREATE OR REPLACE VIEW ca.v_active_nominal_price_factors AS
SELECT 
    symbol, action_type, factor_category, event_id, revision_number,
    ex_date, record_date, effective_date,
    nominal_price_factor AS price_factor,
    volume_adjustment_factor,
    is_verified, dispute_status, source_reference
FROM ca.v_latest_corporate_action_factors
WHERE price_factor_applicable = TRUE
  AND is_verified = TRUE
  AND dispute_status = 'UNDISPUTED'
  AND nominal_price_factor != 1.00000000
  AND factor_category IN ('SHARE_TRANSFORMATION', 'DILUTION_OFFERING', 'PRICE_ADJUSTMENT');

CREATE OR REPLACE VIEW ca.v_active_total_return_factors AS
SELECT 
    symbol, action_type, factor_category, event_id, revision_number,
    ex_date, record_date, effective_date,
    total_return_factor AS price_factor,
    volume_adjustment_factor,
    is_verified, dispute_status, source_reference
FROM ca.v_latest_corporate_action_factors
WHERE price_factor_applicable = TRUE
  AND is_verified = TRUE
  AND dispute_status = 'UNDISPUTED'
  AND total_return_factor != 1.00000000
  AND factor_category IN ('SHARE_TRANSFORMATION', 'DILUTION_OFFERING', 'PRICE_ADJUSTMENT', 'CASH_DISTRIBUTION');

CREATE OR REPLACE VIEW ca.v_tax_cost_allocations AS
SELECT *
FROM ca.v_latest_corporate_action_factors
WHERE factor_category = 'TAX_COST_ALLOCATION';

CREATE OR REPLACE VIEW ca.v_symbology_lineage AS
SELECT *
FROM ca.v_latest_corporate_action_factors
WHERE factor_category = 'SYMBOLOGY_LINEAGE';

COMMIT;
