-- HalalEquity Migration 20: P1.9.4 Database Privilege Hardening and Evidence Authorization Guard
--
-- Objective:
-- 1. Establish non-login procedure owner role (ca_security_admin) and non-superuser application role (halal_app_user).
-- 2. Revoke public execute on ca.apply_verified_corporate_action(); restrict to authorized roles.
-- 3. Add cryptographic and primary-source artifact validation in ca.apply_verified_corporate_action().
-- 4. Add verification guard trigger on corporate_action_events (trg_ca_event_verification_guard)
--    to prevent manufacturing of unverified 'VERIFIED_EXCHANGE_PRIMARY' events without authentic primary artifacts.
-- 5. Harden ca_factor_ingest_guard() on corporate_action_adjustment_factors to prevent
--    manufacturing of unverified factors claiming is_verified=true.
-- 6. Apply strict least-privilege grants to halal_app_user.

BEGIN;

-- 1. Create dedicated non-login role for procedure ownership
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ca_security_admin') THEN
        CREATE ROLE ca_security_admin WITH NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'halal_app_user') THEN
        CREATE ROLE halal_app_user WITH LOGIN PASSWORD 'app_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
    END IF;
END
$$;

-- 2. Create verification guard trigger on corporate_action_events
CREATE OR REPLACE FUNCTION fn_ca_event_verification_guard()
RETURNS TRIGGER AS $$
DECLARE
    v_artifact_status TEXT;
    v_artifact_authority TEXT;
    v_artifact_checksum TEXT;
BEGIN
    -- If event claims primary exchange verification, require authenticated primary artifact
    IF NEW.verification_status = 'VERIFIED_EXCHANGE_PRIMARY' OR NEW.coverage_status = 'HARVESTED_EXCHANGE_PRIMARY' THEN
        IF NEW.source_artifact_id IS NULL THEN
            RAISE EXCEPTION 'Event verification rejected: VERIFIED_EXCHANGE_PRIMARY requires source_artifact_id to be populated (event_id=%)', NEW.event_id;
        END IF;

        SELECT validation_status, issuing_authority, sha256_checksum
        INTO v_artifact_status, v_artifact_authority, v_artifact_checksum
        FROM corporate_action_source_artifacts
        WHERE id = NEW.source_artifact_id;

        IF v_artifact_status IS NULL THEN
            RAISE EXCEPTION 'Event verification rejected: source_artifact_id % does not exist in corporate_action_source_artifacts', NEW.source_artifact_id;
        END IF;

        IF v_artifact_status NOT IN ('PARSED_VALIDATED', 'PRIMARY_VERIFIED') THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % has invalid status % (must be PARSED_VALIDATED or PRIMARY_VERIFIED)', 
                NEW.source_artifact_id, v_artifact_status;
        END IF;

        IF v_artifact_authority NOT IN ('NSE_INDIA', 'BSE_INDIA') THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % issuing_authority % is not a recognized primary exchange', 
                NEW.source_artifact_id, v_artifact_authority;
        END IF;

        IF v_artifact_checksum IS NULL OR length(v_artifact_checksum) != 64 THEN
            RAISE EXCEPTION 'Event verification rejected: source artifact % lacks a valid SHA256 checksum', NEW.source_artifact_id;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ca_event_verification_guard ON corporate_action_events;
CREATE TRIGGER trg_ca_event_verification_guard
BEFORE INSERT OR UPDATE ON corporate_action_events
FOR EACH ROW
EXECUTE FUNCTION fn_ca_event_verification_guard();

-- 3. Harden ca_factor_ingest_guard on corporate_action_adjustment_factors
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

    -- If factor claims is_verified=true or price_factor_applicable=true, verify underlying event and primary artifact proof
    IF NEW.is_verified = TRUE OR NEW.price_factor_applicable = TRUE THEN
        SELECT ev.verification_status, (art.id IS NOT NULL AND art.sha256_checksum IS NOT NULL AND art.validation_status IN ('PARSED_VALIDATED', 'PRIMARY_VERIFIED'))
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

-- 4. Rebuild ca.apply_verified_corporate_action with cryptographic artifact verification
CREATE OR REPLACE FUNCTION ca.apply_verified_corporate_action(
    p_symbol TEXT,
    p_start_date DATE,
    p_ex_date DATE,
    p_factor NUMERIC,
    p_event_id TEXT
)
RETURNS BIGINT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public, ca, pg_temp
AS $$
DECLARE
    v_symbol_id INT;
    v_verified_event_id TEXT;
    v_verified_factor NUMERIC;
    v_artifact_id INT;
    v_artifact_checksum TEXT;
    v_rows_updated BIGINT := 0;
