-- Migration 12: P1.8.7 EOD Ingestion Reconciliation & Outcome Accounting
-- Adds reconciliation_status to eod_ingestion_runs to ensure every fetched candle
-- is accounted for in mutually exclusive, exhaustive categories before marking SUCCESS.

ALTER TABLE eod_ingestion_runs
ADD COLUMN IF NOT EXISTS reconciliation_status character varying(30) NOT NULL DEFAULT 'UNRECONCILED';

-- Add check constraint for valid reconciliation status values
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_eod_reconciliation_status'
    ) THEN
        ALTER TABLE eod_ingestion_runs
        ADD CONSTRAINT chk_eod_reconciliation_status
        CHECK (reconciliation_status IN ('RECONCILED', 'UNRECONCILED', 'EXCLUDED_FROM_AUDIT'));
    END IF;
END $$;

COMMENT ON COLUMN eod_ingestion_runs.reconciliation_status IS 'Strict reconciliation status verifying fetched == inserted + updated + unchanged + quarantined + rejected + explicitly_skipped';
