-- HalalEquity Migration 25: P1.9.17 Statutory Evidence Acquisition, Authentication, and Point-in-Time Data Readiness
--
-- Objectives:
-- 1. Extend corporate_action_source_artifacts and corporate_action_events verification states
--    to include: ACQUIRED_UNVERIFIED, PARSED_VALIDATED, AUTHENTICITY_PENDING, PRIMARY_VERIFIED, REJECTED, AMBIGUOUS.
-- 2. Create financial_disclosure_source_artifacts for immutable storage of official audited financial reports.
-- 3. Create audited_footnote_disclosures for storing page-referenced, extracted non-operating interest income footnotes.
-- 4. Create corporate_action_reconciliation_reviews for auditing discrepancies between official exchange documents and existing records.
-- 5. Enforce strict database triggers and privilege boundaries preventing halal_app_user from forging evidence.

BEGIN;

-- 1. Extend corporate_action_source_artifacts status constraints
ALTER TABLE corporate_action_source_artifacts DROP CONSTRAINT IF EXISTS ck_ca_artifact_validation_status;
ALTER TABLE corporate_action_source_artifacts ADD CONSTRAINT ck_ca_artifact_validation_status 
    CHECK (validation_status IN (
        'ACQUIRED_UNVERIFIED',
        'PARSED_VALIDATED',
        'AUTHENTICITY_PENDING',
        'PRIMARY_VERIFIED',
        'REJECTED',
        'AMBIGUOUS',
        'SUPERSEDED',
        'RAW_RETRIEVED',
        'HASH_VERIFIED',
        'VENDOR_CONFIRMED'
    ));

-- Extend corporate_action_events verification status constraint
ALTER TABLE corporate_action_events DROP CONSTRAINT IF EXISTS ck_ca_event_verification_status;
ALTER TABLE corporate_action_events ADD CONSTRAINT ck_ca_event_verification_status 
    CHECK (verification_status IN (
        'SOURCE_MISSING',
        'ARCHIVE_UNAUTHENTICATED',
        'HASH_VERIFIED',
        'VENDOR_CONFIRMED',
        'AUTHENTICITY_PENDING',
        'PRIMARY_SOURCE_AUTHENTICATED',
        'VERIFIED_EXCHANGE_PRIMARY',
        'AMBIGUOUS',
        'REJECTED',
        'UNVERIFIED'
    ));

