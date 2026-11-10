-- P1.9.29: track upcoming (candidate) AND confirmed breakouts, store the reasons, de-duplicate repeat signals, per-stock test results,
-- stock news / filings / events, and automatic paper portfolios that trade the system's own signals.
-- Additive only: no existing column is changed or dropped.

-- ---------------------------------------------------------------------------------------------- confirmed breakouts
ALTER TABLE breakout_tracking
    ADD COLUMN IF NOT EXISTS reasons          jsonb,
    ADD COLUMN IF NOT EXISTS stage            text,
    ADD COLUMN IF NOT EXISTS resistance_level numeric(18,4),
    ADD COLUMN IF NOT EXISTS atr              numeric(18,4),
    ADD COLUMN IF NOT EXISTS score_breakdown  jsonb,
    ADD COLUMN IF NOT EXISTS counts_in_stats  boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS excluded_reason  text;

-- A stock that re-signals while an earlier signal is still open, or within 7 calendar days (about 5 sessions, the same cool-down the
-- research study uses), is a repeat of the same move. It stays visible but no longer counts in statistics or paper trades.
-- Walk each symbol's signals oldest first; the first one counts, and a later one counts only once the previous counted one is
-- resolved and 7 days have passed.
DO $$
DECLARE r record; last_counted date; last_resolved timestamptz; cur_symbol int := -1; dup boolean;
BEGIN
    FOR r IN SELECT id, symbol_id, signal_date, resolved_at FROM breakout_tracking ORDER BY symbol_id, signal_date, id LOOP
        IF r.symbol_id <> cur_symbol THEN cur_symbol := r.symbol_id; last_counted := NULL; last_resolved := NULL; END IF;
        dup := last_counted IS NOT NULL AND (r.signal_date - last_counted < 7 OR last_resolved IS NULL);
        IF dup THEN
            UPDATE breakout_tracking SET counts_in_stats = false,
                   excluded_reason = 'REPEAT_SIGNAL: the signal of ' || to_char(last_counted, 'YYYY-MM-DD') || ' is still open or under 5 sessions old'
            WHERE id = r.id;
        ELSE
            last_counted := r.signal_date; last_resolved := r.resolved_at;
        END IF;
    END LOOP;
END $$;

-- ---------------------------------------------------------------------------------------------- upcoming (candidate) breakouts
CREATE TABLE IF NOT EXISTS breakout_watch (
    id                  bigserial PRIMARY KEY,
    symbol_id           integer       NOT NULL REFERENCES symbols(id),
    flagged_date        date          NOT NULL,
    origin              text          NOT NULL DEFAULT 'LIVE' CHECK (origin IN ('LIVE', 'HISTORY_REPLAY')),
    stage               text          NOT NULL DEFAULT 'BREAKOUT_CANDIDATE' CHECK (stage = 'BREAKOUT_CANDIDATE'),
    flagged_close       numeric(18,4) NOT NULL,
    resistance_level    numeric(18,4) NOT NULL,
    stop_level          numeric(18,4) NOT NULL,
    entry_low           numeric(18,4),
    entry_high          numeric(18,4),
    target_1            numeric(18,4),
    target_2            numeric(18,4),
    structure_score     numeric(8,2),
    rvol                numeric(10,3),
    atr                 numeric(18,4),
    reasons             jsonb,
    score_breakdown     jsonb,
    shariah_status      text,
    first_seen_at       timestamptz   NOT NULL DEFAULT now(),
    -- outcome: did the engine's own CONFIRMED_BREAKOUT follow within 10 sessions? (the same label the nightly study tests)
    status              text          NOT NULL DEFAULT 'WATCHING' CHECK (status IN ('WATCHING', 'BROKE_OUT', 'INVALIDATED', 'EXPIRED')),
    sessions_elapsed    integer       NOT NULL DEFAULT 0,
    last_session        date,
    last_close          numeric(18,4),
    broke_out_session   date,
    sessions_to_breakout integer,
    ret_5d_pct          numeric(10,4),
    ret_10d_pct         numeric(10,4),
    max_gain_pct        numeric(10,4),
    max_drawdown_pct    numeric(10,4),
    resolved_at         timestamptz,
    updated_at          timestamptz   NOT NULL DEFAULT now(),
    counts_in_stats     boolean       NOT NULL DEFAULT true,
    UNIQUE (symbol_id, flagged_date)
);
CREATE INDEX IF NOT EXISTS idx_breakout_watch_open ON breakout_watch (status) WHERE status = 'WATCHING';

-- The flag as it stood on the day is frozen: the outcome columns may change, the claim may not.
CREATE OR REPLACE FUNCTION fn_breakout_watch_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.symbol_id, NEW.flagged_date, NEW.origin, NEW.flagged_close, NEW.resistance_level, NEW.stop_level, NEW.target_1, NEW.target_2, NEW.reasons::text)
       IS DISTINCT FROM
       (OLD.symbol_id, OLD.flagged_date, OLD.origin, OLD.flagged_close, OLD.resistance_level, OLD.stop_level, OLD.target_1, OLD.target_2, OLD.reasons::text) THEN
        RAISE EXCEPTION 'a breakout flag is frozen on the day it is raised and cannot be changed';
    END IF;
    IF OLD.status <> 'WATCHING' AND NEW.status IS DISTINCT FROM OLD.status THEN
        RAISE EXCEPTION 'a resolved breakout flag cannot change its outcome';
    END IF;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS trg_breakout_watch_immutable ON breakout_watch;