BEGIN
    SELECT id INTO v_symbol_id FROM symbols WHERE symbol = p_symbol;
    IF v_symbol_id IS NULL THEN
        RAISE EXCEPTION 'Symbol % not found', p_symbol;
    END IF;

    IF p_factor <= 0 THEN
        RAISE EXCEPTION 'Adjustment factor must be positive, got %', p_factor;
    END IF;

    IF p_start_date >= p_ex_date THEN
        RAISE EXCEPTION 'Start date % must be strictly before ex-date %', p_start_date, p_ex_date;
    END IF;

    -- Complete chain of custody verification:
    -- Symbol -> Event -> Primary Artifact -> Valid Factor
    SELECT ev.event_id, ca.total_return_factor, art.id, art.sha256_checksum
    INTO v_verified_event_id, v_verified_factor, v_artifact_id, v_artifact_checksum
    FROM corporate_action_events ev
    JOIN corporate_action_adjustment_factors ca ON ca.event_id = ev.event_id AND ca.symbol_id = ev.symbol_id
    JOIN corporate_action_source_artifacts art ON art.id = ev.source_artifact_id
    WHERE ev.symbol = p_symbol
      AND ev.event_id = p_event_id
      AND ev.verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
      AND art.issuing_authority IN ('NSE_INDIA', 'BSE_INDIA')
      AND art.validation_status IN ('PARSED_VALIDATED', 'PRIMARY_VERIFIED')
      AND art.sha256_checksum IS NOT NULL
      AND length(art.sha256_checksum) = 64
      AND ca.is_verified = TRUE
      AND ca.price_factor_applicable = TRUE
      AND ca.dispute_status = 'UNDISPUTED'
    ORDER BY ca.revision_number DESC
    LIMIT 1;

    IF v_verified_event_id IS NULL THEN
        RAISE EXCEPTION 'Corporate action event % for symbol % is not verified by validated primary exchange evidence', p_event_id, p_symbol;
    END IF;

    IF ABS(p_factor - v_verified_factor) > 0.0000001 THEN
        RAISE EXCEPTION 'Supplied factor % does not match verified factor % for event %', p_factor, v_verified_factor, p_event_id;
    END IF;

    UPDATE ohlcv_daily
    SET adjusted_close = ROUND(close * p_factor, 4),
        source = 'NSE_CORPORATE_ACTION',
        retrieved_at = NOW()
    WHERE symbol_id = v_symbol_id
      AND timestamp::date >= p_start_date
      AND timestamp::date < p_ex_date;

    GET DIAGNOSTICS v_rows_updated = ROW_COUNT;
    RETURN v_rows_updated;
END;
$$;

-- 5. Set procedure owner to non-login security admin role
ALTER FUNCTION ca.apply_verified_corporate_action(TEXT, DATE, DATE, NUMERIC, TEXT) OWNER TO ca_security_admin;

-- 6. Configure explicit least-privilege grants for ca_security_admin
GRANT USAGE ON SCHEMA public, ca TO ca_security_admin;
GRANT SELECT, UPDATE ON ohlcv_daily TO ca_security_admin;
GRANT SELECT ON symbols, corporate_action_events, corporate_action_adjustment_factors, corporate_action_source_artifacts TO ca_security_admin;

-- 7. Revoke execution from PUBLIC; grant only to authorized execution roles
REVOKE ALL ON FUNCTION ca.apply_verified_corporate_action(TEXT, DATE, DATE, NUMERIC, TEXT) FROM PUBLIC;
REVOKE ALL ON SCHEMA ca FROM PUBLIC;
GRANT USAGE ON SCHEMA ca TO halal_user, halal_app_user, ca_security_admin;
GRANT EXECUTE ON FUNCTION ca.apply_verified_corporate_action(TEXT, DATE, DATE, NUMERIC, TEXT) TO halal_user, halal_app_user;

-- 8. Configure application least-privilege grants for halal_app_user
GRANT USAGE ON SCHEMA public TO halal_app_user;
GRANT SELECT, INSERT, UPDATE ON ohlcv_daily TO halal_app_user;
GRANT SELECT ON symbols, corporate_action_events, corporate_action_adjustment_factors, corporate_action_source_artifacts TO halal_app_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO halal_app_user;

COMMIT;
