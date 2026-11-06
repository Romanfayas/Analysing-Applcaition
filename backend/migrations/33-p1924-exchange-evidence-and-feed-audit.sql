-- HalalEquity Migration 33: P1.9.24 authenticated exchange evidence, reconciliation and feed audit
--
-- Adds EVIDENCE only. Nothing here modifies ohlcv_daily, corporate_action_events or any eligibility state.
--  * exchange_source_files / exchange_daily_bars : NSE CM bhavcopy files (the exchange's own end-of-day record),
--    fetched over verified TLS from nsearchives.nseindia.com, with SHA-256 and TLS certificate fingerprint.
--    These record provenance and byte integrity only; they are NOT proof of authenticity.
--  * ca_reconciliation_runs / ca_reconciliation_findings : results of comparing canonical corporate-action events
--    with NSE's corporate-actions feed. Findings are advisory; promotion stays a human operator decision.
--  * ohlcv_intraday.first_received_at : first receipt time of each bar (retrieved_at is overwritten on re-poll).
-- All evidence tables are append-only and writable only by an operator role; the runtime role may only read.

BEGIN;

CREATE TABLE IF NOT EXISTS exchange_source_files (
    id                 BIGSERIAL PRIMARY KEY,
    trade_date         DATE        NOT NULL UNIQUE,
    document_type      TEXT        NOT NULL CHECK (document_type IN ('NSE_CM_BHAVCOPY_UDIFF', 'NSE_CM_BHAVCOPY_LEGACY')),
    issuing_authority  TEXT        NOT NULL CHECK (issuing_authority = 'NSE_INDIA'),
    source_url         TEXT        NOT NULL UNIQUE CHECK (source_url ~ '^https://nsearchives\.nseindia\.com/'),
    http_status        INTEGER     NOT NULL CHECK (http_status = 200),
    byte_size          INTEGER     NOT NULL CHECK (byte_size > 0),
    sha256             TEXT        NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    tls_peer_subject   TEXT        NOT NULL,
    tls_peer_issuer    TEXT        NOT NULL,
    tls_cert_sha256    TEXT        NOT NULL CHECK (tls_cert_sha256 ~ '^[0-9a-f]{64}$'),
    retrieved_at       TIMESTAMPTZ NOT NULL,
    rows_total         INTEGER     NOT NULL,
    rows_loaded        INTEGER     NOT NULL,
    parser_version     TEXT        NOT NULL,
    loaded_by          TEXT        NOT NULL DEFAULT current_user,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS exchange_daily_bars (
    symbol_id      INTEGER     NOT NULL REFERENCES symbols(id),
    trade_date     DATE        NOT NULL,
    ticker         TEXT        NOT NULL,
    isin           TEXT        NOT NULL,
    series         TEXT        NOT NULL,
    open           NUMERIC(18,4) NOT NULL CHECK (open > 0),
    high           NUMERIC(18,4) NOT NULL,
    low            NUMERIC(18,4) NOT NULL CHECK (low > 0),
    close          NUMERIC(18,4) NOT NULL CHECK (close > 0),
    prev_close     NUMERIC(18,4),
    volume         BIGINT      NOT NULL CHECK (volume >= 0),
    turnover_inr   NUMERIC(24,2),
    trades         BIGINT,
    source_file_id BIGINT      NOT NULL REFERENCES exchange_source_files(id),
    PRIMARY KEY (symbol_id, trade_date),
    CHECK (high >= low AND high >= open AND high >= close AND low <= open AND low <= close)
);

CREATE TABLE IF NOT EXISTS ca_reconciliation_runs (
    id            BIGSERIAL PRIMARY KEY,
    run_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    window_from   DATE        NOT NULL,
    window_to     DATE        NOT NULL,
    summary       JSONB       NOT NULL,
    run_by        TEXT        NOT NULL DEFAULT current_user
);

CREATE TABLE IF NOT EXISTS ca_reconciliation_findings (
    id                    BIGSERIAL PRIMARY KEY,
    run_id                BIGINT      NOT NULL REFERENCES ca_reconciliation_runs(id),
    symbol_id             INTEGER     NOT NULL REFERENCES symbols(id),
    finding_type          TEXT        NOT NULL CHECK (finding_type IN (
                              'CANONICAL_CONFIRMED_BY_FEED',      -- feed lists the same action/ex-date (still needs operator review)
                              'CANONICAL_NOT_IN_FEED',            -- canonical event the exchange feed does not list
                              'FEED_ACTION_MISSING_FROM_CANONICAL', -- exchange lists an action the platform never recorded
                              'PARSER_FRAGMENT',                  -- canonical "event" that is a fragment of another event's text
                              'FEED_UNAVAILABLE')),
    action_class          TEXT,
    ex_date               DATE,
    feed_subject          TEXT,
    canonical_event_id    TEXT,
    artifact_id           INTEGER     REFERENCES corporate_action_source_artifacts(id),
    implied_price_factor  NUMERIC(14,8),
    observed_vendor_ratio NUMERIC(14,8),
    detail                JSONB       NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_ca_findings_run ON ca_reconciliation_findings(run_id, finding_type);

CREATE OR REPLACE FUNCTION fn_evidence_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only evidence: % is prohibited', TG_TABLE_NAME, TG_OP;
END $$;

DO $$
DECLARE t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['exchange_source_files', 'exchange_daily_bars', 'ca_reconciliation_runs', 'ca_reconciliation_findings'] LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS trg_%1$s_append_only ON %1$I', t);
        EXECUTE format('CREATE TRIGGER trg_%1$s_append_only BEFORE UPDATE OR DELETE ON %1$I FOR EACH ROW EXECUTE FUNCTION fn_evidence_append_only()', t);
    END LOOP;
END $$;

-- Runtime role: read-only. Evidence is written by the operator command only.
GRANT SELECT ON exchange_source_files, exchange_daily_bars, ca_reconciliation_runs, ca_reconciliation_findings TO halal_app_user;

-- Vendor (ohlcv_daily) vs exchange (bhavcopy) reconciliation, per stored bar.
CREATE OR REPLACE VIEW v_daily_bar_reconciliation AS
WITH v AS (
    SELECT d.symbol_id, (d.timestamp AT TIME ZONE 'Asia/Kolkata')::date AS trade_date,
           d.open, d.high, d.low, d.close, d.volume, d.adjusted_close
    FROM ohlcv_daily d
)
SELECT s.symbol,
       COALESCE(v.symbol_id, e.symbol_id) AS symbol_id,
       COALESCE(v.trade_date, e.trade_date) AS trade_date,
       v.close  AS vendor_close,
       e.close  AS exchange_close,
       v.adjusted_close IS NULL AS vendor_quarantined,
       CASE WHEN v.close IS NOT NULL AND e.close IS NOT NULL THEN round(v.close / e.close, 6) END AS close_ratio,
       CASE
           WHEN e.symbol_id IS NULL AND f.id IS NULL THEN 'NO_EXCHANGE_FILE'
           WHEN e.symbol_id IS NULL THEN 'MISSING_EXCHANGE_BAR'
           WHEN v.symbol_id IS NULL THEN 'MISSING_VENDOR_BAR'
           WHEN abs(v.close / e.close - 1) <= 0.0005 AND abs(v.open / e.open - 1) <= 0.0005
            AND abs(v.high / e.high - 1) <= 0.0005 AND abs(v.low / e.low - 1) <= 0.0005 THEN 'NOMINAL_MATCH'
           WHEN abs((v.close / e.close) - (v.open / e.open)) <= 0.0005
            AND abs((v.close / e.close) - (v.high / e.high)) <= 0.0005 THEN 'VENDOR_ADJUSTED'
           ELSE 'MISMATCH'
       END AS classification,
       CASE WHEN v.volume IS NOT NULL AND e.volume > 0 THEN round(v.volume::numeric / e.volume, 6) END AS volume_ratio
FROM v
FULL OUTER JOIN exchange_daily_bars e ON e.symbol_id = v.symbol_id AND e.trade_date = v.trade_date
LEFT JOIN exchange_source_files f ON f.trade_date = COALESCE(v.trade_date, e.trade_date)
JOIN symbols s ON s.id = COALESCE(v.symbol_id, e.symbol_id);

GRANT SELECT ON v_daily_bar_reconciliation TO halal_app_user;

-- First receipt time of intraday bars. Existing rows stay NULL ("unknown, received before P1.9.24").
ALTER TABLE ohlcv_intraday ADD COLUMN IF NOT EXISTS first_received_at TIMESTAMPTZ;
ALTER TABLE ohlcv_intraday ALTER COLUMN first_received_at SET DEFAULT now();

COMMIT;