CREATE TRIGGER trg_breakout_watch_immutable BEFORE UPDATE ON breakout_watch FOR EACH ROW EXECUTE FUNCTION fn_breakout_watch_immutable();

-- ---------------------------------------------------------------------------------------------- per-stock test results (append-only)
CREATE TABLE IF NOT EXISTS stock_signal_stats (
    id         bigserial PRIMARY KEY,
    symbol_id  integer     NOT NULL REFERENCES symbols(id),
    as_of      date        NOT NULL,
    result     jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_stock_signal_stats_latest ON stock_signal_stats (symbol_id, id DESC);
CREATE OR REPLACE VIEW stock_signal_stats_latest AS
    SELECT DISTINCT ON (symbol_id) symbol_id, as_of, result, created_at FROM stock_signal_stats ORDER BY symbol_id, id DESC;

-- ---------------------------------------------------------------------------------------------- news, filings, events
CREATE TABLE IF NOT EXISTS stock_news (
    id           bigserial PRIMARY KEY,
    symbol_id    integer     NOT NULL REFERENCES symbols(id),
    source       text        NOT NULL,
    title        text        NOT NULL,
    publisher    text,
    url          text        NOT NULL,
    url_sha256   char(64)    NOT NULL,
    published_at timestamptz,
    fetched_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol_id, url_sha256)
);
CREATE INDEX IF NOT EXISTS idx_stock_news_symbol ON stock_news (symbol_id, published_at DESC);

CREATE TABLE IF NOT EXISTS stock_filings (
    id             bigserial PRIMARY KEY,
    symbol_id      integer     NOT NULL REFERENCES symbols(id),
    source         text        NOT NULL DEFAULT 'NSE_ANNOUNCEMENT',
    announced_at   timestamptz NOT NULL,
    category       text,
    subject        text        NOT NULL,
    attachment_url text,
    key_sha256     char(64)    NOT NULL,
    fetched_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol_id, key_sha256)
);
CREATE INDEX IF NOT EXISTS idx_stock_filings_symbol ON stock_filings (symbol_id, announced_at DESC);

CREATE TABLE IF NOT EXISTS stock_events (
    id         bigserial PRIMARY KEY,
    symbol_id  integer     NOT NULL REFERENCES symbols(id),
    kind       text        NOT NULL CHECK (kind IN ('BOARD_MEETING', 'CORPORATE_ACTION')),
    event_date date        NOT NULL,
    title      text        NOT NULL,
    detail     text,
    source     text        NOT NULL,
    key_sha256 char(64)    NOT NULL,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (symbol_id, key_sha256)
);
CREATE INDEX IF NOT EXISTS idx_stock_events_symbol ON stock_events (symbol_id, event_date DESC);

CREATE TABLE IF NOT EXISTS news_fetch_log (
    symbol_id  integer     NOT NULL REFERENCES symbols(id),
    feed       text        NOT NULL,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    ok         boolean     NOT NULL,
    items      integer     NOT NULL DEFAULT 0,
    error      text,
    PRIMARY KEY (symbol_id, feed, fetched_at)
);

-- ---------------------------------------------------------------------------------------------- automatic paper trading
ALTER TABLE portfolios
    ADD COLUMN IF NOT EXISTS kind  text NOT NULL DEFAULT 'MANUAL' CHECK (kind IN ('MANUAL', 'AUTO_STRATEGY')),
    ADD COLUMN IF NOT EXISTS label text;

-- One row per decision the automatic trader took on a signal, so nothing is ever traded twice and every skip has a reason.
CREATE TABLE IF NOT EXISTS auto_paper_actions (
    id           bigserial PRIMARY KEY,
    portfolio_id integer     NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    tracking_id  bigint      NOT NULL REFERENCES breakout_tracking(id),
    leg          text        NOT NULL CHECK (leg IN ('ENTRY', 'EXIT')),
    session_date date        NOT NULL,
    action       text        NOT NULL CHECK (action IN ('FILLED', 'SKIPPED')),
    reason       text,
    trade_id     integer,
    price        numeric(18,4),
    shares       integer,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (portfolio_id, tracking_id, leg)
);
CREATE TABLE IF NOT EXISTS auto_paper_state (
    portfolio_id      integer PRIMARY KEY REFERENCES portfolios(id) ON DELETE CASCADE,
    strategy_name     text    NOT NULL,
    plan              jsonb   NOT NULL,
    processed_through date,
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------------------------- grants (runtime role)
GRANT SELECT, INSERT, UPDATE ON breakout_watch, auto_paper_state TO halal_app_user;
GRANT SELECT, INSERT ON stock_signal_stats, stock_news, stock_filings, stock_events, news_fetch_log TO halal_app_user;
GRANT SELECT, INSERT, UPDATE ON auto_paper_actions TO halal_app_user;
GRANT SELECT ON stock_signal_stats_latest TO halal_app_user;
GRANT USAGE ON SEQUENCE breakout_watch_id_seq, stock_signal_stats_id_seq, stock_news_id_seq, stock_filings_id_seq, stock_events_id_seq, auto_paper_actions_id_seq TO halal_app_user;
