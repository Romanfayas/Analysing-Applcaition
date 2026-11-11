-- P1.9.30: official NSE bulk and block deals (client name, side, quantity, price), kept per stock so large buyers can be followed.
-- Additive only. The NSE files carry only the latest trading day, so history builds up from the day this runs.
CREATE TABLE IF NOT EXISTS large_deals (
    id          bigserial PRIMARY KEY,
    trade_date  date          NOT NULL,
    symbol_id   integer       NOT NULL REFERENCES symbols(id),
    kind        text          NOT NULL CHECK (kind IN ('BULK', 'BLOCK')),
    client_name text          NOT NULL,
    side        text          NOT NULL CHECK (side IN ('BUY', 'SELL')),
    quantity    bigint        NOT NULL CHECK (quantity > 0),
    price       numeric(18,4) NOT NULL CHECK (price > 0),
    value_inr   numeric(22,2) NOT NULL,
    remarks     text,
    client_type text          NOT NULL DEFAULT 'OTHER',
    source      text          NOT NULL DEFAULT 'NSE_ARCHIVES',
    fetched_at  timestamptz   NOT NULL DEFAULT now(),
    UNIQUE (trade_date, symbol_id, kind, client_name, side, quantity, price)
);
CREATE INDEX IF NOT EXISTS idx_large_deals_symbol ON large_deals (symbol_id, trade_date DESC);
CREATE INDEX IF NOT EXISTS idx_large_deals_date ON large_deals (trade_date DESC);
GRANT SELECT, INSERT ON large_deals TO halal_app_user;
GRANT USAGE ON SEQUENCE large_deals_id_seq TO halal_app_user;
