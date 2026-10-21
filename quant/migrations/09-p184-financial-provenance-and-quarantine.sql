-- ============================================================================
-- Migration 09: Financial Provenance, Data Quality & Durable Ingestion Runs
--
-- Objective:
-- 1. Extend data_quality_status enum with ADJUSTMENT_SENSITIVE.
-- 2. Add explicit provenance, currency, units, and filing references to financial_statements.
-- 3. Create eod_ingestion_runs for durable operational auditing of scheduled runs.
-- ============================================================================

-- 1. Extend data_quality_status enum (PostgreSQL does not allow ALTER TYPE inside transaction block)
ALTER TYPE data_quality_status ADD VALUE IF NOT EXISTS 'ADJUSTMENT_SENSITIVE';

-- 2. Add provenance and unit columns to financial_statements
ALTER TABLE financial_statements ADD COLUMN IF NOT EXISTS currency VARCHAR(10) DEFAULT 'INR';
ALTER TABLE financial_statements ADD COLUMN IF NOT EXISTS unit VARCHAR(20) DEFAULT 'RAW_INR';
ALTER TABLE financial_statements ADD COLUMN IF NOT EXISTS filing_reference TEXT;
ALTER TABLE financial_statements ADD COLUMN IF NOT EXISTS provenance JSONB;

-- 3. Create durable operational run records table for EOD market data ingestion
CREATE TABLE IF NOT EXISTS eod_ingestion_runs (
    id SERIAL PRIMARY KEY,
    run_id VARCHAR(64) UNIQUE NOT NULL,
    trigger_type VARCHAR(20) NOT NULL,               -- 'SCHEDULED', 'MANUAL', 'RETRY'
    provider VARCHAR(50) NOT NULL,                   -- 'yahoo_finance', 'nse_direct'
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL,                     -- 'RUNNING', 'SUCCESS', 'PARTIAL_DEGRADED', 'FAILED'
    symbols_attempted INT NOT NULL DEFAULT 0,
    candles_fetched INT NOT NULL DEFAULT 0,
    candles_stored INT NOT NULL DEFAULT 0,
    candles_failed INT NOT NULL DEFAULT 0,
    retry_count INT NOT NULL DEFAULT 0,
    is_exchange_verified BOOLEAN NOT NULL DEFAULT FALSE,
    error_details TEXT,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eod_ingestion_runs_status ON eod_ingestion_runs(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_eod_ingestion_runs_started ON eod_ingestion_runs(started_at DESC);

COMMENT ON TABLE eod_ingestion_runs IS 'Durable execution and audit ledger for EOD daily market data ingestion runs';
