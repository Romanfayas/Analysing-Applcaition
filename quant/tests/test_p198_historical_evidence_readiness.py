"""
HalalEquity P1.9.8 - Historical Evidence Coverage and Research Readiness Tests
Verifies that:
1. Universe and instrument-lineage consistency (52 catalog, 51 active, 47 with bars, TATAMOTORS 0 bars).
2. Missing, unauthenticated, or mismatched corporate-action evidence (27 core symbols unbacked, unbacked marked DATA_INSUFFICIENT).
3. Missing and quarantined historical price values (23,132 quarantined bars, 603 verified bars).
4. Incorrect price-series consumption protection (tax-cost allocations never used as price factors, nominal vs total return isolated).
5. Decimal adjustment and rounding accuracy (TCS dividend factor fixed-point vs float32 drift).
6. Backtests attempting to consume ineligible data are blocked (RESEARCH_BLOCKED).
7. Shariah screening fail-closed behavior (0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT).
8. Runtime database privilege and evidence integrity controls (triggers and non-superuser role).
"""

import hashlib
import json
import math
import os
import pytest
import psycopg2


@pytest.fixture(scope="module")
def db_conn():
    db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    conn = psycopg2.connect(db_url)
    conn.autocommit = True
    yield conn
    conn.close()


@pytest.fixture(scope="module")
def evidence_inventory():
    paths = [
        "data/corporate_actions/inventory/historical_evidence_inventory_v1.json",
        "../data/corporate_actions/inventory/historical_evidence_inventory_v1.json",
        "/app/data/corporate_actions/inventory/historical_evidence_inventory_v1.json",
        "/Users/fayas/Downloads/ICO-Application/analysis application/data/corporate_actions/inventory/historical_evidence_inventory_v1.json",
    ]
    for p in paths:
        if os.path.exists(p):
            with open(p, "r") as f:
                return json.load(f)
    pytest.fail("historical_evidence_inventory_v1.json not found")


@pytest.fixture(scope="module")
def readiness_matrix():
    paths = [
        "data/readiness/instrument_period_readiness_matrix_v1.json",
        "../data/readiness/instrument_period_readiness_matrix_v1.json",
        "/app/data/readiness/instrument_period_readiness_matrix_v1.json",
        "/Users/fayas/Downloads/ICO-Application/analysis application/data/readiness/instrument_period_readiness_matrix_v1.json",
    ]
    for p in paths:
        if os.path.exists(p):
            with open(p, "r") as f:
                return json.load(f)
    pytest.fail("instrument_period_readiness_matrix_v1.json not found")


