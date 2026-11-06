-- HalalEquity Migration 36: P1.9.24 record Shariah status on every paper trade
--
-- The owner removed the Shariah BUY restriction for PAPER trading (configurable: PAPER_SHARIAH_GATE_ENABLED,
-- default off). So that removing the block never hides what was traded, each paper trade now records the
-- instrument's Shariah screening status at execution time. Trades executed before this change keep NULL.
-- Live/broker trading remains disabled; the prediction engine's Shariah rule is unchanged.

BEGIN;
ALTER TABLE portfolio_trades ADD COLUMN IF NOT EXISTS shariah_status_at_trade TEXT;
COMMIT;
