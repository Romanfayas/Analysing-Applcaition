-- HalalEquity Migration 29: P1.9.20 IPO feed integrity
--
-- The NSE current-issue parser used snake_case JSON tags against a camelCase feed, so every record
-- arrived with an empty company name and all of them collapsed onto one ("", NSE) row (id 8, MAXENGS).
-- That row is parser debris, carries no dependents, and is removed; a CHECK prevents a repeat.

BEGIN;

DELETE FROM ipos
WHERE btrim(company_name) = ''
  AND NOT EXISTS (SELECT 1 FROM ipo_decisions   d WHERE d.ipo_id = ipos.id)
  AND NOT EXISTS (SELECT 1 FROM ipo_predictions p WHERE p.ipo_id = ipos.id)
  AND NOT EXISTS (SELECT 1 FROM ipo_gmp         g WHERE g.ipo_id = ipos.id)
  AND NOT EXISTS (SELECT 1 FROM ipo_financials  f WHERE f.ipo_id = ipos.id)
  AND NOT EXISTS (SELECT 1 FROM ipo_subscriptions s WHERE s.ipo_id = ipos.id);

ALTER TABLE ipos DROP CONSTRAINT IF EXISTS ipos_company_name_not_blank;
ALTER TABLE ipos ADD CONSTRAINT ipos_company_name_not_blank CHECK (btrim(company_name) <> '');

COMMIT;
