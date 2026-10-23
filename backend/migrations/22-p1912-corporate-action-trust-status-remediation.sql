-- ============================================================================
-- Migration 22: Corporate-Action Trust Status Remediation, Audit Trail Logging,
--               and Adversarial Source Authenticity Verification Guard
-- Phase: P1.9.12 — Authorized Data Acquisition and Provenance Remediation
-- ============================================================================

-- 1. Create Auditable Verification History Table
-- Preserves complete history of all status demotions and remediations.
CREATE TABLE IF NOT EXISTS corporate_action_verification_audit_log (
    id SERIAL PRIMARY KEY,
    event_id VARCHAR(128) NOT NULL,
    symbol VARCHAR(32) NOT NULL,
    prior_verification_status VARCHAR(40) NOT NULL,
    new_verification_status VARCHAR(40) NOT NULL,
    prior_coverage_status VARCHAR(40),
    new_coverage_status VARCHAR(40),
    remediation_phase VARCHAR(32) NOT NULL DEFAULT 'P1.9.12',
    demotion_reason TEXT NOT NULL,
    source_artifact_id INT REFERENCES corporate_action_source_artifacts(id),
    artifact_source_url TEXT,
    artifact_checksum VARCHAR(64),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ca_audit_event_id ON corporate_action_verification_audit_log(event_id);
CREATE INDEX IF NOT EXISTS idx_ca_audit_symbol ON corporate_action_verification_audit_log(symbol);

-- Append-only guard for corporate_action_verification_audit_log
CREATE OR REPLACE FUNCTION ca_audit_log_guard()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'corporate_action_verification_audit_log is append-only: UPDATE and DELETE are strictly prohibited (fail-closed)';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ca_audit_log_guard ON corporate_action_verification_audit_log;
CREATE TRIGGER trg_ca_audit_log_guard
BEFORE UPDATE OR DELETE ON corporate_action_verification_audit_log
FOR EACH ROW EXECUTE FUNCTION ca_audit_log_guard();

-- 2. Audit and Record Status Demotions for all 58 events backed by CONTROLLED_ARCHIVE payloads
-- Captures prior state and rationale before demoting.
INSERT INTO corporate_action_verification_audit_log (
    event_id,
    symbol,
    prior_verification_status,
    new_verification_status,
    prior_coverage_status,
    new_coverage_status,
    remediation_phase,
    demotion_reason,
    source_artifact_id,
    artifact_source_url,
    artifact_checksum
)
SELECT 
    ev.event_id,
    ev.symbol,
    ev.verification_status,
    'ARCHIVE_UNAUTHENTICATED',
    ev.coverage_status,
    ev.coverage_status,
    'P1.9.12',
    'P1.9.12 Remediation: Backing artifact (' || art.source_url || ') is an unauthenticated local controlled archive lacking independently authenticated primary exchange circular or digital signature. Demoted from VERIFIED_EXCHANGE_PRIMARY to ARCHIVE_UNAUTHENTICATED to separate byte integrity from authentic origin evidence.',
    ev.source_artifact_id,
    art.source_url,
    art.sha256_checksum
FROM corporate_action_events ev
JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
WHERE ev.verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
  AND art.source_url LIKE 'CONTROLLED_ARCHIVE:%'
ON CONFLICT DO NOTHING;

-- 3. Demote Corporate Action Events from VERIFIED_EXCHANGE_PRIMARY to ARCHIVE_UNAUTHENTICATED
UPDATE corporate_action_events
SET verification_status = 'ARCHIVE_UNAUTHENTICATED',
    updated_at = NOW()
WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
  AND source_artifact_id IN (
      SELECT id FROM corporate_action_source_artifacts WHERE source_url LIKE 'CONTROLLED_ARCHIVE:%'
  );

-- 4. Harden fn_ca_event_verification_guard to prevent unauthenticated archives or vendor JSON from claiming VERIFIED_EXCHANGE_PRIMARY
CREATE OR REPLACE FUNCTION public.fn_ca_event_verification_guard()
RETURNS TRIGGER AS $$
DECLARE
    v_artifact_status TEXT;
    v_artifact_authority TEXT;
    v_artifact_checksum TEXT;
    v_artifact_url TEXT;
    v_artifact_doc_type TEXT;
BEGIN
    -- If event claims primary exchange verification, require authenticated primary artifact
    IF NEW.verification_status = 'VERIFIED_EXCHANGE_PRIMARY' OR NEW.coverage_status = 'HARVESTED_EXCHANGE_PRIMARY' THEN
        IF NEW.source_artifact_id IS NULL THEN
            RAISE EXCEPTION 'Event verification rejected: VERIFIED_EXCHANGE_PRIMARY requires source_artifact_id to be populated (event_id=%)', NEW.event_id;
        END IF;

        SELECT validation_status, issuing_authority, sha256_checksum, source_url, document_type
        INTO v_artifact_status, v_artifact_authority, v_artifact_checksum, v_artifact_url, v_artifact_doc_type
        FROM corporate_action_source_artifacts
        WHERE id = NEW.source_artifact_id;

        IF v_artifact_status IS NULL THEN
            RAISE EXCEPTION 'Event verification rejected: source_artifact_id % does not exist in corporate_action_source_artifacts', NEW.source_artifact_id;
        END IF;

        -- Adversarial Provenance Rule 1: Local archives, fixtures, or unauthenticated local caches CANNOT establish VERIFIED_EXCHANGE_PRIMARY
        IF v_artifact_url LIKE 'CONTROLLED_ARCHIVE:%' OR v_artifact_url LIKE 'FIXTURE:%' OR v_artifact_url LIKE 'LOCAL:%' THEN
            RAISE EXCEPTION 'Event verification rejected: VERIFIED_EXCHANGE_PRIMARY cannot be established from unauthenticated archive payload (%) (event_id=%)',
                v_artifact_url, NEW.event_id;
        END IF;

        -- Adversarial Provenance Rule 2: Document type must represent an authenticated exchange primary source
        IF v_artifact_doc_type NOT IN ('CIRCULAR_PDF', 'AUTHENTICATED_EXCHANGE_FEED', 'STATUTORY_FILING') THEN
            RAISE EXCEPTION 'Event verification rejected: document_type % lacks authenticated primary origin proof (must be CIRCULAR_PDF, AUTHENTICATED_EXCHANGE_FEED, or STATUTORY_FILING)',
                v_artifact_doc_type;
        END IF;

        -- Adversarial Provenance Rule 3: Validation status must be PRIMARY_VERIFIED
        IF v_artifact_status NOT IN ('PRIMARY_VERIFIED') THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % has status % (must be PRIMARY_VERIFIED)', 
                NEW.source_artifact_id, v_artifact_status;
        END IF;

        -- Adversarial Provenance Rule 4: Issuing authority must be recognized statutory exchange or regulatory body
        IF v_artifact_authority NOT IN ('NSE_INDIA', 'BSE_INDIA', 'NCLT_MUMBAI', 'MCA_INDIA') THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % issuing_authority % is not a recognized statutory primary authority', 
                NEW.source_artifact_id, v_artifact_authority;
        END IF;

        -- Adversarial Provenance Rule 5: Cryptographic SHA256 digest length check
        IF v_artifact_checksum IS NULL OR length(v_artifact_checksum) != 64 THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % lacks a valid SHA256 checksum', NEW.source_artifact_id;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 5. Grant permissions to halal_app_user
GRANT SELECT, INSERT ON corporate_action_verification_audit_log TO halal_app_user;
GRANT USAGE, SELECT ON SEQUENCE corporate_action_verification_audit_log_id_seq TO halal_app_user;
