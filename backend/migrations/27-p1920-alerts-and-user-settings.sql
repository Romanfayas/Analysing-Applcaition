-- HalalEquity Migration 27: P1.9.20 Application Completion — alerts and per-user settings
--
-- Replaces fake-success alert/settings endpoints with real persistence.
--  * user_alerts   : price alerts owned by a user (evaluated by the worker; one-shot)
--  * alert_events  : append-only record of every trigger, with the observed value
--  * user_settings : per-user display/risk preferences (no secrets are ever stored here)
-- The runtime role receives only the DML it needs; alert_events is insert/select only.

BEGIN;

CREATE TABLE IF NOT EXISTS user_alerts (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol_id   INTEGER     NOT NULL REFERENCES symbols(id),
    alert_type  TEXT        NOT NULL CHECK (alert_type IN ('PRICE_ABOVE', 'PRICE_BELOW')),
    threshold   NUMERIC(18,4) NOT NULL CHECK (threshold > 0),
    channel     TEXT        NOT NULL DEFAULT 'IN_APP' CHECK (channel IN ('IN_APP', 'TELEGRAM')),
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    triggered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_user_alerts_user ON user_alerts(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_alerts_active ON user_alerts(symbol_id) WHERE is_active;

CREATE TABLE IF NOT EXISTS alert_events (
    id             BIGSERIAL PRIMARY KEY,
    alert_id       BIGINT      NOT NULL REFERENCES user_alerts(id) ON DELETE CASCADE,
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    symbol_id      INTEGER     NOT NULL REFERENCES symbols(id),
    alert_type     TEXT        NOT NULL,
    threshold      NUMERIC(18,4) NOT NULL,
    observed_value NUMERIC(18,4) NOT NULL,
    observed_at    TIMESTAMPTZ NOT NULL,
    message        TEXT        NOT NULL,
    triggered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_events_user ON alert_events(user_id, triggered_at DESC);

CREATE TABLE IF NOT EXISTS user_settings (
    user_id           UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_benchmark TEXT          NOT NULL DEFAULT 'NIFTY 50'
                      CHECK (default_benchmark IN ('NIFTY 50', 'NIFTY 500', 'SENSEX')),
    default_risk_pct  NUMERIC(4,2)  NOT NULL DEFAULT 1.00
                      CHECK (default_risk_pct > 0 AND default_risk_pct <= 5),
    display_timezone  TEXT          NOT NULL DEFAULT 'Asia/Kolkata'
                      CHECK (display_timezone IN ('Asia/Kolkata', 'UTC')),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON user_alerts, user_settings TO halal_app_user;
GRANT SELECT, INSERT ON alert_events TO halal_app_user;
GRANT USAGE, SELECT ON SEQUENCE user_alerts_id_seq, alert_events_id_seq TO halal_app_user;

COMMIT;
