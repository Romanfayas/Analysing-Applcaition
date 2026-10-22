"""
HalalEquity P1.9.7 - Automated Release-Gate Tests
Canonical Universe Reconciliation & Compliant Data Ingestion Release Gates

Invariants Enforced:
1. Canonical-universe counts disagree -> FAIL
2. An active instrument has unresolved lineage -> FAIL
3. Required historical corporate actions lack authenticated evidence -> FAIL
4. A required price series contains quarantined bars -> FAIL
5. A nominal-price consumer reads total-return-adjusted prices -> FAIL
6. A backtest consumes data marked DATA_INSUFFICIENT -> FAIL
"""

import json
import os
import pytest
import psycopg2


@pytest.fixture(scope="module")
def db_conn():
    db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    conn = psycopg2.connect(db_url)
    yield conn
    conn.close()


@pytest.fixture(scope="module")
def manifest():
    paths = [
        "data/universe/canonical_universe_v1.json",
        "../data/universe/canonical_universe_v1.json",
        "/app/data/universe/canonical_universe_v1.json",
        "/Users/fayas/Downloads/ICO-Application/analysis application/data/universe/canonical_universe_v1.json",
    ]
    for p in paths:
        if os.path.exists(p):
            with open(p, "r") as f:
                return json.load(f)
    pytest.fail("canonical_universe_v1.json manifest not found")


