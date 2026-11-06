-- HalalEquity Migration 30: P1.9.20 widen ipos.status
--
-- The platform's IPO lifecycle vocabulary includes CLOSED_AWAITING_LISTING (23 chars), which the
-- varchar(20) column rejected (SQLSTATE 22001), silently dropping every closed-bidding issue.

BEGIN;
ALTER TABLE ipos ALTER COLUMN status TYPE varchar(32);
COMMIT;
