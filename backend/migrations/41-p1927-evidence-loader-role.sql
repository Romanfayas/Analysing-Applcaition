-- P1.9.27: the worker loads NSE's daily bhavcopy automatically. The runtime role must stay unable to write evidence, so
-- this login can do exactly one thing: append exchange files and their bars. It cannot update, delete or read users.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'halal_evidence_loader') THEN
        CREATE ROLE halal_evidence_loader WITH LOGIN PASSWORD 'evidence_password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
    END IF;
END $$;
GRANT USAGE ON SCHEMA public TO halal_evidence_loader;
GRANT SELECT ON symbols TO halal_evidence_loader;
GRANT SELECT, INSERT ON exchange_source_files, exchange_daily_bars TO halal_evidence_loader;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO halal_evidence_loader;
