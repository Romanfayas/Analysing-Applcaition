-- P1.9.27: every confirmed breakout, recorded once with the plan frozen at the signal, then followed session by session.
-- The plan is immutable (a trigger refuses changes), so results cannot be flattered by moving the goalposts afterwards.
CREATE TABLE IF NOT EXISTS breakout_tracking (
    id              bigserial PRIMARY KEY,
    symbol_id       integer       NOT NULL REFERENCES symbols(id),
    signal_date     date          NOT NULL,
    origin          text          NOT NULL DEFAULT 'LIVE' CHECK (origin IN ('LIVE', 'HISTORY_REPLAY')),
    signal_close    numeric(18,4) NOT NULL,
    entry_low       numeric(18,4) NOT NULL,
    entry_high      numeric(18,4) NOT NULL,
    stop_level      numeric(18,4) NOT NULL,
    target_1        numeric(18,4) NOT NULL,
    target_2        numeric(18,4) NOT NULL,
    structure_score numeric(8,2),
    rvol            numeric(10,3),
    shariah_status  text,
    first_seen_at   timestamptz   NOT NULL DEFAULT now(),
    -- what happened (updated by the tracker)
    status          text          NOT NULL DEFAULT 'AWAITING_OPEN',
    verdict         text          NOT NULL DEFAULT 'PENDING',
    entry_at        timestamptz,
    entry_price     numeric(18,4),
    exit_at         timestamptz,
    exit_price      numeric(18,4),
    return_pct      numeric(10,4),
    best_pct        numeric(10,4),
    worst_pct       numeric(10,4),
    sessions        integer       NOT NULL DEFAULT 0,
    target_1_at     timestamptz,
    target_2_at     timestamptz,
    stop_at         timestamptz,
    closed_below_at timestamptz,
    last_price      numeric(18,4),
    last_price_at   timestamptz,
    nifty_return_pct numeric(10,4),
    resolved_at     timestamptz,
    updated_at      timestamptz   NOT NULL DEFAULT now(),
    UNIQUE (symbol_id, signal_date)
);
CREATE INDEX IF NOT EXISTS idx_breakout_tracking_open ON breakout_tracking (resolved_at) WHERE resolved_at IS NULL;

CREATE OR REPLACE FUNCTION fn_breakout_plan_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.symbol_id, NEW.signal_date, NEW.signal_close, NEW.entry_low, NEW.entry_high, NEW.stop_level, NEW.target_1, NEW.target_2, NEW.origin)
       IS DISTINCT FROM
       (OLD.symbol_id, OLD.signal_date, OLD.signal_close, OLD.entry_low, OLD.entry_high, OLD.stop_level, OLD.target_1, OLD.target_2, OLD.origin) THEN
        RAISE EXCEPTION 'a breakout plan is frozen at the signal and cannot be changed';
    END IF;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS trg_breakout_plan_immutable ON breakout_tracking;
CREATE TRIGGER trg_breakout_plan_immutable BEFORE UPDATE ON breakout_tracking FOR EACH ROW EXECUTE FUNCTION fn_breakout_plan_immutable();

GRANT SELECT, INSERT, UPDATE ON breakout_tracking TO halal_app_user;
GRANT USAGE ON SEQUENCE breakout_tracking_id_seq TO halal_app_user;
