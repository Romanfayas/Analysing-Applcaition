-- ============================================================================
-- Migration 13: Corporate Action Factor Semantic Separation & Dispute Governance
--
-- Objective:
-- 1. Extend corporate_action_adjustment_factors schema to distinguish:
--    - SHARE_TRANSFORMATION (Splits, reverse splits, bonus issues)
--    - CASH_DISTRIBUTION (Cash dividends - nominal vs total return)
--    - DILUTION_OFFERING (Rights issues - TERP)
--    - TAX_COST_ALLOCATION (Demerger acquisition cost apportionment - NOT a price factor)
--    - PRICE_ADJUSTMENT (Market-derived price adjustments)
--    - SYMBOLOGY_LINEAGE (Ticker renames, successor mappings)
-- 2. Add explicit price_factor_applicable flag to guard against applying tax allocations
--    as stock price adjustment factors.
-- 3. Insert Revision 2 records honoring the append-only trigger:
--    - TCS Revision 2: Corrects reference price metadata from Oct 5 boundary (2114.40)
--      to true cum-dividend close on Oct 13 (2269.30). Mathematical factor 0.99471197 verified.
--    - TATAMOTORS Revision 2: Classifies 0.6885 as TAX_COST_ALLOCATION (TMPV 68.85%),
--      sets price_factor_applicable=FALSE and dispute_status='DISPUTED_NON_PRICE_FACTOR'.
--    - LTIM Revision 2: Explicitly classifies 1:1 rename as SYMBOLOGY_LINEAGE.
-- ============================================================================

-- 1. Schema Extensions
ALTER TABLE corporate_action_adjustment_factors
ALTER COLUMN verification_status TYPE VARCHAR(60),
ADD COLUMN IF NOT EXISTS factor_category VARCHAR(35) NOT NULL DEFAULT 'UNSPECIFIED',
ADD COLUMN IF NOT EXISTS price_factor_applicable BOOLEAN NOT NULL DEFAULT TRUE,
ADD COLUMN IF NOT EXISTS nominal_price_factor NUMERIC(18, 8) NOT NULL DEFAULT 1.0,
ADD COLUMN IF NOT EXISTS total_return_factor NUMERIC(18, 8) NOT NULL DEFAULT 1.0,
ADD COLUMN IF NOT EXISTS volume_adjustment_factor NUMERIC(18, 8) NOT NULL DEFAULT 1.0,
ADD COLUMN IF NOT EXISTS tax_cost_allocation_pct NUMERIC(8, 4),
ADD COLUMN IF NOT EXISTS cum_price_reference NUMERIC(18, 4),
ADD COLUMN IF NOT EXISTS ex_price_reference NUMERIC(18, 4),
ADD COLUMN IF NOT EXISTS dispute_status VARCHAR(40) NOT NULL DEFAULT 'UNDISPUTED',
ADD COLUMN IF NOT EXISTS dispute_reason TEXT;

CREATE INDEX IF NOT EXISTS idx_ca_adj_category ON corporate_action_adjustment_factors(factor_category, price_factor_applicable);
CREATE INDEX IF NOT EXISTS idx_ca_adj_dispute ON corporate_action_adjustment_factors(dispute_status);

-- 2. Insert Revision 2 for TCS (Cash Dividend Resolution)
INSERT INTO corporate_action_adjustment_factors (
    symbol_id, action_type, factor_category, ex_date, record_date, effective_date,
    split_ratio_old, split_ratio_new, dividend_amount, cash_dividend_per_share,
    adjustment_factor, cumulative_split_factor, cumulative_dividend_factor, cumulative_total_factor,
    nominal_price_factor, total_return_factor, volume_adjustment_factor,
    price_factor_applicable, cum_price_reference, ex_price_reference,
    source_reference, revision_number, is_verified, verification_status, dispute_status, details
)
SELECT 
    s.id, 'CASH_DIVIDEND', 'CASH_DISTRIBUTION', '2026-10-14'::date, '2026-10-14'::date, '2026-10-14'::date,
    NULL, NULL, 12.0000, 12.0000,
    0.99471197, 1.00000000, 0.99471197, 0.99471197,
    1.00000000, 0.99471197, 1.00000000,
    TRUE, 2269.3000, 2304.8999,
    'NSE_DISCLOSURE_TCS_INTERIM_DIVIDEND_2026Q2', 2, TRUE, 'VERIFIED_EXCHANGE_PRIMARY', 'UNDISPUTED',
    '{"dividend_amount_inr": 12.00, "cum_date": "2026-10-13", "cum_close": 2269.30, "ex_date": "2026-10-14", "ex_close": 2304.90, "computed_total_return_factor": 0.99471197, "lookback_boundary_oct05_raw": 2114.40, "lookback_boundary_oct05_adj": 2103.2190, "resolution": "Corrected reference price from Oct 05 boundary close (2114.40) to true cum-dividend close on Oct 13 (2269.30). Factor 0.99471197 is mathematically verified."}'::jsonb
