-- HalalEquity Migration 35: P1.9.24 primary-document checks for corporate actions
--
-- Records what was established about each primary document (a company's statutory filing to the exchange):
-- PDF signature integrity over the document bytes, the signer certificate, and whether the document's text
-- contains the action's ratio and dates. A download, HTTPS connection or checksum is NOT proof of authenticity,
-- and neither is an intact signature on its own: the signer chain is not validated against an independently
-- obtained trust anchor and revocation is not checked. The verdict therefore never says "verified"; promotion
-- of any corporate-action event remains a human reviewer's decision. Append-only; operator-written.

BEGIN;

CREATE TABLE IF NOT EXISTS primary_document_checks (
    id                        BIGSERIAL PRIMARY KEY,
    artifact_id               INTEGER     NOT NULL REFERENCES corporate_action_source_artifacts(id),
    symbol_id                 INTEGER     NOT NULL REFERENCES symbols(id),
    action_class              TEXT        NOT NULL,
    ex_date                   DATE        NOT NULL,
    filing_date               TEXT,
    filing_description        TEXT,
    signature_count           INTEGER     NOT NULL,
    all_signatures_intact     BOOLEAN     NOT NULL,
    final_signature_covers_document BOOLEAN NOT NULL,
    signer_subject            TEXT,
    signer_organisation       TEXT,
    chain_root_subject        TEXT,
    signing_time              TEXT,
    chain_validated           BOOLEAN     NOT NULL DEFAULT FALSE CHECK (chain_validated = FALSE),
    revocation_checked        BOOLEAN     NOT NULL DEFAULT FALSE CHECK (revocation_checked = FALSE),
    content_checks            JSONB       NOT NULL,
    verdict                   TEXT        NOT NULL CHECK (verdict IN (
                                  'SIGNATURE_INTACT_CONTENT_MATCHED_PENDING_REVIEW',
                                  'SIGNATURE_INTACT_CONTENT_UNCONFIRMED_PENDING_REVIEW',
                                  'UNSIGNED_PENDING_REVIEW',
                                  'SIGNATURE_INVALID',
                                  'UNREADABLE')),
    checked_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    checked_by                TEXT        NOT NULL DEFAULT current_user
);
CREATE INDEX IF NOT EXISTS idx_primary_doc_checks_symbol ON primary_document_checks(symbol_id, ex_date);

DROP TRIGGER IF EXISTS trg_primary_document_checks_append_only ON primary_document_checks;
CREATE TRIGGER trg_primary_document_checks_append_only BEFORE UPDATE OR DELETE ON primary_document_checks
    FOR EACH ROW EXECUTE FUNCTION fn_evidence_append_only();

GRANT SELECT ON primary_document_checks TO halal_app_user;

COMMIT;
