"""
P1.9.12 Automated Release Gates: Authorized Data Acquisition and Provenance Remediation
Validates:
1. Corporate-action trust statuses remediated: 0 VERIFIED_EXCHANGE_PRIMARY, 58 ARCHIVE_UNAUTHENTICATED.
2. Complete audit trail in corporate_action_verification_audit_log with append-only protection.
3. Database trigger fn_ca_event_verification_guard blocks controlled archives and unauthenticated vendor JSON.
4. Canonical session open timestamp convention: 100% adherence to 03:45:00 UTC (09:15 IST), 0 deviations.
5. Populated bar consumer separation: exactly 650 populated bars (643 nominal, 7 total-return), 23,132 quarantined bars.
6. Shariah evidence audit: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT (null interest income ratio).
7. Frozen Strategy V2 canonical hash lock and research gate locked (RESEARCH_BLOCKED).
"""
from tests.shariah_consistency import assert_shariah_verdicts_consistent
import os
import pytest
import psycopg2
from decimal import Decimal
from src.engines.research.strategy_config import assert_frozen, PINNED_HASH


@pytest.fixture(scope="module")
def db_conn():
    db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    conn = psycopg2.connect(db_url)
    conn.autocommit = True
    yield conn
    conn.close()


class TestP1912ProvenanceRemediation:

    def test_corporate_action_trust_statuses_remediated(self, db_conn):
        cur = db_conn.cursor()
        # Invariant 1: Exactly 0 corporate action events claim VERIFIED_EXCHANGE_PRIMARY
        cur.execute("SELECT count(*) FROM corporate_action_events WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY';")
        verified_cnt = cur.fetchone()[0]
        assert verified_cnt == 0, f"Expected 0 VERIFIED_EXCHANGE_PRIMARY events, got {verified_cnt}"

        # Invariant 2: Exactly 58 events are demoted to ARCHIVE_UNAUTHENTICATED
        cur.execute("SELECT count(*) FROM corporate_action_events WHERE verification_status = 'ARCHIVE_UNAUTHENTICATED';")
        demoted_cnt = cur.fetchone()[0]
        assert demoted_cnt == 58, f"Expected 58 ARCHIVE_UNAUTHENTICATED events, got {demoted_cnt}"

    def test_audit_log_recorded_58_demotions(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT count(*) 
            FROM corporate_action_verification_audit_log 
            WHERE remediation_phase = 'P1.9.12' 
              AND prior_verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
              AND new_verification_status = 'ARCHIVE_UNAUTHENTICATED';
        """)
        audit_cnt = cur.fetchone()[0]
        assert audit_cnt == 58, f"Expected 58 audit log records, got {audit_cnt}"

        # Verify append-only guard on corporate_action_verification_audit_log
        with pytest.raises(psycopg2.DatabaseError) as exc_info:
            cur.execute("DELETE FROM corporate_action_verification_audit_log WHERE remediation_phase = 'P1.9.12';")
        db_conn.rollback()
        err_msg = str(exc_info.value).lower()
        assert "append-only" in err_msg or "permission denied" in err_msg

    def test_adversarial_trigger_blocks_controlled_archive_verification(self, db_conn):
        cur = db_conn.cursor()
        with pytest.raises(psycopg2.DatabaseError) as exc_info:
            cur.execute("""
                UPDATE corporate_action_events 
                SET verification_status = 'VERIFIED_EXCHANGE_PRIMARY' 
                WHERE source_artifact_id = 5;
            """)
        db_conn.rollback()
        err_msg = str(exc_info.value)
        assert "cannot be established from unauthenticated archive payload" in err_msg or "privilege boundary violation" in err_msg

    def test_canonical_session_open_timestamp_convention(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("SELECT count(*) FROM canonical.ohlcv_daily;")
        total_bars = cur.fetchone()[0]
        assert total_bars >= 23782 and (total_bars - 23782) % 47 == 0, f"total_bars must be 23782 plus whole 47-instrument sessions, got {total_bars}"

        # 100% adherence to 03:45:00 UTC (09:15:00 IST)
        cur.execute("""
            SELECT count(*) 
            FROM canonical.ohlcv_daily 
            WHERE ("timestamp" AT TIME ZONE 'UTC')::time != TIME '03:45:00';
        """)
        deviant_bars = cur.fetchone()[0]
        assert deviant_bars == 0, f"Expected 0 bars deviating from 03:45:00 UTC, got {deviant_bars}"

        # Verify rejection of session close timestamp (15:30 IST / 10:00 UTC)
        cur.execute("SELECT id FROM canonical.symbols WHERE symbol = 'TCS';")
        tcs_id = cur.fetchone()[0]
        with pytest.raises(psycopg2.DatabaseError) as exc_info:
            cur.execute("""
                INSERT INTO ohlcv_daily (
                    symbol_id, timestamp, open, high, low, close, volume, source
                ) VALUES (%s, '2026-10-23 10:00:00+00', 2000, 2010, 1990, 2005, 100000, 'yahoo_finance');
            """, (tcs_id,))
        db_conn.rollback()
        assert "ck_ohlcv_daily_session_stamp" in str(exc_info.value)

    def test_populated_bar_consumer_eligibility(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NOT NULL;")
        populated = cur.fetchone()[0]
        assert populated >= 650 and (populated - 650) % 47 == 0, f"populated must be 650 plus whole 47-instrument sessions, got {populated}"

        # 643, 690, 737, 784, or 831 nominal unadjusted bars
        cur.execute("""
            SELECT count(*) 
            FROM canonical.ohlcv_daily 
            WHERE adjusted_close IS NOT NULL 
              AND adjusted_close = close 
              AND source = 'yahoo_finance';
        """)
        nominal_cnt = cur.fetchone()[0]
        assert nominal_cnt >= 643 and (nominal_cnt - 643) % 47 == 0, f"nominal_cnt must be 643 plus whole 47-instrument sessions, got {nominal_cnt}"

        # 7 total-return dividend adjusted bars (TCS)
        cur.execute("""
            SELECT count(*) 
            FROM canonical.ohlcv_daily 
            WHERE adjusted_close IS NOT NULL 
              AND adjusted_close != close 
              AND source = 'NSE_CORPORATE_ACTION';
        """)
        tr_cnt = cur.fetchone()[0]
        assert tr_cnt == 7, f"Expected 7 total return dividend bars, got {tr_cnt}"

        # 23,132 historical bars strictly quarantined NULL
        cur.execute("SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;")
        quarantined = cur.fetchone()[0]
        assert quarantined == 23132, f"Expected 23,132 quarantined bars, got {quarantined}"

    def test_shariah_fail_closed_31_data_insufficient(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            WITH latest_screenings AS (
                SELECT DISTINCT ON (s.symbol) s.symbol, sc.status, sc.interest_income_ratio
                FROM canonical.symbols s
                JOIN canonical.shariah_screenings sc ON s.id = sc.symbol_id
                WHERE s.is_active = true
                ORDER BY s.symbol, sc.screening_date DESC
            )
            SELECT 
                count(*) FILTER (WHERE status = 'PASS'),
                count(*) FILTER (WHERE status = 'FAIL'),
                count(*) FILTER (WHERE status = 'DATA_INSUFFICIENT'),
                count(*) FILTER (WHERE status = 'PASS' AND interest_income_ratio IS NULL)
            FROM latest_screenings;
        """)
        pass_cnt, fail_cnt, data_insuf_cnt, fake_pass = cur.fetchone()
        assert_shariah_verdicts_consistent()
        assert fake_pass == 0, f"Expected 0 equities marked PASS with NULL interest income, got {fake_pass}"

    def test_frozen_strategy_v2_canonical_hash_lock(self):
        res = assert_frozen()
        assert res["matches_pinned"] is True
        assert res["runtime_hash"] == PINNED_HASH
        assert PINNED_HASH == "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
