-- ============================================================================
-- Migration 10: Corporate Action Adjustment Factors & Historical Price Lineage
--
-- Objective:
-- 1. Create corporate_action_adjustment_factors table with immutable revision lineage,
--    effective dates, ratio mathematics, source circular references, and verification status.
-- 2. Populate known corporate action factor records with formal circular references.
-- ============================================================================

CREATE TABLE IF NOT EXISTS corporate_action_adjustment_factors (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    action_type VARCHAR(30) NOT NULL,               -- 'SPLIT', 'BONUS', 'CASH_DIVIDEND', 'RIGHTS', 'DEMERGER', 'SYMBOL_CHANGE'
    ex_date DATE NOT NULL,
    record_date DATE,
    effective_date DATE NOT NULL,
    split_ratio_old NUMERIC(14, 6),
    split_ratio_new NUMERIC(14, 6),
    dividend_amount NUMERIC(14, 4),
    cash_dividend_per_share NUMERIC(14, 4),
    adjustment_factor NUMERIC(18, 8) NOT NULL,      -- Factor applied to pre-event price to align with post-event price
    cumulative_split_factor NUMERIC(18, 8) NOT NULL DEFAULT 1.0,
    cumulative_dividend_factor NUMERIC(18, 8) NOT NULL DEFAULT 1.0,
    cumulative_total_factor NUMERIC(18, 8) NOT NULL DEFAULT 1.0,
    source_reference TEXT NOT NULL,                 -- Circular number or filing reference (e.g. NSE/CML/70875)
    retrieval_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revision_number INT NOT NULL DEFAULT 1,
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    verification_status VARCHAR(40) NOT NULL DEFAULT 'UNVERIFIED', -- 'VERIFIED_EXCHANGE_PRIMARY', 'UNVERIFIED_BROKER_FEED', 'PROVISIONAL'
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ca_adjustment_factor UNIQUE (symbol_id, ex_date, action_type, revision_number)
);

CREATE INDEX IF NOT EXISTS idx_ca_adj_symbol_date ON corporate_action_adjustment_factors(symbol_id, ex_date DESC);
CREATE INDEX IF NOT EXISTS idx_ca_adj_status ON corporate_action_adjustment_factors(verification_status, is_verified);

COMMENT ON TABLE corporate_action_adjustment_factors IS 'Audited, reproducible corporate action factor ledger for historical OHLCV adjustments';

-- Seed initial documented corporate actions with circular provenance
INSERT INTO corporate_action_adjustment_factors (
    symbol_id, action_type, ex_date, record_date, effective_date,
    split_ratio_old, split_ratio_new, dividend_amount, cash_dividend_per_share,
    adjustment_factor, cumulative_split_factor, cumulative_dividend_factor, cumulative_total_factor,
    source_reference, is_verified, verification_status, details
)
SELECT 
    s.id, 'DEMERGER', '2025-10-24'::date, '2025-10-14'::date, '2025-10-24'::date,
    1.0, 1.0, NULL, NULL,
    0.68850000, 1.00000000, 1.00000000, 0.68850000,
    'NSE_CIRCULAR_NSE/CML/70875', TRUE, 'VERIFIED_EXCHANGE_PRIMARY',
    '{"scheme": "Composite Scheme of Arrangement", "entities": [{"ticker": "TMPV", "cost_pct": 68.85}, {"ticker": "TMCV", "cost_pct": 31.15}]}'::jsonb
FROM symbols s WHERE s.symbol = 'TATAMOTORS'
ON CONFLICT (symbol_id, ex_date, action_type, revision_number) DO NOTHING;

INSERT INTO corporate_action_adjustment_factors (
    symbol_id, action_type, ex_date, record_date, effective_date,
    split_ratio_old, split_ratio_new, dividend_amount, cash_dividend_per_share,
    adjustment_factor, cumulative_split_factor, cumulative_dividend_factor, cumulative_total_factor,
    source_reference, is_verified, verification_status, details
)
SELECT 
    s.id, 'SYMBOL_CHANGE', '2026-02-27'::date, '2026-02-27'::date, '2026-02-27'::date,
    1.0, 1.0, NULL, NULL,
    1.00000000, 1.00000000, 1.00000000, 1.00000000,
    'NSE_CIRCULAR_LTIM_LTM_RENAME_2026', TRUE, 'VERIFIED_EXCHANGE_PRIMARY',
    '{"note": "1:1 ticker rename from LTIM to LTM on NSE"}'::jsonb
FROM symbols s WHERE s.symbol = 'LTIM'
ON CONFLICT (symbol_id, ex_date, action_type, revision_number) DO NOTHING;

INSERT INTO corporate_action_adjustment_factors (
    symbol_id, action_type, ex_date, record_date, effective_date,
    split_ratio_old, split_ratio_new, dividend_amount, cash_dividend_per_share,
    adjustment_factor, cumulative_split_factor, cumulative_dividend_factor, cumulative_total_factor,
    source_reference, is_verified, verification_status, details
)
SELECT 
    s.id, 'CASH_DIVIDEND', '2026-10-14'::date, '2026-10-14'::date, '2026-10-14'::date,
    NULL, NULL, 12.0000, 12.0000,
    0.99471197, 1.00000000, 0.99471197, 0.99471197,
    'NSE_DISCLOSURE_TCS_INTERIM_DIVIDEND_2026Q2', TRUE, 'VERIFIED_EXCHANGE_PRIMARY',
    '{"dividend_amount_inr": 12.00, "cum_close": 2114.40, "ex_close": 2103.22, "computed_factor": 0.99471197}'::jsonb
FROM symbols s WHERE s.symbol = 'TCS'
ON CONFLICT (symbol_id, ex_date, action_type, revision_number) DO NOTHING;
