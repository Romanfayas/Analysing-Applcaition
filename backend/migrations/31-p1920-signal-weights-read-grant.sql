-- HalalEquity Migration 31: P1.9.20 runtime read access to the frozen signal weights
--
-- GET /signals/weights now reads signal_weight_configs instead of returning a hardcoded literal, which
-- exposed that the least-privilege runtime role was never granted it. Read-only: the weights are part
-- of the frozen Strategy V2 definition and the API refuses to edit them.

BEGIN;
GRANT SELECT ON signal_weight_configs TO halal_app_user;
COMMIT;
