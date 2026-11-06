-- P1.9.25: owner-added stocks. The runtime role may register a NEW symbol and fill in its descriptive fields,
-- but cannot rewrite or deactivate the canonical universe (no UPDATE on symbol, name, exchange, is_active).
GRANT INSERT ON symbols TO halal_app_user;
GRANT UPDATE (sector, industry, metadata, updated_at) ON symbols TO halal_app_user;
GRANT USAGE ON SEQUENCE symbols_id_seq TO halal_app_user;
