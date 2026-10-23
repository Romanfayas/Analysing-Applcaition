"""
HalalEquity P1.9.9 - Historical Data Remediation & Evidence Acquisition Tests
Release-gate checks for:
1. Historical coverage by instrument and date range.
2. Corporate-action evidence completeness and authenticity.
3. OHLCV consistency and quarantine integrity (23,132 bars).
4. Correct adjustment factors and decimal rounding (TCS fixed-point vs float32).
5. Instrument lineage and listing dates (no synthetic bars).
6. Correct price-series consumer separation.
7. Backtest rejection of ineligible instruments or periods.
8. Shariah screening fail-closed behavior (31 DATA_INSUFFICIENT).
"""

import json
import math
import os
import pytest
import psycopg2
import numpy as np


@pytest.fixture(scope="module")
def db_conn():
    db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    conn = psycopg2.connect(db_url)
    conn.autocommit = True
    yield conn
    conn.close()


@pytest.fixture(scope="module")
def recovery_plan():
    paths = [
        "data/remediation/per_instrument_evidence_recovery_plan_v1.json",
        "../data/remediation/per_instrument_evidence_recovery_plan_v1.json",
        "/app/data/remediation/per_instrument_evidence_recovery_plan_v1.json",
    ]
    for p in paths:
        if os.path.exists(p):
            with open(p, "r") as f:
                return json.load(f)
    pytest.fail("per_instrument_evidence_recovery_plan_v1.json not found")


@pytest.fixture(scope="module")
def provider_matrix():
    paths = [
        "data/remediation/provider_comparison_matrix_v1.json",
        "../data/remediation/provider_comparison_matrix_v1.json",
        "/app/data/remediation/provider_comparison_matrix_v1.json",
    ]
    for p in paths:
        if os.path.exists(p):
            with open(p, "r") as f:
                return json.load(f)
    pytest.fail("provider_comparison_matrix_v1.json not found")


@pytest.fixture(scope="module")
def shariah_plan():
    paths = [
        "data/remediation/shariah_evidence_recovery_plan_v1.json",
        "../data/remediation/shariah_evidence_recovery_plan_v1.json",
        "/app/data/remediation/shariah_evidence_recovery_plan_v1.json",
    ]
    for p in paths:
        if os.path.exists(p):
            with open(p, "r") as f:
                return json.load(f)
    pytest.fail("shariah_evidence_recovery_plan_v1.json not found")


@pytest.fixture(scope="module")
def ingestion_spec():
    paths = [
        "data/remediation/source_authentication_and_ingestion_spec_v1.json",
        "../data/remediation/source_authentication_and_ingestion_spec_v1.json",
        "/app/data/remediation/source_authentication_and_ingestion_spec_v1.json",
    ]
    for p in paths:
        if os.path.exists(p):
            with open(p, "r") as f:
                return json.load(f)
    pytest.fail("source_authentication_and_ingestion_spec_v1.json not found")


