"""Shared invariant for the corporate-action evidence table after P1.9.24.

Earlier phases locked the table at exactly 23 artifacts to catch test fixtures or forged evidence leaking into the
application database. P1.9.24 added legitimate operator evidence (NSE feed snapshots and company filings). The
invariant keeps its intent: the baseline is still exactly 23, and every additional artifact is exchange-hosted,
checksummed and NOT promoted (AUTHENTICITY_PENDING).
"""

BASELINE_FILTER = "parser_version NOT LIKE 'p1.9.24%%'"


def assert_artifact_baseline_and_new_evidence(cur):
    cur.execute(f"SELECT count(*), count(DISTINCT document_identifier) FROM corporate_action_source_artifacts WHERE {BASELINE_FILTER}")
    total, distinct = cur.fetchone()
    assert total == 23, f"baseline artifacts must remain exactly 23, got {total}"
    assert distinct == 23
    cur.execute(f"""
        SELECT count(*) FROM corporate_action_source_artifacts
        WHERE NOT ({BASELINE_FILTER}) AND NOT (
            validation_status = 'AUTHENTICITY_PENDING'
            AND document_type IN ('EXCHANGE_API_JSON', 'STATUTORY_FILING')
            AND issuing_authority = 'NSE_INDIA'
            AND source_url ~ '^https://(www|nsearchives)\\.nseindia\\.com/'
            AND sha256_checksum ~ '^[0-9a-f]{{64}}$')""")
    assert cur.fetchone()[0] == 0, "every non-baseline artifact must be unpromoted, checksummed, exchange-hosted evidence"
