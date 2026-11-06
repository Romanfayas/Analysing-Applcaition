-- P1.9.27: everything known about an issue beyond the headline row. `data` keeps official (NSE) and unofficial (IPOWatch)
-- facts apart, so the application can always say where a number came from.
CREATE TABLE IF NOT EXISTS ipo_details (
    ipo_id     integer PRIMARY KEY REFERENCES ipos(id) ON DELETE CASCADE,
    data       jsonb       NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE ON ipo_details TO halal_app_user;
