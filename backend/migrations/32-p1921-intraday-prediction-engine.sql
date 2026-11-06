-- HalalEquity Migration 32: P1.9.21 intraday prediction engine
--
--  * prediction_model_runs : one row per (horizon, training run) with walk-forward evidence and verdict.
--                            The fitted model is stored in-row (artifact + sha256) so evidence and model
--                            can never drift apart. At most one active run per horizon.
--  * predictions           : forecasts issued live. Forecast columns are IMMUTABLE once written; only the
--                            outcome columns may be filled in, once, when the horizon has elapsed. This is
--                            what makes forward tracking trustworthy.

BEGIN;

CREATE TABLE IF NOT EXISTS prediction_model_runs (
    id               BIGSERIAL PRIMARY KEY,
    model_version    TEXT        NOT NULL,
    horizon_minutes  INTEGER     NOT NULL CHECK (horizon_minutes > 0),
    trained_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data_from        DATE,
    data_to          DATE,
    n_symbols        INTEGER,
    verdict          TEXT        NOT NULL CHECK (verdict IN ('ACTIONABLE', 'NO_DEMONSTRATED_EDGE', 'DATA_INSUFFICIENT')),
    metrics          JSONB       NOT NULL,
    params           JSONB       NOT NULL,
    cost_model       JSONB       NOT NULL,
    artifact         BYTEA,
    artifact_sha256  TEXT,
    is_active        BOOLEAN     NOT NULL DEFAULT FALSE
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_prediction_model_active ON prediction_model_runs(horizon_minutes) WHERE is_active;

CREATE TABLE IF NOT EXISTS predictions (
    id               BIGSERIAL PRIMARY KEY,
    model_run_id     BIGINT      NOT NULL REFERENCES prediction_model_runs(id),
    symbol_id        INTEGER     NOT NULL REFERENCES symbols(id),
    horizon_minutes  INTEGER     NOT NULL,
    as_of            TIMESTAMPTZ NOT NULL,   -- close time of the last bar the forecast used
    target_time      TIMESTAMPTZ NOT NULL,   -- as_of + horizon, always within the same session
    issued_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reference_price  NUMERIC(18,4) NOT NULL CHECK (reference_price > 0),
    p_down           NUMERIC(7,6) NOT NULL,
    p_neutral        NUMERIC(7,6) NOT NULL,
    p_up             NUMERIC(7,6) NOT NULL,
    expected_return  NUMERIC(10,6) NOT NULL,
    return_q10       NUMERIC(10,6) NOT NULL,
    return_q50       NUMERIC(10,6) NOT NULL,
    return_q90       NUMERIC(10,6) NOT NULL,
    action           TEXT        NOT NULL CHECK (action IN ('BUY_CANDIDATE', 'EXIT_OR_AVOID', 'NO_TRADE')),
    action_reasons   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    entry_price      NUMERIC(18,4),
    target_price     NUMERIC(18,4),
    stop_price       NUMERIC(18,4),
    shariah_status   TEXT        NOT NULL,
    realized_price   NUMERIC(18,4),
    realized_return  NUMERIC(10,6),
    realized_class   TEXT CHECK (realized_class IN ('DOWN', 'NEUTRAL', 'UP')),
    resolved_at      TIMESTAMPTZ,
    CHECK (target_time > as_of),
    CHECK (abs(p_down + p_neutral + p_up - 1) < 0.0001),
    UNIQUE (model_run_id, symbol_id, as_of)
);
CREATE INDEX IF NOT EXISTS idx_predictions_recent ON predictions(issued_at DESC);
CREATE INDEX IF NOT EXISTS idx_predictions_unresolved ON predictions(target_time) WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_predictions_symbol ON predictions(symbol_id, as_of DESC);

CREATE OR REPLACE FUNCTION fn_predictions_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'predictions are append-only: issued forecasts cannot be deleted';
    END IF;
    IF (NEW.model_run_id, NEW.symbol_id, NEW.horizon_minutes, NEW.as_of, NEW.target_time, NEW.issued_at,
        NEW.reference_price, NEW.p_down, NEW.p_neutral, NEW.p_up, NEW.expected_return, NEW.return_q10,
        NEW.return_q50, NEW.return_q90, NEW.action, NEW.action_reasons, NEW.entry_price, NEW.target_price,
        NEW.stop_price, NEW.shariah_status)
       IS DISTINCT FROM
       (OLD.model_run_id, OLD.symbol_id, OLD.horizon_minutes, OLD.as_of, OLD.target_time, OLD.issued_at,
        OLD.reference_price, OLD.p_down, OLD.p_neutral, OLD.p_up, OLD.expected_return, OLD.return_q10,
        OLD.return_q50, OLD.return_q90, OLD.action, OLD.action_reasons, OLD.entry_price, OLD.target_price,
        OLD.stop_price, OLD.shariah_status) THEN
        RAISE EXCEPTION 'prediction % forecast fields are immutable once issued', OLD.id;
    END IF;
    IF OLD.resolved_at IS NOT NULL THEN
        RAISE EXCEPTION 'prediction % is already resolved', OLD.id;
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_predictions_immutable ON predictions;
CREATE TRIGGER trg_predictions_immutable BEFORE UPDATE OR DELETE ON predictions
    FOR EACH ROW EXECUTE FUNCTION fn_predictions_immutable();

GRANT SELECT, INSERT, UPDATE ON prediction_model_runs TO halal_app_user;
GRANT SELECT, INSERT, UPDATE ON predictions TO halal_app_user;
GRANT USAGE, SELECT ON SEQUENCE prediction_model_runs_id_seq, predictions_id_seq TO halal_app_user;

COMMIT;
