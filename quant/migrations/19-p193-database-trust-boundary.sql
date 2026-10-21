-- HalalEquity Migration 19: P1.9.3 Database Trust-Boundary Hardening & Quarantine Bypass Elimination
--
-- Objective:
-- 1. Eliminate trust-boundary vulnerability where source string ('NSE_CORPORATE_ACTION' or 'PRIMARY_EXCHANGE')
--    was treated as proof of authorization.
-- 2. Enforce relational verification in fn_ohlcv_daily_quarantine_guard():
--    - An incoming adjusted_close != close (or un-quarantining of NULL) is permitted ONLY IF an active,
--      verified corporate action factor exists in corporate_action_adjustment_factors with supporting
--      VERIFIED_EXCHANGE_PRIMARY evidence in corporate_action_events.
--    - The supplied adjusted_close MUST match the verified mathematical factor formula ROUND(close * factor, 4).
--    - Forged source strings without verified evidence are strictly forced back to NULL / unadjusted close / vendor source.
-- 3. Provide ca.apply_verified_corporate_action() trusted database workflow procedure with SECURITY DEFINER.

BEGIN;

-- 1. Create or replace the trusted workflow procedure
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

    SELECT ev.event_id, ca.total_return_factor
    INTO v_verified_event_id, v_verified_factor
    FROM corporate_action_events ev
    JOIN corporate_action_adjustment_factors ca ON ca.event_id = ev.event_id AND ca.symbol_id = ev.symbol_id
    WHERE ev.symbol = p_symbol
      AND ev.event_id = p_event_id
      AND ev.verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
      AND ca.is_verified = TRUE
      AND ca.price_factor_applicable = TRUE
      AND ca.dispute_status = 'UNDISPUTED'
    ORDER BY ca.revision_number DESC
    LIMIT 1;

    IF v_verified_event_id IS NULL THEN
        RAISE EXCEPTION 'Corporate action event % for symbol % is not verified by primary exchange evidence', p_event_id, p_symbol;
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

-- 2. Upgrade the row-level quarantine and authorization guard trigger
CREATE OR REPLACE FUNCTION fn_ohlcv_daily_quarantine_guard()
RETURNS TRIGGER AS $$
DECLARE
    v_has_verified_ca BOOLEAN := FALSE;
    v_active_factor NUMERIC;
    v_expected_adj NUMERIC;
BEGIN
    -- Query corporate-action relational proof:
    -- An adjustment is verified ONLY IF an active, undisputed factor exists with verified primary exchange evidence
    SELECT true, ca.total_return_factor
    INTO v_has_verified_ca, v_active_factor
    FROM corporate_action_adjustment_factors ca
    JOIN corporate_action_events ev ON ev.event_id = ca.event_id 
                                   AND ev.symbol_id = ca.symbol_id
                                   AND ev.verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
    WHERE ca.symbol_id = NEW.symbol_id
      AND ca.is_verified = TRUE
      AND ca.price_factor_applicable = TRUE
      AND ca.dispute_status = 'UNDISPUTED'
      AND NEW.timestamp::date < ca.ex_date
      AND NEW.timestamp::date >= '2026-10-05'
    ORDER BY ca.revision_number DESC
    LIMIT 1;

    IF v_has_verified_ca IS NULL THEN
        v_has_verified_ca := FALSE;
    END IF;

    IF v_has_verified_ca THEN
        v_expected_adj := ROUND(NEW.close * v_active_factor, 4);
    END IF;

    -- Handle UPDATE operations
    IF TG_OP = 'UPDATE' THEN
        -- Case A: Existing row is quarantined NULL
        IF OLD.adjusted_close IS NULL THEN
            IF NEW.adjusted_close IS NOT NULL THEN
                -- Permitted ONLY IF verified corporate action evidence exists AND adjusted_close matches formula
                IF v_has_verified_ca 
                   AND NEW.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE')
                   AND ABS(NEW.adjusted_close - v_expected_adj) <= 0.0005 THEN
                    RETURN NEW;
                ELSE
                    -- Block bypass: forged source or unverified values forced strictly to NULL
                    NEW.adjusted_close := NULL;
                    NEW.source := OLD.source;
                END IF;
            END IF;
            RETURN NEW;
        END IF;

        -- Case B: Existing row is a verified corporate action adjustment
        IF OLD.adjusted_close IS NOT NULL AND OLD.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN
            -- Cannot be overwritten by vendor feeds or mismatched adjustments
            IF NOT (v_has_verified_ca 
                    AND NEW.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE')
                    AND ABS(NEW.adjusted_close - v_expected_adj) <= 0.0005) THEN
                NEW.adjusted_close := OLD.adjusted_close;
                NEW.source := OLD.source;
            END IF;
            RETURN NEW;
        END IF;

        -- Case C: Existing row is an unadjusted bar (adjusted_close == close)
        IF OLD.adjusted_close IS NOT NULL AND OLD.adjusted_close = OLD.close THEN
            -- Reject forged source string if no verified corporate action applies
            IF NEW.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') AND NOT v_has_verified_ca THEN
                NEW.source := 'yahoo_finance';
            END IF;

            -- Reject fabricated adjustment on unadjusted bar
            IF NEW.adjusted_close != NEW.close THEN
                IF v_has_verified_ca 
                   AND NEW.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE')
                   AND ABS(NEW.adjusted_close - v_expected_adj) <= 0.0005 THEN
                    RETURN NEW;
                ELSE
                    NEW.adjusted_close := NEW.close;
                    NEW.source := 'yahoo_finance';
                END IF;
            END IF;

            RETURN NEW;
        END IF;

        RETURN NEW;
    END IF;

    -- Handle INSERT operations
    IF TG_OP = 'INSERT' THEN
        IF NEW.adjusted_close IS NOT NULL AND NEW.adjusted_close != NEW.close THEN
            IF v_has_verified_ca 
               AND NEW.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE')
               AND ABS(NEW.adjusted_close - v_expected_adj) <= 0.0005 THEN
                RETURN NEW;
            ELSE
                -- Fail-closed quarantine on insert for unverified vendor adjustments
                NEW.adjusted_close := NULL;
                IF NEW.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN
                    NEW.source := 'yahoo_finance';
                END IF;
            END IF;
        END IF;

        IF NEW.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') AND NOT v_has_verified_ca THEN
            NEW.source := 'yahoo_finance';
        END IF;

        RETURN NEW;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 3. Ensure trigger is attached and active
DROP TRIGGER IF EXISTS trg_ohlcv_daily_quarantine_guard ON ohlcv_daily;
CREATE TRIGGER trg_ohlcv_daily_quarantine_guard
BEFORE INSERT OR UPDATE ON ohlcv_daily
FOR EACH ROW
EXECUTE FUNCTION fn_ohlcv_daily_quarantine_guard();

-- 4. Set explicit grants
GRANT USAGE ON SCHEMA ca TO PUBLIC;
GRANT EXECUTE ON FUNCTION ca.apply_verified_corporate_action(TEXT, DATE, DATE, NUMERIC, TEXT) TO PUBLIC;

COMMIT;
