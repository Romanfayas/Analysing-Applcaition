"""
HalalEquity Comprehensive Remediation & Release-Readiness Pytest Suite

Validates end-to-end evidence trust, database security boundaries,
universe lineage reconciliation, historical bar classification, look-ahead safety,
Shariah fail-closed screening, and platform research gate lock.
"""

import os
import pytest
import psycopg2
from psycopg2 import sql


@pytest.fixture(scope="module")
def db_conn():
    conn_str = os.environ.get(
        "ADMIN_DATABASE_URL",
        "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable"
    )
    conn = psycopg2.connect(conn_str)
    yield conn
    conn.close()


@pytest.fixture(scope="module")
def app_conn():
    conn_str = os.environ.get(
        "DATABASE_URL",
        "postgres://halal_app_user:app_password@postgres:5432/halal_equity?sslmode=disable"
    )
    conn = psycopg2.connect(conn_str)
    yield conn
    conn.close()


class TestComprehensiveRemediationReleaseGate:

    def test_comprehensive_explicit_evidence_taxonomy_and_transitions(self, db_conn, app_conn):
        """Invariant 1: Check constraints reject invalid statuses; transition guard blocks unlawful promotion."""
        cur = db_conn.cursor()

        # 1. Invalid verification_status on corporate_action_events rejected by check constraint
        with pytest.raises(psycopg2.IntegrityError) as excinfo:
            cur.execute("""
                INSERT INTO corporate_action_events (
                    event_id, symbol_id, symbol, action_type, source_subject, ex_date, currency, verification_status, coverage_status
                ) VALUES (
                    'TEST:PY:INVALID:STATUS', 1, 'DRREDDY', 'CASH_DIVIDEND', 'Test', '2026-11-01', 'INR', 'FORGED_STATUS', 'PARSED_EVENT'
                );
            """)
        db_conn.rollback()
        assert "ck_ca_event_verification_status" in str(excinfo.value)

        # 2. Application role cannot promote event to PRIMARY_SOURCE_AUTHENTICATED
        app_cur = app_conn.cursor()
        with pytest.raises(psycopg2.DatabaseError) as excinfo:
            app_cur.execute("""
                UPDATE corporate_action_events
                SET verification_status = 'PRIMARY_SOURCE_AUTHENTICATED'
                WHERE event_id = 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1';
            """)
        app_conn.rollback()
        assert "privilege boundary violation" in str(excinfo.value)

    def test_comprehensive_database_privilege_boundaries(self, app_conn):
        """Invariant 2: Application role is strictly blocked from DELETE on artifacts and audit log."""
        app_cur = app_conn.cursor()

        # DELETE on corporate_action_source_artifacts prohibited
        with pytest.raises(psycopg2.ProgrammingError) as excinfo:
            app_cur.execute("DELETE FROM corporate_action_source_artifacts WHERE id = 1;")
        app_conn.rollback()
        assert "permission denied" in str(excinfo.value).lower()

        # DELETE on corporate_action_verification_audit_log prohibited
        with pytest.raises(psycopg2.ProgrammingError) as excinfo:
            app_cur.execute("DELETE FROM corporate_action_verification_audit_log WHERE id = 1;")
        app_conn.rollback()
        assert "permission denied" in str(excinfo.value).lower()

    def test_comprehensive_universe_and_sector_completeness(self, db_conn):
        """Invariant 3: Exactly 52 catalog symbols, 51 active, 0 symbols with empty sector."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*),
                count(*) FILTER (WHERE is_active = true),
                count(*) FILTER (WHERE is_active = false),
                count(*) FILTER (WHERE sector IS NULL OR TRIM(sector) = '')
            FROM symbols;
        """)
        total, active, archived, empty_sectors = cur.fetchone()

        assert total == 52, f"Total catalog symbols must be 52, got {total}"
        assert active == 51, f"Active symbols must be 51, got {active}"
        assert archived == 1, f"Archived symbols must be 1, got {archived}"
        assert empty_sectors == 0, f"All 52 symbols must have explicit sector, got {empty_sectors} empty"

    def test_comprehensive_canonical_instrument_lineage(self, db_conn):
        """Invariant 4: Canonical lineage table records demergers and renames as non-price adjustments."""
        cur = db_conn.cursor()
        cur.execute("SELECT count(*) FROM instrument_lineage;")
        lineage_count = cur.fetchone()[0]
        assert lineage_count == 3, f"Expected 3 lineage records, got {lineage_count}"

        cur.execute("SELECT count(*) FROM instrument_lineage WHERE is_price_adjustment = true;")
        price_adj_count = cur.fetchone()[0]
        assert price_adj_count == 0, "All lineage records must be is_price_adjustment = false"

        cur.execute("""
            SELECT successor_symbol, allocation_ratio
            FROM instrument_lineage
            WHERE predecessor_symbol = 'TATAMOTORS'
            ORDER BY successor_symbol;
        """)
        ratios = dict(cur.fetchall())
        assert abs(float(ratios["TMCV"]) - 0.3115) < 0.0001
        assert abs(float(ratios["TMPV"]) - 0.6885) < 0.0001

    def test_comprehensive_ohlcv_quarantine_and_nominal_isolation(self, db_conn):
        """Invariant 5: 23,132 quarantined bars, 643 nominal Yahoo bars, 7 TCS bars, 0 backtest eligible."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*),
                count(*) FILTER (WHERE adjusted_close IS NULL),
                count(*) FILTER (WHERE adjusted_close IS NOT NULL AND source = 'yahoo_finance'),
                count(*) FILTER (WHERE adjusted_close IS NOT NULL AND source = 'NSE_CORPORATE_ACTION')
            FROM ohlcv_daily;
        """)
        total, quarantined, yahoo_nominal, tcs_adjusted = cur.fetchone()

        assert total == 23782
        assert quarantined == 23132, "23,132 historical bars must remain quarantined"
        assert yahoo_nominal == 643, "643 Yahoo Finance bars are nominal forward execution series"
        assert tcs_adjusted == 7, "7 TCS bars are diagnostic adjustments"

        # Zero events possess verified exchange primary authentication
        cur.execute("""
            SELECT count(*) 
            FROM corporate_action_events 
            WHERE verification_status IN ('VERIFIED_EXCHANGE_PRIMARY', 'PRIMARY_SOURCE_AUTHENTICATED');
        """)
        verified_events = cur.fetchone()[0]
        assert verified_events == 0, "Zero events may possess primary exchange verification"

    def test_comprehensive_lookahead_leakage_and_session_safety(self, db_conn):
        """Invariant 6: Session open timestamp convention 03:45 UTC strictly adhered to."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT count(*) 
            FROM ohlcv_daily 
            WHERE timestamp::time <> '03:45:00';
        """)
        mismatched_timestamps = cur.fetchone()[0]
        assert mismatched_timestamps == 0, "All daily rows must have 03:45:00 UTC session open timestamp"

    def test_comprehensive_shariah_screening_distribution(self, db_conn):
        """Invariant 7: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT strictly maintained."""
        cur = db_conn.cursor()
        cur.execute("""
            WITH latest_screenings AS (
                SELECT DISTINCT ON (symbol_id) symbol_id, status
                FROM shariah_screenings
                ORDER BY symbol_id, screening_date DESC, id DESC
            )
            SELECT 
                count(*) FILTER (WHERE s.status = 'PASS'),
                count(*) FILTER (WHERE s.status = 'FAIL'),
                count(*) FILTER (WHERE s.status = 'DATA_INSUFFICIENT')
            FROM symbols sym
            JOIN latest_screenings s ON sym.id = s.symbol_id
            WHERE sym.is_active = true;
        """)
        pass_cnt, fail_cnt, data_insuff_cnt = cur.fetchone()

        assert pass_cnt == 0, "0 active equities may have PASS status without Note 24/25 footnote disclosures"
        assert fail_cnt == 20, "20 active equities must be certified FAIL"
        assert data_insuff_cnt == 31, "31 active equities must be certified DATA_INSUFFICIENT"

    def test_comprehensive_strategy_v2_canonical_lock_and_gate_status(self):
        """Invariant 8: Strategy V2 hash locked, canonical serialization verified, and research gate status is RESEARCH_BLOCKED."""
        import json
        import hashlib
        import pytest
        from src.engines.research.strategy_config import (
            compute_hash,
            canonical_json,
            FROZEN,
            PINNED_HASH,
            runtime_config,
            enforce_request,
            StrategyDrift
        )

        canonical_hash = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
        assert PINNED_HASH == canonical_hash

        # 1. Verify exact canonical parameters and canonical serialization
        assert FROZEN == {"BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 2.0}
        expected_json = '{"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}'
        assert canonical_json(FROZEN) == expected_json

        # 2. Recompute hash directly
        direct_hash = hashlib.sha256(expected_json.encode("utf-8")).hexdigest()
        assert direct_hash == canonical_hash
        assert compute_hash(FROZEN) == canonical_hash

        # 3. Verify runtime configuration introspection
        cfg = runtime_config()
        assert cfg["matches_pinned"] is True
        assert cfg["runtime_hash"] == canonical_hash
        assert not cfg["problems"], f"Runtime problems detected: {cfg['problems']}"

        # 4. Negative test: changing ATR_MULTIPLIER to 1.50 alters hash
        drifted_params = {"BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 1.5}
        drifted_hash = compute_hash(drifted_params)
        assert drifted_hash != canonical_hash

        # 5. Negative test: enforce_request rejects drifted parameters
        with pytest.raises(StrategyDrift) as excinfo:
            enforce_request(buy_threshold=80.0, sell_threshold=45.0, atr_multiplier=1.50)
        assert "differ from frozen Strategy V2" in str(excinfo.value)