-- 2. Table: financial_disclosure_source_artifacts (immutable statutory annual reports / filings)
CREATE TABLE IF NOT EXISTS financial_disclosure_source_artifacts (
    id SERIAL PRIMARY KEY,
    artifact_id VARCHAR(64) UNIQUE NOT NULL,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    symbol VARCHAR(32) NOT NULL,
    fiscal_year INT NOT NULL,
    period_type VARCHAR(16) NOT NULL DEFAULT 'ANNUAL', -- 'ANNUAL', 'QUARTERLY'
    period_end DATE NOT NULL,
    document_title TEXT NOT NULL,
    issuing_authority VARCHAR(64) NOT NULL, -- 'BSE_INDIA', 'NSE_INDIA', 'COMPANY_ANNUAL_REPORT', 'MCA_INDIA'
    source_url TEXT NOT NULL,
    http_status INT,
    byte_size BIGINT,
    content_type VARCHAR(64),
    final_redirect_url TEXT,
    raw_payload TEXT,
    sha256_checksum VARCHAR(64) NOT NULL,
    retrieval_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    auditor_firm TEXT,
    auditor_opinion TEXT DEFAULT 'UNMODIFIED', -- 'UNMODIFIED', 'QUALIFIED', 'ADVERSE', 'DISCLAIMER'
    validation_status VARCHAR(32) NOT NULL DEFAULT 'ACQUIRED_UNVERIFIED',
    review_status VARCHAR(32) NOT NULL DEFAULT 'PENDING_REVIEW', -- 'PENDING_REVIEW', 'REVIEWED_CONFIRMED', 'REJECTED'
    reviewer TEXT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Constraint on financial_disclosure_source_artifacts validation_status
ALTER TABLE financial_disclosure_source_artifacts DROP CONSTRAINT IF EXISTS ck_fin_artifact_validation_status;
ALTER TABLE financial_disclosure_source_artifacts ADD CONSTRAINT ck_fin_artifact_validation_status 
    CHECK (validation_status IN (
        'ACQUIRED_UNVERIFIED',
        'PARSED_VALIDATED',
        'AUTHENTICITY_PENDING',
        'PRIMARY_VERIFIED',
        'REJECTED',
        'AMBIGUOUS'
    ));

-- 3. Table: audited_footnote_disclosures (audited Note 24/25 etc. with exact page and extracted passage)
CREATE TABLE IF NOT EXISTS audited_footnote_disclosures (
    id SERIAL PRIMARY KEY,
    artifact_id INT NOT NULL REFERENCES financial_disclosure_source_artifacts(id),
    symbol_id INT NOT NULL REFERENCES symbols(id),
    symbol VARCHAR(32) NOT NULL,
    period_end DATE NOT NULL,
    disclosure_type VARCHAR(64) NOT NULL, -- 'INTEREST_INCOME', 'TOTAL_RECEIVABLES', 'DEBT_BREAKDOWN', 'CASH_AND_BANK'
    page_number INT,
    note_reference VARCHAR(64) NOT NULL, -- e.g. 'Note 24 - Finance Income', 'Note 28 - Other Income'
    extracted_passage TEXT NOT NULL,
    extracted_amount NUMERIC(24, 4) NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'INR',
    unit VARCHAR(16) NOT NULL DEFAULT 'CRORES', -- 'CRORES', 'LAKHS', 'RAW_INR', 'MILLIONS'
    amount_in_raw_inr NUMERIC(24, 4) NOT NULL,
    reporting_period_months INT NOT NULL DEFAULT 12,
    parser_name VARCHAR(64) NOT NULL,
    parser_version VARCHAR(32) NOT NULL,
    parser_confidence NUMERIC(5, 4) NOT NULL, -- 0.0000 to 1.0000
    manual_review_status VARCHAR(32) NOT NULL DEFAULT 'PENDING_QUALIFIED_REVIEW', -- 'PENDING_QUALIFIED_REVIEW', 'CERTIFIED_BY_REVIEWER', 'REJECTED'
    shariah_methodology_version VARCHAR(64) NOT NULL DEFAULT 'AAOIFI_STANDARD_NO_21_2026',
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 4. Table: corporate_action_reconciliation_reviews (discrepancy tracking)
CREATE TABLE IF NOT EXISTS corporate_action_reconciliation_reviews (
    id SERIAL PRIMARY KEY,
    event_id VARCHAR(64) NOT NULL,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    symbol VARCHAR(32) NOT NULL,
    source_artifact_id INT REFERENCES corporate_action_source_artifacts(id),
    official_document_ref TEXT,
    discrepancy_field VARCHAR(64) NOT NULL, -- 'DIVIDEND_AMOUNT', 'EX_DATE', 'RECORD_DATE', 'RATIO', 'ACTION_TYPE'
    existing_claim_value TEXT NOT NULL,
    official_claim_value TEXT NOT NULL,
    discrepancy_reason TEXT NOT NULL,
    review_status VARCHAR(32) NOT NULL DEFAULT 'PENDING_REVIEW', -- 'PENDING_REVIEW', 'RESOLVED_OFFICIAL_ADOPTED', 'REJECTED_CLAIM', 'AMBIGUOUS_RETAINED'
    reviewer TEXT,
    resolution_notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 5. Trigger Guards for financial_disclosure_source_artifacts (append-only & privilege boundary)
CREATE OR REPLACE FUNCTION fn_fin_artifact_guard()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        RAISE EXCEPTION 'financial_disclosure_source_artifacts is append-only: UPDATE and DELETE are prohibited (fail-closed)';
    END IF;

    IF TG_OP = 'INSERT' AND NEW.validation_status = 'PRIMARY_VERIFIED' AND CURRENT_USER = 'halal_app_user' THEN
        RAISE EXCEPTION 'Privilege boundary violation: application role halal_app_user cannot manufacture PRIMARY_VERIFIED financial disclosure artifacts';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fin_artifact_guard ON financial_disclosure_source_artifacts;
CREATE TRIGGER trg_fin_artifact_guard
BEFORE INSERT OR UPDATE OR DELETE ON financial_disclosure_source_artifacts
FOR EACH ROW EXECUTE FUNCTION fn_fin_artifact_guard();

-- 6. Trigger Guards for audited_footnote_disclosures
CREATE OR REPLACE FUNCTION fn_footnote_disclosure_guard()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' AND (NEW.is_verified = TRUE OR NEW.manual_review_status = 'CERTIFIED_BY_REVIEWER') AND CURRENT_USER = 'halal_app_user' THEN
        RAISE EXCEPTION 'Privilege boundary violation: application role halal_app_user cannot certify footnote disclosures';
    END IF;

    -- Negative numbers not permitted for extracted amounts
    IF NEW.extracted_amount < 0 OR NEW.amount_in_raw_inr < 0 THEN
        RAISE EXCEPTION 'Footnote disclosure invalid: extracted amount cannot be negative (got %)', NEW.extracted_amount;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_footnote_disclosure_guard ON audited_footnote_disclosures;
CREATE TRIGGER trg_footnote_disclosure_guard
BEFORE INSERT OR UPDATE ON audited_footnote_disclosures
FOR EACH ROW EXECUTE FUNCTION fn_footnote_disclosure_guard();

-- 7. Permissions and Grants
GRANT SELECT, INSERT ON financial_disclosure_source_artifacts TO halal_app_user;
GRANT USAGE, SELECT ON SEQUENCE financial_disclosure_source_artifacts_id_seq TO halal_app_user;

GRANT SELECT, INSERT ON audited_footnote_disclosures TO halal_app_user;
GRANT USAGE, SELECT ON SEQUENCE audited_footnote_disclosures_id_seq TO halal_app_user;

GRANT SELECT, INSERT, UPDATE ON corporate_action_reconciliation_reviews TO halal_app_user;
GRANT USAGE, SELECT ON SEQUENCE corporate_action_reconciliation_reviews_id_seq TO halal_app_user;

COMMIT;
