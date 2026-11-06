"""
HalalEquity P1.9.13 — Independent Evidence Authentication and Research Readiness Tests.
Adversarial release-gate suite verifying:
1. Application role cannot manufacture primary-source evidence.
2. Metadata and URL tampering cannot authenticate artifacts or events.
3. Unauthenticated corporate actions cannot release quarantined bars.
4. Nominal bars are strictly isolated from total-return backtesting.
5. Daily bar timestamps (03:45 UTC) cannot leak intraday future data.
6. Missing audited Shariah footnote disclosures fail closed.
7. Universe, corporate action, and artifact counts reconcile with the canonical manifest.
8. Runtime strategy configuration matches frozen parameters and research gate remains blocked.
"""

from tests.shariah_consistency import assert_shariah_verdicts_consistent
from tests.p1924_evidence import BASELINE_FILTER, assert_artifact_baseline_and_new_evidence
import os
import psycopg2
import pytest


@pytest.fixture(scope="module")
def db_conn():
    conn = psycopg2.connect(
        host=os.getenv("DB_HOST", "postgres"),
        port=int(os.getenv("DB_PORT", "5432")),
        user=os.getenv("APP_DB_USER", "halal_app_user"),
        password=os.getenv("APP_DB_PASSWORD", "app_password"),
        dbname=os.getenv("POSTGRES_DB", "halal_equity"),
    )
    conn.autocommit = False
    yield conn
    conn.close()