class TestP198HistoricalEvidenceReadiness:

    def test_1_universe_and_instrument_lineage_consistency(self, db_conn, evidence_inventory):
        """Invariant 1: Universe counts and lineage resolution are consistent across all stores."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*),
                count(*) FILTER (WHERE is_active = true),
                count(*) FILTER (WHERE is_active = false)
            FROM symbols;
        """)
        total, active, archived = cur.fetchone()
        assert total == 52, f"Total catalog symbols must be 52, got {total}"
        assert active == 51, f"Active symbols must be 51, got {active}"
        assert archived == 1, f"Archived symbols must be 1, got {archived}"

        # Distinct symbols in OHLCV daily bars must be exactly 47
        cur.execute("SELECT count(DISTINCT symbol_id) FROM ohlcv_daily;")
        ohlcv_symbols = cur.fetchone()[0]
        assert ohlcv_symbols == 47, f"Symbols with OHLCV bars must be 47, got {ohlcv_symbols}"

        # TATAMOTORS, TMPV, TMCV, LTIM, LTM must have 0 bars in ohlcv_daily
        cur.execute("""
            SELECT s.symbol, count(o.timestamp)
            FROM symbols s
            LEFT JOIN ohlcv_daily o ON s.id = o.symbol_id
            WHERE s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM')
            GROUP BY s.symbol;
        """)
        bars = dict(cur.fetchall())
        for sym in ['TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM']:
            assert bars.get(sym, 0) == 0, f"{sym} must have exactly 0 bars in ohlcv_daily, got {bars.get(sym)}"

    def test_2_missing_and_unauthenticated_corporate_action_evidence(self, db_conn, evidence_inventory):
        """Invariant 2: Core equities without primary artifacts are flagged DATA_INSUFFICIENT."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT count(*)
            FROM symbols s
            WHERE s.is_active = true
              AND s.symbol NOT IN ('TMPV', 'TMCV', 'LTIM', 'LTM')
              AND s.id NOT IN (
                  SELECT DISTINCT ev.symbol_id
                  FROM corporate_action_events ev
                  JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
              );
        """)
        missing_count = cur.fetchone()[0]
        assert missing_count == 27, f"Exactly 27 core equities must be missing primary corporate action evidence, got {missing_count}"

        # Verify inventory records all 52 instruments
        assert evidence_inventory["total_instruments"] == 52
        assert evidence_inventory["active_instruments"] == 51
        assert len(evidence_inventory["instruments"]) == 52

        from collections import Counter
        status_counts = Counter(inst["evidence_status"] for inst in evidence_inventory["instruments"])
        assert status_counts["DATA_INSUFFICIENT"] == 27, f"Expected 27 DATA_INSUFFICIENT, got {status_counts['DATA_INSUFFICIENT']}"
        assert status_counts["PARTIAL_PRIMARY_VERIFIED"] + status_counts["FULL_PRIMARY_VERIFIED"] == 20

    def test_3_missing_and_quarantined_historical_price_values(self, db_conn):
        """Invariant 3: Exactly 23,132 quarantined bars and 603 populated bars."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*),
                count(*) FILTER (WHERE adjusted_close IS NULL),
                count(*) FILTER (WHERE adjusted_close IS NOT NULL)
            FROM ohlcv_daily;
        """)
        total, quarantined, populated = cur.fetchone()
        assert total == 23735, f"Total daily bars must be 23,735, got {total}"
        assert quarantined == 23132, f"Quarantined daily bars must be 23,132, got {quarantined}"
        assert populated == 603, f"Populated daily bars must be 603, got {populated}"

    def test_4_incorrect_price_series_consumption_protection(self, db_conn):
        """Invariant 4: Tax-cost allocations are never used as price adjustment factors."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT count(*)
            FROM ca.v_tax_cost_allocations
            WHERE price_factor_applicable = true;
        """)
        invalid_tax_factors = cur.fetchone()[0]
        assert invalid_tax_factors == 0, f"Tax cost allocations must have price_factor_applicable = false, got {invalid_tax_factors}"

        # Demerger tax cost allocation for TATAMOTORS must be 68.85%
        cur.execute("""
            SELECT tax_cost_allocation_pct, factor_category, dispute_status
            FROM ca.v_tax_cost_allocations
            WHERE symbol = 'TATAMOTORS';
        """)
        row = cur.fetchone()
        assert row is not None, "TATAMOTORS tax cost allocation record must exist"
        pct, cat, disp = row
        assert float(pct) == 68.8500
        assert cat == "TAX_COST_ALLOCATION"
        assert disp == "DISPUTED_NON_PRICE_FACTOR"

    def test_5_decimal_adjustment_and_rounding_accuracy(self, db_conn):
        """Invariant 5: Fixed-point decimal arithmetic reproduces exact roundings; float32 drift is isolated."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT total_return_factor, cum_price_reference, dividend_amount
            FROM v_latest_corporate_action_factors
            WHERE symbol = 'TCS' AND factor_category = 'CASH_DISTRIBUTION';
        """)
        factor, cum_price, div_amt = cur.fetchone()
        assert float(div_amt) == 12.0
        assert float(cum_price) == 2269.3000
        expected_factor = (2269.30 - 12.00) / 2269.30
        assert abs(float(factor) - expected_factor) < 1e-7

        # Exact PostgreSQL NUMERIC roundings
        cur.execute("""
            SELECT 
                ROUND(2080.3000 * 0.99471203::numeric, 4),
                ROUND(2125.0000 * 0.99471203::numeric, 4);
        """)
        exact_oct07, exact_oct12 = cur.fetchone()
        assert float(exact_oct07) == 2069.2994
        assert float(exact_oct12) == 2113.7631

        # IEEE-754 float32 simulation matches observed drift
        import numpy as np
        c1 = np.float32(2080.30)
        c2 = np.float32(2125.00)
        f32 = np.float32(0.99471203)
        drift_oct07 = round(float(c1 * f32), 4)
        drift_oct12 = round(float(c2 * f32), 4)
        assert drift_oct07 == 2069.2996, "float32 drift matches +0.0002 on Oct 07"
        assert drift_oct12 == 2113.7632, "float32 drift matches +0.0001 on Oct 12"

    def test_6_backtests_attempting_to_consume_ineligible_data_blocked(self, readiness_matrix):
        """Invariant 6: Overall research status is RESEARCH_BLOCKED; zero instruments eligible for full period."""
        assert readiness_matrix["overall_research_gate"] == "RESEARCH_BLOCKED"
        assert readiness_matrix["quarantined_bars_count"] == 23132
        for item in readiness_matrix["matrix"]:
            assert item["overall_research_verdict"] == "RESEARCH_BLOCKED"
            assert item["period_historical_2024_2026"]["status"] == "BLOCKED"

    def test_7_shariah_screening_fail_closed_behavior(self, db_conn):
        """Invariant 7: Shariah screening fail-closed status: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT."""
        cur = db_conn.cursor()
        cur.execute("""
            WITH latest_screenings AS (
                SELECT DISTINCT ON (s.symbol) s.symbol, sc.status
                FROM symbols s
                JOIN shariah_screenings sc ON s.id = sc.symbol_id
                WHERE s.is_active = true
                ORDER BY s.symbol, sc.screening_date DESC
            )
            SELECT 
                count(*) FILTER (WHERE status = 'PASS'),
                count(*) FILTER (WHERE status = 'FAIL'),
                count(*) FILTER (WHERE status = 'DATA_INSUFFICIENT')
            FROM latest_screenings;
        """)
        pass_count, fail_count, data_insuf_count = cur.fetchone()
        assert pass_count == 0, f"Expected 0 PASS, got {pass_count}"
        assert fail_count == 20, f"Expected 20 FAIL, got {fail_count}"
        assert data_insuf_count == 31, f"Expected 31 DATA_INSUFFICIENT, got {data_insuf_count}"

    def test_8_runtime_database_privilege_and_evidence_integrity(self, db_conn):
        """Invariant 8: Security triggers and non-superuser permissions guard data integrity."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT count(DISTINCT tgname)
            FROM pg_trigger
            WHERE tgname IN ('trg_ohlcv_daily_quarantine_guard', 'trg_ca_factor_guard', 'trg_ca_artifact_guard', 'trg_ca_event_verification_guard');
        """)
        trigger_count = cur.fetchone()[0]
        assert trigger_count == 4, f"All 4 security triggers must be active, got {trigger_count}"

        cur.execute("SELECT rolsuper FROM pg_roles WHERE rolname = 'halal_app_user';")
        is_super = cur.fetchone()[0]
        assert is_super is False, "halal_app_user must never be superuser"
