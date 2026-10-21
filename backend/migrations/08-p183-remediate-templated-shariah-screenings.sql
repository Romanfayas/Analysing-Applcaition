-- ============================================================================
-- Migration 08: Remediate Templated Shariah Screenings (P1.8.3 Audit)
--
-- Objective:
-- Revoke static, templated, and demonstration Shariah screening ratios
-- that were seeded without traceable audited financial filings.
-- Preserve full audit history in details JSONB and set status to DATA_INSUFFICIENT.
-- ============================================================================

BEGIN;

-- 1. Preserve audit trail and transition unverified templated records to DATA_INSUFFICIENT
UPDATE shariah_screenings
SET 
    status = 'DATA_INSUFFICIENT',
    source = 'SUPERSEDED_UNVERIFIED_TEMPLATE',
    details = jsonb_build_object(
        'audit_action', 'REVOKE_TEMPLATED_RATIOS',
        'remediated_in', 'P1.8.3',
        'revocation_timestamp', NOW(),
        'previous_status', status::text,
        'previous_source', source,
        'previous_debt_ratio', debt_ratio,
        'previous_interest_income_ratio', interest_income_ratio,
        'previous_cash_deposit_ratio', cash_deposit_ratio,
        'previous_receivables_ratio', receivables_ratio,
        'reason', 'P1.8.3 audit identified static templated ratios (0.0450/0.0120); revoked to DATA_INSUFFICIENT pending genuine audited financial statements'
    ),
    debt_ratio = NULL,
    debt_ratio_pass = NULL,
    interest_income_ratio = NULL,
    interest_income_pass = NULL,
    cash_deposit_ratio = NULL,
    cash_deposit_pass = NULL,
    receivables_ratio = NULL,
    receivables_pass = NULL,
    purification_per_share = NULL
WHERE source = 'AAOIFI_AUDITED' AND screening_date = '2026-10-01';

-- 2. Add an audit log entry or comment on shariah_screenings table
COMMENT ON TABLE shariah_screenings IS 'Shariah compliance screenings. Templated inputs revoked in P1.8.3; genuine audited data required for PASS.';

COMMIT;
