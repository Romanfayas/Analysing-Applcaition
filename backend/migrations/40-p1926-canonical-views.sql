-- P1.9.26: owner-added stocks live alongside the canonical universe. The release-gate invariants (52 instruments,
-- 23,132 quarantined bars, ...) describe the CANONICAL universe only, so the test runners put this schema first on the
-- search path. Application code never uses it: the app sees every symbol, including added ones.
CREATE SCHEMA IF NOT EXISTS canonical;

CREATE OR REPLACE VIEW canonical.symbols AS
    SELECT * FROM public.symbols WHERE COALESCE(metadata->>'user_added', 'false') <> 'true';

CREATE OR REPLACE VIEW canonical.ohlcv_daily AS
    SELECT o.* FROM public.ohlcv_daily o
    WHERE NOT EXISTS (SELECT 1 FROM public.symbols s WHERE s.id = o.symbol_id AND COALESCE(s.metadata->>'user_added', 'false') = 'true');

CREATE OR REPLACE VIEW canonical.shariah_screenings AS
    SELECT c.* FROM public.shariah_screenings c
    WHERE NOT EXISTS (SELECT 1 FROM public.symbols s WHERE s.id = c.symbol_id AND COALESCE(s.metadata->>'user_added', 'false') = 'true');

GRANT USAGE ON SCHEMA canonical TO PUBLIC;
GRANT SELECT ON ALL TABLES IN SCHEMA canonical TO PUBLIC;