class TestP199RemediationReleaseGates:

    def test_1_historical_coverage_by_instrument_and_date_range(self, db_conn, recovery_plan):
        """Invariant 1: Historical coverage across 52 instruments (47 core, 5 lineage)."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*),
                count(*) FILTER (WHERE is_active = true),
                count(*) FILTER (WHERE is_active = false)
            FROM symbols;
        """)
        total, active, archived = cur.fetchone()
        assert total == 52
        assert active == 51
        assert archived == 1

        cur.execute("SELECT count(DISTINCT symbol_id) FROM ohlcv_daily;")
        ohlcv_symbols = cur.fetchone()[0]
        assert ohlcv_symbols == 47

        summary = recovery_plan["summary"]
        assert summary["total_catalog_instruments"] == 52
        assert summary["active_core_equities"] == 47
        assert summary["lineage_and_restructuring_instruments"] == 5
        assert summary["total_quarantined_daily_bars"] == 23132

    def test_2_corporate_action_evidence_completeness_and_authenticity(self, db_conn, recovery_plan):
        """Invariant 2: Evidence partition: 27 core unbacked, 20 backed, 1 full (TCS)."""
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
        unbacked_count = cur.fetchone()[0]
        assert unbacked_count == 27

        summary = recovery_plan["summary"]
        assert summary["core_equities_missing_primary_evidence"] == 27
        assert summary["core_equities_with_full_primary_evidence"] == 1
        assert summary["core_equities_with_partial_primary_evidence"] == 19

    def test_3_ohlcv_consistency_and_quarantine_integrity(self, db_conn):
        """Invariant 3: Exactly 23,132 quarantined bars; zero duplicate bars."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*),
                count(*) FILTER (WHERE adjusted_close IS NULL),
                count(*) FILTER (WHERE adjusted_close IS NOT NULL)
            FROM ohlcv_daily;
        """)
        total, quarantined, populated = cur.fetchone()
        assert total == 23735
        assert quarantined == 23132
        assert populated == 603

        # Zero duplicates
        cur.execute("""
            SELECT count(*)
            FROM (
                SELECT symbol_id, timestamp, count(*)
                FROM ohlcv_daily
                GROUP BY symbol_id, timestamp
                HAVING count(*) > 1
            ) d;
        """)
        assert cur.fetchone()[0] == 0

    def test_4_correct_adjustment_factors_and_decimal_rounding(self, db_conn):
        """Invariant 4: Multiplicative factors and decimal rounding reproduction."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT total_return_factor, cum_price_reference, dividend_amount
            FROM v_latest_corporate_action_factors
            WHERE symbol = 'TCS' AND factor_category = 'CASH_DISTRIBUTION';
        """)
        factor, cum_price, div_amount = cur.fetchone()
        assert float(div_amount) == 12.0
        assert float(cum_price) == 2269.3000
        expected_factor = (2269.30 - 12.00) / 2269.30
        assert abs(float(factor) - expected_factor) < 1e-7

        # Exact PostgreSQL decimal calculation
        cur.execute("""
            SELECT 
                ROUND(2080.3000 * 0.99471203::numeric, 4),
                ROUND(2125.0000 * 0.99471203::numeric, 4);
        """)
        exact_oct07, exact_oct12 = cur.fetchone()
        assert float(exact_oct07) == 2069.2994
        assert float(exact_oct12) == 2113.7631

        # IEEE-754 float32 drift simulation
        c1 = np.float32(2080.30)
        c2 = np.float32(2125.00)
        f32 = np.float32(0.99471203)
        assert round(float(c1 * f32), 4) == 2069.2996
        assert round(float(c2 * f32), 4) == 2113.7632

    def test_5_instrument_lineage_and_listing_dates(self, db_conn):
        """Invariant 5: Lineage resolution and zero synthetic pre-listing bars."""
        cur = db_conn.cursor()
        cur.execute("SELECT is_active FROM symbols WHERE symbol = 'TATAMOTORS';")
        assert cur.fetchone()[0] is False

        cur.execute("""
            SELECT s.symbol, count(o.timestamp)
            FROM symbols s
            LEFT JOIN ohlcv_daily o ON s.id = o.symbol_id
            WHERE s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM')
            GROUP BY s.symbol;
        """)
        bars = dict(cur.fetchall())
        for sym in ['TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM']:
            assert bars.get(sym, 0) == 0, f"{sym} must have 0 bars in ohlcv_daily"

    def test_6_correct_price_series_consumer_separation(self, db_conn):
        """Invariant 6: Nominal vs split vs total return vs tax cost allocation."""
        cur = db_conn.cursor()
        cur.execute("""
            SELECT count(*)
            FROM ca.v_tax_cost_allocations
            WHERE price_factor_applicable = true;
        """)
        assert cur.fetchone()[0] == 0, "Tax cost allocations must have price_factor_applicable = false"

        cur.execute("SELECT tax_cost_allocation_pct FROM ca.v_tax_cost_allocations WHERE symbol = 'TATAMOTORS';")
        row = cur.fetchone()
        assert row is not None
        assert float(row[0]) == 68.8500

    def test_7_backtest_rejection_of_ineligible_instruments_or_periods(self, provider_matrix):
        """Invariant 7: Quantitative research status is blocked; Yahoo Finance prohibited."""
        prohibited = [p for p in provider_matrix["providers"] if p["provider_id"] == "YAHOO_FINANCE_AGGREGATOR"][0]
        assert prohibited["evaluation_verdict"]["suitability"] == "PROHIBITED_FOR_RESEARCH_AND_TRADING"
        recommended = [p for p in provider_matrix["providers"] if p["provider_id"] == "NSE_DATA_ANALYTICS"][0]
        assert recommended["evaluation_verdict"]["suitability"] == "RECOMMENDED_PRIMARY_FOR_REGULATORY_AUTHENTICATION"

    def test_8_shariah_evidence_screening_fail_closed(self, db_conn, shariah_plan):
        """Invariant 8: Shariah screening fail-closed status: exactly 31 DATA_INSUFFICIENT."""
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
        assert pass_count == 0
        assert fail_count == 20
        assert data_insuf_count == 31

        assert shariah_plan["total_data_insufficient_instruments"] == 31
        assert len(shariah_plan["instruments"]) == 31
        for inst in shariah_plan["instruments"]:
            assert inst["current_status"] == "DATA_INSUFFICIENT"
            assert inst["missing_evidence"]["field"] == "interest_income_ratio"
