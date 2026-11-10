-- P1.9.28: governed learning loop. Everything here is APPEND-ONLY: a strategy version's parameters never change, a promotion or
-- rollback is a new status event, and every learner run and evaluation is kept. The runtime role gets SELECT and INSERT only.
--
-- strategy_versions        immutable definitions drawn from a finite, pre-registered space (the learner cannot invent rules)
-- strategy_status_events   CONTROL | CANDIDATE | ACTIVE_PAPER | RETIRED | ROLLED_BACK | PROBATION_FAILED ... (current = newest event)
-- learning_runs            one row per learner run: data cut-off, candidates tried, multiple-testing note, full table, decision
-- strategy_evaluations     walk-forward out-of-sample results of a version vs the frozen control, with the gate outcomes
-- breakout_lessons         one row per resolved tracked breakout: what happened and the tags that describe why

CREATE TABLE IF NOT EXISTS strategy_versions (
    id            bigserial PRIMARY KEY,
    name          text        NOT NULL UNIQUE,
    family        text        NOT NULL DEFAULT 'BREAKOUT_PLAN',
    space_version text        NOT NULL,
    params        jsonb       NOT NULL,
    params_sha256 char(64)    NOT NULL UNIQUE,
    parent_id     bigint      REFERENCES strategy_versions(id),
    origin        text        NOT NULL CHECK (origin IN ('FROZEN_CONTROL', 'LEARNER_SPACE')),
    rationale     text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS learning_runs (
    id               bigserial PRIMARY KEY,
    space_version    text        NOT NULL,
    data_through     date        NOT NULL,
    candidates_tried integer     NOT NULL,
    result           jsonb       NOT NULL,
    decision         text        NOT NULL CHECK (decision IN ('PROMOTED', 'NO_CHANGE_GATES_NOT_MET', 'NO_CHANGE_INSUFFICIENT_DATA', 'ROLLED_BACK_PROBATION', 'KEPT_ACTIVE')),
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS strategy_evaluations (
    id               bigserial PRIMARY KEY,
    run_id           bigint      NOT NULL REFERENCES learning_runs(id),
    version_id       bigint      NOT NULL REFERENCES strategy_versions(id),
    kind             text        NOT NULL CHECK (kind IN ('SELECTION_POLICY_WALK_FORWARD', 'FORWARD_PROBATION')),
    oos_trades       integer     NOT NULL,
    oos_mean_net_pct numeric(10,4),
    oos_ci_low       numeric(10,4),
    oos_ci_high      numeric(10,4),
    control_trades   integer     NOT NULL,
    control_mean_net_pct numeric(10,4),
    diff_mean_pct    numeric(10,4),
    diff_ci_low      numeric(10,4),
    diff_ci_high     numeric(10,4),
    gates            jsonb       NOT NULL,
    passed           boolean     NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS strategy_status_events (
    id            bigserial PRIMARY KEY,
    version_id    bigint      NOT NULL REFERENCES strategy_versions(id),
    status        text        NOT NULL CHECK (status IN ('CONTROL', 'CANDIDATE', 'ACTIVE_PAPER', 'RETIRED', 'ROLLED_BACK', 'PROBATION_FAILED')),
    reason        text        NOT NULL,
    evaluation_id bigint      REFERENCES strategy_evaluations(id),
    actor         text        NOT NULL CHECK (actor IN ('LEARNER', 'OWNER', 'WATCHDOG', 'SEED')),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_status_events_version ON strategy_status_events (version_id, id DESC);

CREATE TABLE IF NOT EXISTS breakout_lessons (
    tracking_id      bigint PRIMARY KEY REFERENCES breakout_tracking(id),
    origin           text        NOT NULL,
    symbol_id        integer     NOT NULL REFERENCES symbols(id),
    signal_date      date        NOT NULL,
    verdict          text        NOT NULL,
    return_pct       numeric(10,4),
    best_pct         numeric(10,4),
    worst_pct        numeric(10,4),
    sessions         integer,
    rvol             numeric(10,3),
    structure_score  numeric(8,2),
    nifty_return_pct numeric(10,4),
    tags             text[]      NOT NULL DEFAULT '{}',
    lesson           text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_breakout_lessons_tags ON breakout_lessons USING gin (tags);

-- Append-only: refuse any UPDATE or DELETE (a superuser can still repair with session_replication_role = replica).
CREATE OR REPLACE FUNCTION fn_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only: % is not allowed (corrections are new rows)', TG_TABLE_NAME, TG_OP;
END $$;

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['strategy_versions', 'learning_runs', 'strategy_evaluations', 'strategy_status_events', 'breakout_lessons'] LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS trg_append_only ON %I', t);
        EXECUTE format('CREATE TRIGGER trg_append_only BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION fn_append_only()', t);
    END LOOP;
END $$;

-- Current status of every version = its newest event.
CREATE OR REPLACE VIEW strategy_current AS
SELECT v.id, v.name, v.origin, v.params, v.params_sha256, v.rationale, v.created_at,
       e.status, e.reason AS status_reason, e.actor, e.created_at AS status_at, e.evaluation_id
FROM strategy_versions v
LEFT JOIN LATERAL (SELECT * FROM strategy_status_events x WHERE x.version_id = v.id ORDER BY x.id DESC LIMIT 1) e ON true;

GRANT SELECT, INSERT ON strategy_versions, learning_runs, strategy_evaluations, strategy_status_events, breakout_lessons TO halal_app_user;
GRANT SELECT ON strategy_current TO halal_app_user;
GRANT USAGE ON SEQUENCE strategy_versions_id_seq, learning_runs_id_seq, strategy_evaluations_id_seq, strategy_status_events_id_seq TO halal_app_user;
