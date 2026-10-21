-- P1.8.2: Ensure ipos table has shariah_status column for IPO worker ingestion
ALTER TABLE ipos ADD COLUMN IF NOT EXISTS shariah_status VARCHAR(20) DEFAULT 'DATA_UNAVAILABLE';
SELECT setval('ipos_id_seq', (SELECT COALESCE(max(id), 1) FROM ipos));

