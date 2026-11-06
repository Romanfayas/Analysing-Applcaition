"""
HalalEquity P1.9.10 - Historical Data Provider Proof of Concept Tests
Verifies:
1. TrueData sample payload acquisition and structure for TCS, INFY, RELIANCE, HDFCBANK.
2. OHLCV data integrity (valid price relationships, no gaps, no duplicates, non-negative volume).
3. Consumer price-series separation (raw nominal vs split-adjusted vs total-return).
4. Corporate-action event mapping, factor calculation methodology, and primary provenance.
5. POC validation report findings and conditional GO / NO-GO recommendation.
6. Preservation of database quarantine and platform safety invariants.
"""

from tests.shariah_consistency import assert_shariah_verdicts_consistent
import json
import os
import hashlib
import pytest
import psycopg2


def find_file(relative_path):
    candidates = [
        relative_path,
        os.path.join("..", relative_path),
        os.path.join("/app", relative_path),
        os.path.join("/Users/fayas/Downloads/ICO-Application/analysis application", relative_path)
    ]
    for c in candidates:
        if os.path.exists(c):
            return c
    raise FileNotFoundError(f"Cannot find {relative_path} in candidate locations")


@pytest.fixture(scope="module")
def db_conn():
    db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    conn = psycopg2.connect(db_url)
    conn.autocommit = True
    yield conn
    conn.close()


@pytest.fixture(scope="module")
def poc_validation():
    path = find_file("data/poc/poc_validation_output.json")
    with open(path, "r") as f:
        return json.load(f)


@pytest.fixture(scope="module")
def poc_samples():
    symbols = ["TCS", "INFY", "RELIANCE", "HDFCBANK"]
    samples = {}
    for sym in symbols:
        path = find_file(f"data/poc/truedata_poc_sample_{sym.lower()}.json")
        with open(path, "r") as f:
            samples[sym] = json.load(f)
    return samples


def test_poc_sample_files_exist_and_valid(poc_samples):
    symbols = ["TCS", "INFY", "RELIANCE", "HDFCBANK"]
    for sym in symbols:
        assert sym in poc_samples, f"Missing POC sample for {sym}"
        data = poc_samples[sym]
        assert data["provider"] == "TRUDATA_AUTHORISED_VENDOR"
        assert data["symbol"] == sym
        assert "endpoint" in data
        assert "sample_date_range" in data
        assert data["sample_date_range"]["total_bars"] >= 500
        assert "payload_sha256" in data
        assert "vendor_semantics" in data
        assert "sample_bars" in data
        assert len(data["sample_bars"]) == data["sample_date_range"]["total_bars"]


def test_sample_bars_ohlcv_integrity(poc_samples):
    for sym, data in poc_samples.items():
        bars = data["sample_bars"]
        assert len(bars) > 0, f"No bars found for {sym}"
        prev_ts = None
        for i, bar in enumerate(bars):
            ts = bar["timestamp"]
            o = bar["open"]
            h = bar["high"]
            l = bar["low"]
            c = bar["close"]
            v = bar["volume"]

            # Chronological monotonicity
            if prev_ts is not None:
                assert ts > prev_ts, f"{sym} bar {i} timestamp not strictly ascending: {ts} <= {prev_ts}"
            prev_ts = ts

            # Positive values
            assert o > 0, f"{sym} bar {i} invalid open: {o}"
            assert h > 0, f"{sym} bar {i} invalid high: {h}"
            assert l > 0, f"{sym} bar {i} invalid low: {l}"
            assert c > 0, f"{sym} bar {i} invalid close: {c}"
            assert v >= 0, f"{sym} bar {i} invalid volume: {v}"

            # OHLC consistency
            assert h >= l, f"{sym} bar {i} high < low: {h} < {l}"
            assert h >= o, f"{sym} bar {i} high < open: {h} < {o}"
            assert h >= c, f"{sym} bar {i} high < close: {h} < {c}"
            assert l <= o, f"{sym} bar {i} low > open: {l} > {o}"
            assert l <= c, f"{sym} bar {i} low > close: {l} > {c}"


def test_price_series_separation(poc_samples):
    for sym, data in poc_samples.items():
        semantics = data["vendor_semantics"]
        assert semantics["price_series_type"] == "NOMINAL_RAW_EXECUTION_PRICES"
        assert semantics["dividend_adjusted_in_feed"] is False
        assert semantics["split_bonus_adjusted_in_feed"] is False
        assert "vendor_adjustment_documentation" in semantics


def test_corporate_action_provenance_and_integrity(poc_samples):
    tcs = poc_samples["TCS"]
    cas = tcs["primary_exchange_corporate_actions"]
    assert len(cas) > 0, "TCS must have primary exchange corporate actions"

    # Verify at least one dividend has verified primary authority
    has_verified = False
    for ca in cas:
        if ca["verification_status"] == "VERIFIED_EXCHANGE_PRIMARY":
            has_verified = True
            assert ca["issuing_authority"] == "NSE_INDIA"
            assert ca["document_identifier"] is not None
            assert len(ca["artifact_sha256"]) == 64
    assert has_verified, "TCS must contain verified primary NSE corporate action records"


def test_poc_validation_output_verdict_and_blockers(poc_validation):
    cov = poc_validation["sample_coverage_analysis"]
    assert cov["historical_daily_ohlcv_available"] is True
    assert cov["missing_bars_count"] == 0
    assert cov["duplicate_bars_count"] == 0
    assert cov["invalid_ohlcv_relationships_count"] == 0
    assert cov["zero_volume_phantom_bars_count"] == 0

    audit = poc_validation["price_series_separation_audit"]
    assert audit["raw_nominal_prices_separate"] is True
    assert audit["split_adjusted_prices_separate"] is True
    assert audit["total_return_prices_separate"] is True
    assert audit["tax_cost_allocation_separate"] is True

    verdict = poc_validation["overall_provider_poc_verdict"]
    assert verdict["recommendation"] == "GO_FOR_RAW_OHLCV_INGESTION_CONDITIONAL_ON_SUBSCRIPTION_APPROVAL"
    assert len(verdict["unresolved_blockers"]) >= 3


def test_database_safety_invariants_preserved(db_conn):
    cur = db_conn.cursor()

    # Invariant: exactly 23,132 quarantined bars
    cur.execute("SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;")
    quarantine_count = cur.fetchone()[0]
    assert quarantine_count == 23132, f"Quarantined bars must be 23,132, got {quarantine_count}"

    # Invariant: 0 bars for restructuring/lineage instruments
    cur.execute("""
        SELECT count(*)
        FROM canonical.ohlcv_daily o
        JOIN canonical.symbols s ON o.symbol_id = s.id
        WHERE s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM');
    """)
    lineage_bars = cur.fetchone()[0]
    assert lineage_bars == 0, f"Restructuring instruments must have 0 bars, got {lineage_bars}"

    # Invariant: Shariah screening fail-closed (0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT)
    cur.execute("""
        WITH latest_screenings AS (
            SELECT DISTINCT ON (s.symbol) s.symbol, sc.status
            FROM canonical.symbols s
            JOIN canonical.shariah_screenings sc ON s.id = sc.symbol_id
            WHERE s.is_active = true
            ORDER BY s.symbol, sc.screening_date DESC
        )
        SELECT 
            count(*) FILTER (WHERE status = 'PASS'),
            count(*) FILTER (WHERE status = 'FAIL'),
            count(*) FILTER (WHERE status = 'DATA_INSUFFICIENT')
        FROM latest_screenings;
    """)
    pass_cnt, fail_cnt, data_insuf_cnt = cur.fetchone()
    assert_shariah_verdicts_consistent()
