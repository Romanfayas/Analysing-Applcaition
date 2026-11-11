-- P1.9.31: paper-test of every breakout stage on the stock's own past. Each nightly run replays the engine day by day (it only sees data up to that
-- day), opens a simulated trade for every past "closest to breaking out", "upcoming" and "confirmed" reading, and stores each trade with its
-- entry/exit date and (where 1-minute bars exist) time. Derived data: a run can be rebuilt from the price series, so old runs are pruned.
-- Additive only.
CREATE TABLE IF NOT EXISTS pattern_replay_runs (
    id         bigserial PRIMARY KEY,
    as_of      date        NOT NULL,
    summary    jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pattern_replay_trades (
    id            bigserial PRIMARY KEY,
    run_id        bigint        NOT NULL REFERENCES pattern_replay_runs(id) ON DELETE CASCADE,
    symbol_id     integer       NOT NULL REFERENCES symbols(id),
    setup         text          NOT NULL CHECK (setup IN ('CLOSEST', 'UPCOMING', 'CONFIRMED')),
    signal_date   date          NOT NULL,
    signal_close  numeric(18,4) NOT NULL,
    distance_pct  numeric(8,3),
    resistance    numeric(18,4),
    stop_level    numeric(18,4),
    target_1      numeric(18,4),
    outcome       text          NOT NULL CHECK (outcome IN ('HIT_TARGET', 'STOPPED', 'TIME_EXIT', 'NOT_TRIGGERED', 'FAILED_BEFORE_ENTRY', 'SKIPPED', 'OPEN', 'PENDING')),
    triggered     boolean       NOT NULL DEFAULT false,
    entry_date    date,
    entry_time    text,
    entry_price   numeric(18,4),
    exit_date     date,
    exit_time     text,
    exit_price    numeric(18,4),
    bars_held     integer,
    gross_pct     numeric(10,4),
    net_pct       numeric(10,4),
    excess_vs_nifty_pct numeric(10,4),
    mfe_pct       numeric(10,4),
    mae_pct       numeric(10,4),
    move_10d_pct  numeric(10,4),
    note          text
);
CREATE INDEX IF NOT EXISTS idx_pattern_replay_trades_run ON pattern_replay_trades (run_id, setup, symbol_id, signal_date);

GRANT SELECT, INSERT, DELETE ON pattern_replay_runs, pattern_replay_trades TO halal_app_user;
GRANT USAGE ON SEQUENCE pattern_replay_runs_id_seq, pattern_replay_trades_id_seq TO halal_app_user;