FROM symbols s WHERE s.symbol = 'TCS'
ON CONFLICT (symbol_id, ex_date, action_type, revision_number) DO NOTHING;

-- 3. Insert Revision 2 for TATAMOTORS (Demerger Tax Allocation Resolution)
INSERT INTO corporate_action_adjustment_factors (
    symbol_id, action_type, factor_category, ex_date, record_date, effective_date,
    split_ratio_old, split_ratio_new, dividend_amount, cash_dividend_per_share,
    adjustment_factor, cumulative_split_factor, cumulative_dividend_factor, cumulative_total_factor,
    nominal_price_factor, total_return_factor, volume_adjustment_factor,
    price_factor_applicable, tax_cost_allocation_pct,
    source_reference, revision_number, is_verified, verification_status, dispute_status, dispute_reason, details
)
SELECT 
    s.id, 'DEMERGER', 'TAX_COST_ALLOCATION', '2025-10-24'::date, '2025-10-14'::date, '2025-10-24'::date,
    1.0, 1.0, NULL, NULL,
    1.00000000, 1.00000000, 1.00000000, 1.00000000,
    1.00000000, 1.00000000, 1.00000000,
    FALSE, 68.8500,
    'NSE_CIRCULAR_NSE/CML/70875', 2, FALSE, 'TAX_ALLOCATION_ONLY_PRICE_ADJUSTMENT_QUARANTINED',
    'DISPUTED_NON_PRICE_FACTOR',
    'Value 0.6885 represents Section 49(2C) tax acquisition cost allocation for TMPV, not a market price adjustment factor. Delisted parent and successor entities are quarantined from historical price scaling.',
    '{"scheme": "Composite Scheme of Arrangement", "entities": [{"ticker": "TMPV", "cost_pct": 68.85}, {"ticker": "TMCV", "cost_pct": 31.15}], "tax_section": "Section 49(2C) Income Tax Act 1961", "price_adjustment_applicable": false, "resolution": "Revision 2 explicitly classifies factor as TAX_COST_ALLOCATION with price_factor_applicable=false, preventing spurious 31.15% price drops in backtesters."}'::jsonb
FROM symbols s WHERE s.symbol = 'TATAMOTORS'
ON CONFLICT (symbol_id, ex_date, action_type, revision_number) DO NOTHING;

-- 4. Insert Revision 2 for LTIM (Symbol Change Resolution)
INSERT INTO corporate_action_adjustment_factors (
    symbol_id, action_type, factor_category, ex_date, record_date, effective_date,
    split_ratio_old, split_ratio_new, dividend_amount, cash_dividend_per_share,
    adjustment_factor, cumulative_split_factor, cumulative_dividend_factor, cumulative_total_factor,
    nominal_price_factor, total_return_factor, volume_adjustment_factor,
    price_factor_applicable,
    source_reference, revision_number, is_verified, verification_status, dispute_status, details
)
SELECT 
    s.id, 'SYMBOL_CHANGE', 'SYMBOLOGY_LINEAGE', '2026-02-27'::date, '2026-02-27'::date, '2026-02-27'::date,
    1.0, 1.0, NULL, NULL,
    1.00000000, 1.00000000, 1.00000000, 1.00000000,
    1.00000000, 1.00000000, 1.00000000,
    FALSE,
    'NSE_CIRCULAR_LTIM_LTM_RENAME_2026', 2, TRUE, 'VERIFIED_EXCHANGE_PRIMARY', 'UNDISPUTED',
    '{"note": "1:1 ticker rename from LTIM to LTM on NSE. Metadata tracking only, economic prices unchanged."}'::jsonb
FROM symbols s WHERE s.symbol = 'LTIM'
ON CONFLICT (symbol_id, ex_date, action_type, revision_number) DO NOTHING;
