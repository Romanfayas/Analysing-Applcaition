-- Migration 02: Paper Observations Schema
-- For forward paper-observation tracking without lookahead bias.

CREATE TABLE IF NOT EXISTS paper_observations (
    id SERIAL PRIMARY KEY,
    symbol_id INTEGER NOT NULL REFERENCES symbols(id),
    symbol VARCHAR(30) NOT NULL,
    setup_type VARCHAR(50) NOT NULL,
    candidate_timestamp TIMESTAMPTZ NOT NULL,
    signal_timestamp TIMESTAMPTZ NOT NULL,
    data_cutoff_at TIMESTAMPTZ NOT NULL,
    provider VARCHAR(50) NOT NULL,
    rule_version VARCHAR(100) NOT NULL,
    entry_trigger_price NUMERIC(18, 4) NOT NULL,
    invalidation_price NUMERIC(18, 4) NOT NULL,
    target_price NUMERIC(18, 4) NOT NULL,
    risk_reward_ratio NUMERIC(10, 4) NOT NULL,
    shariah_status VARCHAR(20) NOT NULL,
    sector VARCHAR(100),
    liquidity_bucket VARCHAR(30),
    market_regime VARCHAR(50),
    predetermined_horizons JSONB NOT NULL DEFAULT '{"1d": null, "3d": null, "5d": null, "10d": null}'::jsonb,
    subsequent_observations JSONB NOT NULL DEFAULT '[]'::jsonb,
    outcome VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    estimated_costs_bps NUMERIC(10, 2) NOT NULL DEFAULT 28.00,
    realized_gross_return NUMERIC(10, 4),
    realized_net_return NUMERIC(10, 4),
    is_complete BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_paper_obs_symbol ON paper_observations(symbol_id, candidate_timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_paper_obs_setup ON paper_observations(setup_type);
CREATE INDEX IF NOT EXISTS idx_paper_obs_status ON paper_observations(outcome, is_complete);
CREATE UNIQUE INDEX IF NOT EXISTS uq_paper_obs_candidate ON paper_observations(symbol_id, candidate_timestamp, rule_version);