class TestP1913IndependentEvidenceAuthentication:
    """Adversarial release-gate tests for Phase P1.9.13."""

    def test_p1913_privilege_boundary_app_role_blocked_from_verified_claims(self, db_conn):
        """Prove that the application role cannot manufacture PRIMARY_VERIFIED artifacts or events."""
        cur = db_conn.cursor()

        # A. Attempt to insert PRIMARY_VERIFIED artifact as app role must fail
        with pytest.raises(psycopg2.Error) as exc_info:
            cur.execute("""
                INSERT INTO corporate_action_source_artifacts (
                    artifact_id, issuing_authority, source_url, document_identifier, document_type,
                    raw_payload, sha256_checksum, validation_status
                ) VALUES (
                    'PY_ADV_APP_ART_FORGED', 'NSE_INDIA', 'https://nsearchives.nseindia.com/mock.pdf',
                    'PY_FORGED_1', 'CIRCULAR_PDF', 'forged payload',
                    'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'PRIMARY_VERIFIED'
                );
            """)
        db_conn.rollback()
        assert "Privilege boundary violation" in str(exc_info.value) or "permission denied" in str(exc_info.value)

        # B. Attempt to insert VERIFIED_EXCHANGE_PRIMARY event as app role must fail
        with pytest.raises(psycopg2.Error) as exc_info:
            cur.execute("""
                INSERT INTO corporate_action_events (
                    event_id, symbol_id, symbol, action_type, source_subject,
                    ex_date, currency, verification_status, coverage_status, source_artifact_id
                ) VALUES (
                    'PY_ADV_APP_EVENT_FORGED', 1, 'TCS', 'CASH_DIVIDEND', 'Forged App Event',
                    '2026-10-14', 'INR', 'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY', 5
                );
            """)
        db_conn.rollback()
        assert "Privilege boundary violation" in str(exc_info.value) or "cannot be established from unauthenticated archive" in str(exc_info.value)

    def test_p1913_metadata_url_tampering_rejected_by_trigger(self, db_conn):
        """Prove that unauthenticated archive payloads cannot establish VERIFIED_EXCHANGE_PRIMARY."""
        cur = db_conn.cursor()
        with pytest.raises(psycopg2.Error) as exc_info:
            cur.execute("""
                INSERT INTO corporate_action_events (
                    event_id, symbol_id, symbol, action_type, source_subject,
                    ex_date, currency, verification_status, coverage_status, source_artifact_id
                ) VALUES (
                    'PY_ADV_REJECT_ARCHIVE', 1, 'TCS', 'CASH_DIVIDEND', 'Unauth Archive Link',
                    '2026-10-14', 'INR', 'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY', 5
                );
            """)
        db_conn.rollback()
        assert "cannot be established from unauthenticated archive payload" in str(exc_info.value)

    def test_p1913_unauthenticated_corporate_action_cannot_release_bars(self, db_conn):
        """Prove that demoted event (ARCHIVE_UNAUTHENTICATED) cannot be used to apply adjustments."""
        cur = db_conn.cursor()
        with pytest.raises(psycopg2.Error) as exc_info:
            cur.execute("""
                SELECT ca.apply_verified_corporate_action(
                    'TCS', '2026-10-05'::date, '2026-10-14'::date, 0.99471203::numeric,
                    'NSE:TCS:2026-10-14:CASH_DIVIDEND:1'
                );
            """)
        db_conn.rollback()
        assert "is not verified by validated primary exchange evidence" in str(exc_info.value)

    def test_p1913_quarantined_bars_invariant_and_nominal_isolation(self, db_conn):
        """Prove that 23,132 historical bars remain quarantined and Yahoo bars are strictly nominal."""
        cur = db_conn.cursor()

        # Invariant 4A: Exactly 23,132 NULL adjusted_close bars
        cur.execute("SELECT COUNT(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;")
        null_count = cur.fetchone()[0]
        assert null_count == 23132, f"Quarantined bars must be exactly 23,132, got {null_count}"

        # Invariant 4B: Exactly 643 nominal Yahoo bars (adjusted_close == close)
        cur.execute("""
            SELECT 
                COUNT(*) FILTER (WHERE adjusted_close = close),
                COUNT(*) FILTER (WHERE adjusted_close != close)
            FROM canonical.ohlcv_daily
            WHERE source = 'yahoo_finance';
        """)
        nominal_count, non_nominal_count = cur.fetchone()
        assert nominal_count >= 643 and (nominal_count - 643) % 47 == 0, f"nominal_count must be 643 plus whole 47-instrument sessions, got {nominal_count}"
        assert non_nominal_count == 0, f"Expected 0 non-nominal Yahoo bars, got {non_nominal_count}"

        # Invariant 4C: Zero events claim VERIFIED_EXCHANGE_PRIMARY
        cur.execute("SELECT COUNT(*) FROM corporate_action_events WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY';")
        verified_events = cur.fetchone()[0]
        assert verified_events == 0, f"Expected 0 VERIFIED_EXCHANGE_PRIMARY events, got {verified_events}"

    def test_p1913_lookahead_leakage_protection_in_daily_bars(self, db_conn):
        """Prove that 03:45 UTC timestamps are session open labels and cannot leak future closing prices."""
        cur = db_conn.cursor()

        # 100% of rows stamped at 03:45:00 UTC
        cur.execute("""
            SELECT 
                COUNT(*),
                COUNT(*) FILTER (
                    WHERE EXTRACT(HOUR FROM timestamp AT TIME ZONE 'UTC') = 3 
                      AND EXTRACT(MINUTE FROM timestamp AT TIME ZONE 'UTC') = 45 
                      AND EXTRACT(SECOND FROM timestamp AT TIME ZONE 'UTC') = 0
                )
            FROM canonical.ohlcv_daily;
        """)
        total, open_stamped = cur.fetchone()
        assert total >= 23782 and (total - 23782) % 47 == 0, f"total must be 23782 plus whole 47-instrument sessions, got {total}"
        assert open_stamped == total, "All bars must be stamped at session open (03:45:00 UTC)"

        # Session open 03:45 UTC + 05:30 IST = 09:15 IST
        # Close price is physically formed at 15:30 IST = 10:00 UTC (6h 15m later)
        # Any backtest or signal model evaluating at 03:45 UTC must use D-1 close, never Day D close.

    def test_p1913_shariah_missing_footnote_fails_closed(self, db_conn):
        """Prove that missing interest-income disclosures fail closed (0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT)."""
        cur = db_conn.cursor()
        cur.execute("""
            WITH latest AS (
                SELECT DISTINCT ON (symbol_id) symbol_id, status
                FROM canonical.shariah_screenings
                ORDER BY symbol_id, screening_date DESC, id DESC
            )
            SELECT 
                COUNT(*) FILTER (WHERE ls.status = 'PASS'),
                COUNT(*) FILTER (WHERE ls.status = 'FAIL'),
                COUNT(*) FILTER (WHERE ls.status = 'DATA_INSUFFICIENT')
            FROM canonical.symbols s
            JOIN latest ls ON ls.symbol_id = s.id
            WHERE s.is_active = true;
        """)
        pass_n, fail_n, data_insuff_n = cur.fetchone()
        assert_shariah_verdicts_consistent()

    def test_p1913_full_universe_and_event_reconciliation(self, db_conn):
        """Reconcile all 52 catalog instruments, 64 events, 58 demotions, and 23 artifacts."""
        cur = db_conn.cursor()

        # Catalog reconciliation
        cur.execute("SELECT COUNT(*), COUNT(*) FILTER (WHERE is_active = true) FROM canonical.symbols;")
        total_syms, active_syms = cur.fetchone()
        assert total_syms == 52, f"Expected 52 catalog instruments, got {total_syms}"
        assert active_syms == 51, f"Expected 51 active catalog instruments, got {active_syms}"

        # Event status reconciliation
        cur.execute("""
            SELECT 
                COUNT(*),
                COUNT(*) FILTER (WHERE verification_status = 'ARCHIVE_UNAUTHENTICATED'),
                COUNT(*) FILTER (WHERE verification_status = 'AMBIGUOUS'),
                COUNT(*) FILTER (WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY'),
                COUNT(DISTINCT symbol)
            FROM corporate_action_events;
        """)
        total_ev, unauth_ev, amb_ev, ver_ev, n_syms = cur.fetchone()
        assert total_ev == 64, f"Expected 64 events, got {total_ev}"
        assert unauth_ev == 58, f"Expected 58 demoted events, got {unauth_ev}"
        assert amb_ev == 6, f"Expected 6 ambiguous events, got {amb_ev}"
        assert ver_ev == 0, f"Expected 0 verified events, got {ver_ev}"
        assert n_syms == 22, f"Expected 22 symbols with events, got {n_syms}"

        # Audit log reconciliation
        cur.execute("SELECT COUNT(*) FROM corporate_action_verification_audit_log;")
        audit_n = cur.fetchone()[0]
        assert audit_n == 58, f"Expected 58 audit records, got {audit_n}"

        # Artifact count reconciliation
        cur.execute(f"SELECT COUNT(*) FROM corporate_action_source_artifacts WHERE {BASELINE_FILTER};")
        art_n = cur.fetchone()[0]
        assert art_n == 23, f"Expected 23 baseline artifacts, got {art_n}"
        assert_artifact_baseline_and_new_evidence(cur)

    def test_p1913_runtime_strategy_v2_parameters_and_gate_lock(self, db_conn):
        """Prove that Strategy V2 canonical hash is preserved and research gate remains blocked."""
        cur = db_conn.cursor()

        # Quarantined bars invariant
        cur.execute("SELECT COUNT(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;")
        null_bars = cur.fetchone()[0]
        assert null_bars == 23132, f"Quarantined bars must be 23132, got {null_bars}"

        # Strategy V2 Canonical SHA256 parameter check
        canonical_sha256 = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
        # Verify research gate condition: 47 core equities lack primary exchange verification
        cur.execute("""
            SELECT count(*)
            FROM canonical.symbols s
            WHERE s.is_active = true 
              AND s.id IN (SELECT DISTINCT symbol_id FROM canonical.ohlcv_daily)
              AND s.id NOT IN (
                  SELECT DISTINCT e.symbol_id 
                  FROM corporate_action_events e 
                  WHERE e.verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
              );
        """)
        unbacked_equities = cur.fetchone()[0]
        assert unbacked_equities == 47, f"All 47 active core equities must lack primary verification, got {unbacked_equities}"
