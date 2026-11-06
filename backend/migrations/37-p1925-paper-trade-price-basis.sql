-- HalalEquity Migration 37: P1.9.25 paper-trade price basis and execution time
--
-- Paper trades can now be executed "as of" a past date/time (replay against stored history). Every trade records
-- where its fill price came from, so a backdated test can never be mistaken for a live-priced one:
--   price_source    : e.g. INTRADAY_1M, INTRADAY_5M, EXCHANGE_DAILY_CLOSE, LIVE_LATEST
--   price_bar_time  : start of the bar the price was taken from (NULL for daily closes)
--   is_backdated    : true when the requested execution time was in the past
-- trade_date already holds the (simulated) execution time. Existing rows keep NULL / false.

BEGIN;
ALTER TABLE portfolio_trades ADD COLUMN IF NOT EXISTS price_source   TEXT;
ALTER TABLE portfolio_trades ADD COLUMN IF NOT EXISTS price_bar_time TIMESTAMPTZ;
ALTER TABLE portfolio_trades ADD COLUMN IF NOT EXISTS is_backdated   BOOLEAN NOT NULL DEFAULT FALSE;
COMMIT;
