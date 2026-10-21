-- Migration 05 (P1.7): paper-observation lifecycle statuses, immutable decision-time snapshot, mutation guards.
BEGIN;

ALTER TABLE paper_observations
  ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
  ADD COLUMN IF NOT EXISTS status_reason TEXT,
  ADD COLUMN IF NOT EXISTS entry_fill_price NUMERIC(18,4),
  ADD COLUMN IF NOT EXISTS entry_session_date DATE,
  ADD COLUMN IF NOT EXISTS exit_price NUMERIC(18,4),
  ADD COLUMN IF NOT EXISTS exit_session_date DATE,
  ADD COLUMN IF NOT EXISTS cost_model_version VARCHAR(60),
  ADD COLUMN IF NOT EXISTS itemized_costs JSONB,
  ADD COLUMN IF NOT EXISTS notional_inr NUMERIC(14,2),
  ADD COLUMN IF NOT EXISTS calendar_version VARCHAR(80),
  ADD COLUMN IF NOT EXISTS data_source_version VARCHAR(80) NOT NULL DEFAULT 'ohlcv_daily/yahoo_finance',
  ADD COLUMN IF NOT EXISTS decision_snapshot JSONB,
  ADD COLUMN IF NOT EXISTS decision_snapshot_sha256 CHAR(64),
  ADD COLUMN IF NOT EXISTS evaluated_through_session DATE,
  ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

UPDATE paper_observations SET subsequent_observations = '[]'::jsonb WHERE subsequent_observations IS NULL OR subsequent_observations = 'null'::jsonb;
ALTER TABLE paper_observations ALTER COLUMN subsequent_observations SET DEFAULT '[]'::jsonb;

-- The only pre-existing row (INFY, id=1) carries an entry trigger of 1920.50 while INFY's latest daily close is 992.00 and its
-- 30-day high is 1156.00: it cannot have come from the screener on real data. Invalidate it (status only; decision fields untouched).
UPDATE paper_observations
   SET status = 'INVALIDATED',
       status_reason = 'P1.7: entry_trigger 1920.50 is inconsistent with INFY market data (last daily close 992.00 on 2026-10-07, 2026-09 high 1156.00); candidate not reproducible from stored data; excluded from all evidence.',
       outcome = 'INVALIDATED', is_complete = true, completed_at = NOW()
 WHERE id = 1 AND status = 'PENDING';

-- Backfill snapshot for every row (hash covers all decision-time inputs)
UPDATE paper_observations SET
  decision_snapshot = jsonb_build_object(
     'symbol_id', symbol_id, 'symbol', symbol, 'setup_type', setup_type,
     'candidate_timestamp', candidate_timestamp, 'signal_timestamp', signal_timestamp, 'data_cutoff_at', data_cutoff_at,
     'provider', provider, 'rule_version', rule_version, 'entry_trigger_price', entry_trigger_price,
     'invalidation_price', invalidation_price, 'target_price', target_price, 'shariah_status', shariah_status,
     'sector', sector, 'liquidity_bucket', liquidity_bucket, 'market_regime', market_regime)
WHERE decision_snapshot IS NULL;
UPDATE paper_observations SET decision_snapshot_sha256 = encode(sha256(convert_to(decision_snapshot::text,'UTF8')),'hex') WHERE decision_snapshot_sha256 IS NULL;

ALTER TABLE paper_observations
  ADD CONSTRAINT ck_po_status CHECK (status IN ('PENDING','COMPLETED','CENSORED','INVALIDATED','DATA_UNAVAILABLE')),
  ADD CONSTRAINT ck_po_outcome CHECK (outcome IN ('PENDING','TARGET_HIT','STOPPED_OUT','EXPIRED_TIME_STOP','NOT_TRIGGERED','CENSORED','DATA_UNAVAILABLE','INVALIDATED')),
  ADD CONSTRAINT ck_po_shariah_pass CHECK (shariah_status = 'PASS'),
  ADD CONSTRAINT ck_po_time_order CHECK (data_cutoff_at <= signal_timestamp AND signal_timestamp <= candidate_timestamp),
  ADD CONSTRAINT ck_po_levels CHECK (invalidation_price < entry_trigger_price AND entry_trigger_price < target_price),
  ADD CONSTRAINT ck_po_snapshot_present CHECK (decision_snapshot IS NOT NULL AND decision_snapshot_sha256 IS NOT NULL),
  ADD CONSTRAINT ck_po_terminal_consistency CHECK ((status = 'PENDING' AND NOT is_complete) OR (status <> 'PENDING' AND is_complete));

-- Guard: decision-time columns are immutable; terminal rows are fully frozen; no deletes of anything but PENDING-never-evaluated rows.
CREATE OR REPLACE FUNCTION po_guard() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'paper_observations rows may not be deleted (id=%)', OLD.id;
  END IF;
  IF (OLD.symbol_id, OLD.symbol, OLD.setup_type, OLD.candidate_timestamp, OLD.signal_timestamp, OLD.data_cutoff_at, OLD.provider,
      OLD.rule_version, OLD.entry_trigger_price, OLD.invalidation_price, OLD.target_price, OLD.risk_reward_ratio, OLD.shariah_status,
      OLD.sector, OLD.liquidity_bucket, OLD.market_regime, OLD.decision_snapshot::text, OLD.decision_snapshot_sha256, OLD.created_at,
      OLD.estimated_costs_bps)
     IS DISTINCT FROM
     (NEW.symbol_id, NEW.symbol, NEW.setup_type, NEW.candidate_timestamp, NEW.signal_timestamp, NEW.data_cutoff_at, NEW.provider,
      NEW.rule_version, NEW.entry_trigger_price, NEW.invalidation_price, NEW.target_price, NEW.risk_reward_ratio, NEW.shariah_status,
      NEW.sector, NEW.liquidity_bucket, NEW.market_regime, NEW.decision_snapshot::text, NEW.decision_snapshot_sha256, NEW.created_at,
      NEW.estimated_costs_bps) THEN
    RAISE EXCEPTION 'decision-time snapshot of paper_observations id=% is immutable', OLD.id;
  END IF;
  IF OLD.status <> 'PENDING' THEN
    RAISE EXCEPTION 'paper_observations id=% is terminal (status=%) and cannot be modified', OLD.id, OLD.status;
  END IF;
  NEW.updated_at := NOW();
  RETURN NEW;
END $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_po_guard ON paper_observations;
CREATE TRIGGER trg_po_guard BEFORE UPDATE OR DELETE ON paper_observations FOR EACH ROW EXECUTE FUNCTION po_guard();

COMMIT;