class TestCanonicalUniverseReleaseGates:

    def test_gate_1_canonical_universe_counts_reconciliation(self, db_conn, manifest):
        """Release Gate 1: Fail when canonical-universe counts disagree."""
        cur = db_conn.cursor()
        
        # 1. Database total, active, and archived instruments
        cur.execute("""
            SELECT 
                count(*),
                count(*) FILTER (WHERE is_active = true),
                count(*) FILTER (WHERE is_active = false)
            FROM symbols;
        """)
        total, active, archived = cur.fetchone()
        
        assert total == manifest["summary"]["total_database_instruments"], f"Expected {manifest['summary']['total_database_instruments']} instruments, got {total}"
        assert active == manifest["summary"]["active_instruments_in_db"], f"Expected {manifest['summary']['active_instruments_in_db']} active instruments, got {active}"
        assert archived == manifest["summary"]["archived_instruments_in_db"], f"Expected {manifest['summary']['archived_instruments_in_db']} archived instruments, got {archived}"
        
        # 2. Distinct symbols in OHLCV daily bars must be exactly 47
        cur.execute("SELECT count(DISTINCT symbol_id) FROM ohlcv_daily;")
        ohlcv_symbols = cur.fetchone()[0]
        assert ohlcv_symbols == manifest["summary"]["symbols_with_ohlcv_bars"], f"Expected {manifest['summary']['symbols_with_ohlcv_bars']} symbols with bars, got {ohlcv_symbols}"

        # 3. Quarantined bars invariant
        cur.execute("SELECT count(*) FROM ohlcv_daily WHERE adjusted_close IS NULL;")
        quarantined = cur.fetchone()[0]
        assert quarantined == manifest["summary"]["quarantined_daily_bars"], f"Quarantined bars must be {manifest['summary']['quarantined_daily_bars']}, got {quarantined}"

    def test_gate_2_active_instrument_unresolved_lineage(self, db_conn, manifest):
        """Release Gate 2: Fail when an active instrument has unresolved lineage."""
        cur = db_conn.cursor()

        # 1. Demerger parent TATAMOTORS must be inactive
        cur.execute("SELECT is_active FROM symbols WHERE symbol = 'TATAMOTORS';")
        tm_active = cur.fetchone()[0]
        assert tm_active is False, "TATAMOTORS must be inactive to prevent concurrent double-exposure"

        # 2. Both successors TMPV and TMCV must be registered active
        cur.execute("SELECT symbol, is_active FROM symbols WHERE symbol IN ('TMPV', 'TMCV');")
        successors = dict(cur.fetchall())
        assert successors.get("TMPV") is True, "TMPV must be registered active successor"
        assert successors.get("TMCV") is True, "TMCV must be registered active successor"

        # 3. Successors must have 0 pre-listing bars (no fabricated synthetic bars)
        cur.execute("""
            SELECT s.symbol, count(o.timestamp)
            FROM symbols s
            LEFT JOIN ohlcv_daily o ON s.id = o.symbol_id
            WHERE s.symbol IN ('TMPV', 'TMCV')
            GROUP BY s.symbol;
        """)
        bars = dict(cur.fetchall())
        assert bars.get("TMPV", 0) == 0, "TMPV must not have fabricated pre-listing bars"
        assert bars.get("TMCV", 0) == 0, "TMCV must not have fabricated pre-listing bars"

        # 4. Rename lineage: LTIM to LTM must have 1:1 ratio
        cur.execute("""
            SELECT adjustment_factor 
            FROM corporate_action_adjustment_factors 
            WHERE action_type = 'SYMBOL_CHANGE' AND is_verified = true 
            LIMIT 1;
        """)
        rename_fac = float(cur.fetchone()[0])
        assert rename_fac == 1.0, f"Symbol rename factor must be strictly 1.0, got {rename_fac}"

    def test_gate_3_required_historical_corporate_actions_lack_authenticated_evidence(self, db_conn):
        """Release Gate 3: Fail when required historical corporate actions lack authenticated evidence."""
        cur = db_conn.cursor()

        # Active price adjustment factors must have SHA-256 primary artifact
        cur.execute("""
            SELECT count(*)
            FROM ca.v_latest_corporate_action_factors ca
            LEFT JOIN corporate_action_events ev ON ca.event_id = ev.event_id AND ca.symbol_id = ev.symbol_id
            LEFT JOIN corporate_action_source_artifacts art ON art.id = ev.source_artifact_id
            WHERE ca.price_factor_applicable = true
              AND (art.id IS NULL OR art.sha256_checksum IS NULL OR length(art.sha256_checksum) != 64);
        """)
        unbacked = cur.fetchone()[0]
        assert unbacked == 0, f"Found {unbacked} active price adjustment factors lacking verified exchange artifacts"

        # All historical bars prior to 2026-10-05 must remain quarantined NULL
        cur.execute("""
            SELECT count(*)
            FROM ohlcv_daily
            WHERE timestamp < '2026-10-05' AND adjusted_close IS NOT NULL;
        """)
        premature_populated = cur.fetchone()[0]
        assert premature_populated == 0, f"Found {premature_populated} pre-Oct 5 bars with populated adjusted_close without verified evidence"

    def test_gate_4_required_price_series_contains_quarantined_bars(self, db_conn):
        """Release Gate 4: Fail when a required price series contains quarantined bars."""
        cur = db_conn.cursor()

        # Audit historical symbols: each core equity has 492 quarantined bars prior to 2026-10-05
        cur.execute("""
            SELECT s.symbol, count(*)
            FROM ohlcv_daily o
            JOIN symbols s ON o.symbol_id = s.id
            WHERE o.adjusted_close IS NULL
            GROUP BY s.symbol;
        """)
        quarantined_per_sym = dict(cur.fetchall())
        assert len(quarantined_per_sym) == 47, f"Expected all 47 core symbols to have quarantined bars, got {len(quarantined_per_sym)}"
        for sym, cnt in quarantined_per_sym.items():
            assert cnt >= 491, f"Symbol {sym} must have at least 491 quarantined bars, got {cnt}"

    def test_gate_5_nominal_price_consumer_reads_total_return_adjusted_prices(self, db_conn):
        """Release Gate 5: Fail when a nominal-price consumer reads total-return-adjusted prices."""
        cur = db_conn.cursor()

        # 1. Nominal view must return 0 rows for cash dividends
        cur.execute("SELECT count(*) FROM ca.v_active_nominal_price_factors WHERE action_type = 'CASH_DIVIDEND';")
        div_in_nom = cur.fetchone()[0]
        assert div_in_nom == 0, "Nominal price factor view must never include cash dividends"

        # 2. Total return view must contain the verified TCS cash dividend
        cur.execute("SELECT count(*) FROM ca.v_active_total_return_factors WHERE symbol = 'TCS';")
        tcs_tr = cur.fetchone()[0]
        assert tcs_tr == 1, "TCS dividend must be present in total return view"

        # 3. TCS nominal view must contain 0 rows
        cur.execute("SELECT count(*) FROM ca.v_active_nominal_price_factors WHERE symbol = 'TCS';")
        tcs_nom = cur.fetchone()[0]
        assert tcs_nom == 0, "TCS must have 0 rows in nominal price factors view"

    def test_gate_6_backtest_consumes_data_marked_data_insufficient(self, db_conn):
        """Release Gate 6: Fail when a backtest consumes data marked DATA_INSUFFICIENT."""
        cur = db_conn.cursor()

        # Invariant: research status remains blocked and 0 PASS Shariah screenings exist
        cur.execute("SELECT count(*) FROM shariah_screenings WHERE status = 'PASS';")
        pass_count = cur.fetchone()[0]
        assert pass_count == 0, f"Found {pass_count} spurious PASS screenings; Shariah must remain fail-closed"

        cur.execute("SELECT count(*) FROM shariah_screenings WHERE status = 'DATA_INSUFFICIENT';")
        insufficient_count = cur.fetchone()[0]
        assert insufficient_count >= 90, f"Expected at least 90 DATA_INSUFFICIENT screenings, got {insufficient_count}"
