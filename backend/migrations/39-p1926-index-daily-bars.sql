-- P1.9.26: official NSE daily index closes (NIFTY 50/500, sector indices). Used as benchmarks and for valuation context.
-- Context data only: nothing here is evidence for corporate actions or Shariah, so the runtime role may write it.
CREATE TABLE IF NOT EXISTS index_daily_bars (
    index_name    text          NOT NULL,
    trade_date    date          NOT NULL,
    open          numeric(18,4),
    high          numeric(18,4),
    low           numeric(18,4),
    close         numeric(18,4) NOT NULL CHECK (close > 0),
    points_change numeric(18,4),
    volume        bigint,
    turnover_cr   numeric(24,2),
    pe            numeric(10,2),
    pb            numeric(10,2),
    div_yield     numeric(10,2),
    source_url    text          NOT NULL,
    source_sha256 text          NOT NULL,
    retrieved_at  timestamptz   NOT NULL DEFAULT now(),
    PRIMARY KEY (index_name, trade_date)
);
CREATE INDEX IF NOT EXISTS idx_index_daily_bars_date ON index_daily_bars (trade_date);
GRANT SELECT, INSERT, UPDATE ON index_daily_bars TO halal_app_user;
