-- HalalEquity Migration 23: P1.9.13 Independent Evidence Authentication and Privilege Boundary Hardening
--
-- Objective:
-- 1. Enforce strict trigger-level privilege boundaries preventing application role (halal_app_user)
--    from manufacturing primary verified artifacts or promoting events to VERIFIED_EXCHANGE_PRIMARY.
-- 2. Harden ca_artifact_guard() to ensure corporate_action_source_artifacts is append-only
--    and reject any promotion to PRIMARY_VERIFIED by halal_app_user.
-- 3. Harden fn_ca_event_verification_guard() so that neither halal_app_user nor unauthenticated URL/metadata
--    tampering can obtain VERIFIED_EXCHANGE_PRIMARY status without independently verified primary exchange evidence.
-- 4. Harden ca_factor_ingest_guard() to prevent halal_app_user from inserting verified adjustment factors.

BEGIN;

-- 1. Ensure table grants exist so that triggers intercept and enforce security policies
GRANT SELECT, INSERT, UPDATE, DELETE ON corporate_action_source_artifacts TO halal_app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON corporate_action_events TO halal_app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON corporate_action_adjustment_factors TO halal_app_user;
GRANT SELECT, INSERT ON corporate_action_verification_audit_log TO halal_app_user;

-- 2. Harden ca_artifact_guard to enforce privilege boundary and immutability
CREATE OR REPLACE FUNCTION ca_artifact_guard()
RETURNS TRIGGER AS $$
BEGIN
    -- Block UPDATE and DELETE on artifacts (immutable append-only)
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        RAISE EXCEPTION 'corporate_action_source_artifacts is append-only: UPDATE and DELETE are strictly prohibited (fail-closed)';
    END IF;

    -- Block application role from manufacturing PRIMARY_VERIFIED artifacts
    IF TG_OP = 'INSERT' AND NEW.validation_status = 'PRIMARY_VERIFIED' AND CURRENT_USER = 'halal_app_user' THEN
        RAISE EXCEPTION 'Privilege boundary violation: application role halal_app_user cannot manufacture PRIMARY_VERIFIED artifacts';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply ca_artifact_guard to BEFORE INSERT OR UPDATE OR DELETE
DROP TRIGGER IF EXISTS trg_ca_artifact_guard ON corporate_action_source_artifacts;
CREATE TRIGGER trg_ca_artifact_guard
BEFORE INSERT OR UPDATE OR DELETE ON corporate_action_source_artifacts
FOR EACH ROW
EXECUTE FUNCTION ca_artifact_guard();

-- 3. Harden fn_ca_event_verification_guard to prevent metadata-only verification forging
CREATE OR REPLACE FUNCTION fn_ca_event_verification_guard()
RETURNS TRIGGER AS $$
DECLARE
    v_artifact_status TEXT;
    v_artifact_authority TEXT;
    v_artifact_checksum TEXT;
    v_artifact_url TEXT;
    v_artifact_doc_type TEXT;
