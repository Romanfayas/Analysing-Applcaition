-- HalalEquity Migration 28: P1.9.20 per-user paper-portfolio ownership
--
-- portfolios.user_id was an INTEGER hardcoded to 1 ("single-user MVP") while real accounts are
-- UUIDs, so every authenticated user shared one ledger. Ownership now follows users.id.
--  * owner_id UUID -> users(id), unique: exactly one paper portfolio per account
--  * user_id becomes nullable: it is retained only for the legacy pre-auth row, which has no
--    positions or trades and is left untouched (not adopted by any account).

BEGIN;

ALTER TABLE portfolios ADD COLUMN IF NOT EXISTS owner_id UUID REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE portfolios ALTER COLUMN user_id DROP NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS portfolios_owner_id_key ON portfolios(owner_id) WHERE owner_id IS NOT NULL;

COMMIT;
