-- HalalEquity Migration 14: P1.8.10 Fail-Closed Ingestion, Latest-Revision Views & TCS Precision Reconciliation
--
-- Objective:
-- 1. Ensure factor ingestion is strictly fail-closed: price_factor_applicable defaults to FALSE,
--    and trigger blocks any insert with price_factor_applicable=TRUE if category is UNSPECIFIED,
--    TAX_COST_ALLOCATION, or SYMBOLOGY_LINEAGE, or if factor is unverified or disputed.
-- 2. Add event_id to prevent distinct corporate action events on the same symbol and date from merging.
-- 3. Provide deterministic views under dedicated `ca` schema (isolating from public to ensure no views expose intraday as daily):
--    - ca.v_latest_corporate_action_factors: resolves latest revision per event
--    - ca.v_active_price_adjustment_factors: filters strictly valid, verified, undisputed price factors
--    - ca.v_tax_cost_allocations: exposes statutory Section 49(2C) tax allocations without risking price contamination
--    - ca.v_symbology_lineage: exposes ticker rename lineage
-- 4. Reconcile TCS dividend total-return factor to full 8-decimal precision (0.99471203) as Revision 3.

-- 1. Add event_id and update schema defaults to fail-closed
ALTER TABLE corporate_action_adjustment_factors
ADD COLUMN IF NOT EXISTS event_id VARCHAR(64) NOT NULL DEFAULT 'DEFAULT';

ALTER TABLE corporate_action_adjustment_factors
ALTER COLUMN price_factor_applicable SET DEFAULT FALSE;

ALTER TABLE corporate_action_adjustment_factors
ALTER COLUMN factor_category SET DEFAULT 'UNSPECIFIED';

-- Update unique constraint to incorporate event_id
ALTER TABLE corporate_action_adjustment_factors
DROP CONSTRAINT IF EXISTS uq_ca_adjustment_factor;

ALTER TABLE corporate_action_adjustment_factors
ADD CONSTRAINT uq_ca_adjustment_factor UNIQUE (symbol_id, ex_date, action_type, event_id, revision_number);

CREATE INDEX IF NOT EXISTS idx_ca_adj_event_id ON corporate_action_adjustment_factors(symbol_id, event_id, revision_number DESC);

-- 2. Fail-closed Ingestion Trigger
CREATE OR REPLACE FUNCTION ca_factor_ingest_guard()
RETURNS TRIGGER AS $$
BEGIN
    -- Ensure factor_category is not empty
    IF NEW.factor_category IS NULL OR TRIM(NEW.factor_category) = '' THEN
        NEW.factor_category := 'UNSPECIFIED';
    END IF;

    -- Ensure event_id is not empty
    IF NEW.event_id IS NULL OR TRIM(NEW.event_id) = '' THEN
        NEW.event_id := 'DEFAULT';
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

DROP TRIGGER IF EXISTS trg_ca_factor_ingest_guard ON corporate_action_adjustment_factors;
CREATE TRIGGER trg_ca_factor_ingest_guard
BEFORE INSERT ON corporate_action_adjustment_factors
FOR EACH ROW EXECUTE FUNCTION ca_factor_ingest_guard();

-- 3. Dedicated `ca` Schema and Deterministic Views
CREATE SCHEMA IF NOT EXISTS ca;

-- View 1: Latest revision per event (symbol, ex_date, action_type, event_id)
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

-- View 2: Active price adjustment factors (for price reconstruction & backtest engines)
CREATE OR REPLACE VIEW ca.v_active_price_adjustment_factors AS
SELECT *
FROM ca.v_latest_corporate_action_factors
WHERE price_factor_applicable = TRUE
  AND is_verified = TRUE
  AND dispute_status = 'UNDISPUTED'
  AND factor_category IN ('SHARE_TRANSFORMATION', 'CASH_DISTRIBUTION', 'DILUTION_OFFERING', 'PRICE_ADJUSTMENT');

-- View 3: Statutory tax cost allocations (Section 49(2C) demerger apportionment)
CREATE OR REPLACE VIEW ca.v_tax_cost_allocations AS
SELECT *
FROM ca.v_latest_corporate_action_factors
WHERE factor_category = 'TAX_COST_ALLOCATION';

-- View 4: Symbology lineage tracking (renames and successor links)
CREATE OR REPLACE VIEW ca.v_symbology_lineage AS
SELECT *
FROM ca.v_latest_corporate_action_factors
WHERE factor_category = 'SYMBOLOGY_LINEAGE';

-- Grant access and set search_path
ALTER DATABASE halal_equity SET search_path TO "$user", public, ca;
ALTER ROLE halal_user SET search_path TO "$user", public, ca;
GRANT ALL ON SCHEMA ca TO halal_user;

-- 4. Reconcile TCS Dividend Factor to exact 8-decimal precision as Revision 3
-- Recomputed: (2269.3000 - 12.0000) / 2269.3000 = 0.9947120257... -> 0.99471203
INSERT INTO corporate_action_adjustment_factors (
    symbol_id, action_type, factor_category, event_id, ex_date, record_date, effective_date,
    dividend_amount, cash_dividend_per_share,
    adjustment_factor, nominal_price_factor, total_return_factor, volume_adjustment_factor,
    price_factor_applicable, cum_price_reference, ex_price_reference,
    source_reference, revision_number, is_verified, verification_status,
    dispute_status, dispute_reason, details
)
SELECT 
    s.id, 'CASH_DIVIDEND', 'CASH_DISTRIBUTION', 'DEFAULT', '2026-10-14', '2026-10-14', '2026-10-14',
    12.0000, 12.0000,
    0.99471203, 1.00000000, 0.99471203, 1.00000000,
    true, 2269.3000, 2304.8999,
    'NSE/CML/2026/DIV/TCS_INT_12', 3, true, 'VERIFIED_EXCHANGE_PRIMARY',
    'UNDISPUTED', NULL,
    '{"scheme": "Interim Dividend", "cum_price_reference": 2269.3000, "dividend_amount": 12.0000, "total_return_factor_exact": 0.9947120257348081, "total_return_factor_8dec": 0.99471203, "nominal_price_factor": 1.00000000, "volume_adjustment_factor": 1.00000000, "formula": "(P_cum - D) / P_cum", "resolution": "Revision 3 reconciles total return factor to exact 8-decimal precision based on primary exchange cum-close of 2269.3000 on 2026-10-13."}'::jsonb
FROM symbols s
WHERE s.symbol = 'TCS'
ON CONFLICT (symbol_id, ex_date, action_type, event_id, revision_number) DO NOTHING;
