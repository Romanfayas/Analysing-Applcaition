-- ============================================================================
-- Migration 15: Corporate Action Source Provenance, Artifact Immutability,
--              Event Normalization, and Nominal vs Total-Return View Isolation
-- ============================================================================

-- 1. Table: corporate_action_source_artifacts
-- Stores immutable primary source documents (API payloads, circulars, filings)
-- with issuing authority, retrieval metadata, and SHA-256 cryptographic checksums.
CREATE TABLE IF NOT EXISTS corporate_action_source_artifacts (
    id SERIAL PRIMARY KEY,
    artifact_id VARCHAR(128) NOT NULL UNIQUE,
    issuing_authority VARCHAR(64) NOT NULL,          -- e.g. 'NSE_INDIA', 'BSE_INDIA', 'NCLT', 'COMPANY_FILING'
    source_url TEXT NOT NULL,
    document_identifier VARCHAR(128) NOT NULL,       -- e.g. 'CORP_ACTION_TCS_20261016' or circular number
    document_type VARCHAR(64) NOT NULL,              -- e.g. 'EXCHANGE_API_JSON', 'CIRCULAR_PDF', 'COMPANY_ANNOUNCEMENT'
    publication_timestamp TIMESTAMPTZ,
    retrieval_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    raw_payload TEXT NOT NULL,
    sha256_checksum VARCHAR(64) NOT NULL,
    parser_version VARCHAR(32) NOT NULL DEFAULT 'v1.8.11',
    validation_status VARCHAR(40) NOT NULL DEFAULT 'RAW_RETRIEVED', -- 'RAW_RETRIEVED', 'PARSED_VALIDATED', 'PARSE_ERROR', 'MANUAL_REVIEW_REQUIRED'
    reviewer VARCHAR(64),
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ca_artifacts_checksum ON corporate_action_source_artifacts(sha256_checksum);
CREATE INDEX IF NOT EXISTS idx_ca_artifacts_doc_id ON corporate_action_source_artifacts(document_identifier);

-- Append-only trigger guard for corporate_action_source_artifacts
CREATE OR REPLACE FUNCTION ca_artifact_guard()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'corporate_action_source_artifacts is append-only: UPDATE and DELETE are strictly prohibited (fail-closed)';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ca_artifact_guard ON corporate_action_source_artifacts;
CREATE TRIGGER trg_ca_artifact_guard
BEFORE UPDATE OR DELETE ON corporate_action_source_artifacts
FOR EACH ROW EXECUTE FUNCTION ca_artifact_guard();

-- 2. Table: corporate_action_events
-- Normalized corporate action facts extracted from primary source artifacts.
-- Separates declared source facts from quantitative price adjustment factors.
CREATE TABLE IF NOT EXISTS corporate_action_events (
    id SERIAL PRIMARY KEY,
    event_id VARCHAR(128) NOT NULL UNIQUE,           -- Deterministic: <AUTHORITY>:<SYMBOL>:<EX_DATE>:<ACTION_TYPE>:<INDEX>
    symbol_id INT NOT NULL REFERENCES symbols(id),
    symbol VARCHAR(32) NOT NULL,
    source_artifact_id INT REFERENCES corporate_action_source_artifacts(id),
    action_type VARCHAR(32) NOT NULL,                -- 'CASH_DIVIDEND', 'STOCK_SPLIT', 'BONUS_ISSUE', 'RIGHTS_ISSUE', 'DEMERGER', 'SYMBOL_CHANGE'
    source_subject TEXT NOT NULL,
    announcement_date DATE,
    ex_date DATE NOT NULL,
    record_date DATE,
    effective_date DATE,
    payment_date DATE,
    currency VARCHAR(10) NOT NULL DEFAULT 'INR',
    dividend_amount NUMERIC(18, 4),
    split_ratio_old NUMERIC(18, 6),
    split_ratio_new NUMERIC(18, 6),
    bonus_ratio_bonus NUMERIC(18, 6),
    bonus_ratio_held NUMERIC(18, 6),
    rights_subscription_price NUMERIC(18, 4),
    rights_ratio_new NUMERIC(18, 6),
    rights_ratio_old NUMERIC(18, 6),
    tax_cost_allocation_pct NUMERIC(10, 4),
    verification_status VARCHAR(40) NOT NULL DEFAULT 'UNVERIFIED', -- 'VERIFIED_EXCHANGE_PRIMARY', 'UNVERIFIED', 'AMBIGUOUS', 'DISPUTED'
    coverage_status VARCHAR(40) NOT NULL DEFAULT 'PARSED_EVENT',   -- 'PARSED_EVENT', 'NO_EVENT_CONFIRMED', 'SECONDARY_EVIDENCE_REQUIRED'
    details JSONB,
    raw_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ca_event_symbol_exdate_action UNIQUE (symbol_id, ex_date, action_type, event_id)
);

CREATE INDEX IF NOT EXISTS idx_ca_events_symbol_exdate ON corporate_action_events(symbol_id, ex_date DESC);
CREATE INDEX IF NOT EXISTS idx_ca_events_status ON corporate_action_events(verification_status, coverage_status);

-- 3. Dedicated ca Schema View Hardening:
-- Separate views for Nominal-Price Consumers vs Total-Return Consumers to prevent cross-contamination.

-- View: Active Nominal Price Factors (for technical indicators, breakout levels, price returns)
-- Cash dividends have nominal_price_factor = 1.0, and are STRICTLY EXCLUDED from nominal price adjustments.
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

-- View: Active Total Return Factors (for Total Return Index, dividend-adjusted performance)
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

-- Permissions
GRANT ALL ON TABLE corporate_action_source_artifacts TO halal_user;
GRANT ALL ON TABLE corporate_action_events TO halal_user;
GRANT ALL ON SEQUENCE corporate_action_source_artifacts_id_seq TO halal_user;
GRANT ALL ON SEQUENCE corporate_action_events_id_seq TO halal_user;
GRANT ALL ON ca.v_active_nominal_price_factors TO halal_user;
GRANT ALL ON ca.v_active_total_return_factors TO halal_user;