BEGIN
    -- If event claims primary exchange verification or harvested primary coverage
    IF NEW.verification_status = 'VERIFIED_EXCHANGE_PRIMARY' OR NEW.coverage_status = 'HARVESTED_EXCHANGE_PRIMARY' THEN
        -- Must have source artifact
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

        -- Adversarial Rule 1: Local archives, fixtures, or unauthenticated local caches CANNOT establish VERIFIED_EXCHANGE_PRIMARY
        IF v_artifact_url LIKE 'CONTROLLED_ARCHIVE:%' OR v_artifact_url LIKE 'FIXTURE:%' OR v_artifact_url LIKE 'LOCAL:%' THEN
            RAISE EXCEPTION 'Event verification rejected: VERIFIED_EXCHANGE_PRIMARY cannot be established from unauthenticated archive payload (%) (event_id=%)',
                v_artifact_url, NEW.event_id;
        END IF;

        -- Adversarial Rule 2: Application role can NEVER assign VERIFIED_EXCHANGE_PRIMARY
        IF CURRENT_USER = 'halal_app_user' THEN
            RAISE EXCEPTION 'Event verification rejected: application role halal_app_user cannot assign VERIFIED_EXCHANGE_PRIMARY (privilege boundary violation)';
        END IF;

        -- Adversarial Rule 3: Document type must represent an authenticated exchange primary source
        IF v_artifact_doc_type NOT IN ('CIRCULAR_PDF', 'AUTHENTICATED_EXCHANGE_FEED', 'STATUTORY_FILING') THEN
            RAISE EXCEPTION 'Event verification rejected: document_type % lacks authenticated primary origin proof (must be CIRCULAR_PDF, AUTHENTICATED_EXCHANGE_FEED, or STATUTORY_FILING)',
                v_artifact_doc_type;
        END IF;

        -- Adversarial Rule 4: Validation status must be PRIMARY_VERIFIED
        IF v_artifact_status NOT IN ('PRIMARY_VERIFIED') THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % has status % (must be PRIMARY_VERIFIED)', 
                NEW.source_artifact_id, v_artifact_status;
        END IF;

        -- Adversarial Rule 5: Issuing authority must be recognized statutory exchange or regulatory body
        IF v_artifact_authority NOT IN ('NSE_INDIA', 'BSE_INDIA', 'NCLT_MUMBAI', 'MCA_INDIA') THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % issuing_authority % is not a recognized statutory primary authority', 
                NEW.source_artifact_id, v_artifact_authority;
        END IF;

        -- Adversarial Rule 6: Cryptographic SHA256 digest length check
        IF v_artifact_checksum IS NULL OR length(v_artifact_checksum) != 64 THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % lacks a valid SHA256 checksum', NEW.source_artifact_id;
        END IF;

        -- Adversarial Rule 7: URL alone is insufficient - metadata tampering prevention
        -- Valid exchange domains include nseindia.com, nsearchives.nseindia.com, bseindia.com
        IF v_artifact_url NOT SIMILAR TO 'https?://([a-zA-Z0-9_\-]+\.)?(nseindia\.com|bseindia\.com)/%' THEN
            RAISE EXCEPTION 'Event verification rejected: source_url % does not match authenticated exchange primary endpoint pattern', v_artifact_url;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 4. Harden ca_factor_ingest_guard to prohibit halal_app_user from inserting verified factors
CREATE OR REPLACE FUNCTION ca_factor_ingest_guard()
RETURNS TRIGGER AS $$
DECLARE
    v_event_status TEXT;
    v_has_valid_artifact BOOLEAN := FALSE;
BEGIN
    -- Valid deterministic event_id is required
    IF NEW.event_id IS NULL OR NEW.event_id = 'DEFAULT' OR TRIM(NEW.event_id) = '' THEN
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

    -- Application role can NEVER insert verified factors
    IF (NEW.is_verified = TRUE OR NEW.price_factor_applicable = TRUE) AND CURRENT_USER = 'halal_app_user' THEN
        RAISE EXCEPTION 'Ingestion rejected: application role halal_app_user cannot insert verified adjustment factors (privilege boundary violation)';
    END IF;

    -- If factor claims is_verified=true or price_factor_applicable=true, verify underlying event and primary artifact proof
    IF NEW.is_verified = TRUE OR NEW.price_factor_applicable = TRUE THEN
        SELECT ev.verification_status, (art.id IS NOT NULL AND art.sha256_checksum IS NOT NULL AND art.validation_status IN ('PRIMARY_VERIFIED'))
        INTO v_event_status, v_has_valid_artifact
        FROM corporate_action_events ev
        LEFT JOIN corporate_action_source_artifacts art ON art.id = ev.source_artifact_id
        WHERE ev.event_id = NEW.event_id AND ev.symbol_id = NEW.symbol_id;

        IF v_event_status IS NULL THEN
            RAISE EXCEPTION 'Ingestion rejected: event_id % for symbol_id % does not exist in corporate_action_events', NEW.event_id, NEW.symbol_id;
        END IF;

        IF v_event_status <> 'VERIFIED_EXCHANGE_PRIMARY' THEN
            RAISE EXCEPTION 'Ingestion rejected: underlying corporate_action_event % has status %, not VERIFIED_EXCHANGE_PRIMARY', NEW.event_id, v_event_status;
        END IF;

        IF NOT v_has_valid_artifact THEN
            RAISE EXCEPTION 'Ingestion rejected: event % lacks validated primary exchange source artifact in registry', NEW.event_id;
        END IF;
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

COMMIT;
