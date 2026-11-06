-- P1.9.27: stored results of research runs on the NSE exchange series. Append-only by convention: each run is a new row.
CREATE TABLE IF NOT EXISTS research_runs (
    id             bigserial PRIMARY KEY,
    kind           text        NOT NULL,
    version        text        NOT NULL,
    series_version text        NOT NULL,
    result         jsonb       NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_research_runs_kind ON research_runs (kind, id DESC);
GRANT SELECT, INSERT ON research_runs TO halal_app_user;
GRANT USAGE ON SEQUENCE research_runs_id_seq TO halal_app_user;
