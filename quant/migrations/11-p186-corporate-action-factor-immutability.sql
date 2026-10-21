-- ============================================================================
-- Migration 11: Corporate Action Adjustment Factors Immutability Guard
--
-- Objective:
-- Enforce append-only revision semantics on corporate_action_adjustment_factors.
-- Deleting records or modifying existing revision rows is forbidden at the DB trigger level.
-- Any revision or correction must be inserted as a new row with an incremented revision_number.
-- ============================================================================

CREATE OR REPLACE FUNCTION ca_factor_guard() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'corporate_action_adjustment_factors rows are immutable and may not be deleted (id=%)', OLD.id;
  END IF;
  IF TG_OP = 'UPDATE' THEN
    RAISE EXCEPTION 'corporate_action_adjustment_factors rows are append-only (id=%, revision=%); revisions must be inserted with an incremented revision_number', OLD.id, OLD.revision_number;
  END IF;
  RETURN NEW;
END $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ca_factor_guard ON corporate_action_adjustment_factors;
CREATE TRIGGER trg_ca_factor_guard BEFORE UPDATE OR DELETE ON corporate_action_adjustment_factors FOR EACH ROW EXECUTE FUNCTION ca_factor_guard();
