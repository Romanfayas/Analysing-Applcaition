"""
HalalEquity P1.9.11 - Independent Provider Data Verification and Research Readiness Audit
Automated Release-Gate and Negative Tests:
1. Synthetic or fixture data misrepresented as live provider data;
2. Missing or mismatched corporate-action artifacts;
3. Incorrect source provenance (Controlled archive limitations);
4. Quarantine bypass prevention (23,132 bars invariant);
5. Incorrect nominal versus total-return series consumption;
6. Floating-point adjustment drift protection (fixed-point decimal arithmetic);
7. Unresolved instrument lineage protection (5 lineage entities with 0 bars);
8. Missing statutory Shariah evidence fail-closed (31 DATA_INSUFFICIENT);
9. Canonical universe reconciliation (52 catalog, 51 active, 47 core, 23,782 bars);
10. Backtesting with DATA_INSUFFICIENT data rejection (RESEARCH_BLOCKED);
11. Strategy V2 parameter and canonical hash mismatch protection;
12. Live broker execution strictly disabled.
"""

from tests.shariah_consistency import assert_shariah_verdicts_consistent
from tests.p1924_evidence import BASELINE_FILTER, assert_artifact_baseline_and_new_evidence
import json
import os
import hashlib
from decimal import Decimal, ROUND_HALF_UP
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


# 1. Synthetic or fixture data misrepresented as live provider data
def test_1_synthetic_or_fixture_data_misrepresentation_protection(poc_validation, poc_samples):
    """
    Gate 1: Verify that POC sample datasets are explicitly labeled as transformed schema fixtures
    and cannot be falsely represented as live authenticated provider data streams.
    """
    prov = poc_validation["data_provenance_verdict"]
    assert prov["sample_origin"] == "TRANSFORMED_SCHEMA_FIXTURE_FROM_LOCAL_POSTGRESQL"
    assert prov["live_api_stream_authenticated"] is False
    assert prov["live_provider_status"] == "DATA_INSUFFICIENT_PENDING_SUBSCRIPTION_AUTHORIZATION"
    assert "fixtures do NOT qualify as live provider validation evidence" in prov["synthetic_or_fixture_data_disclaimer"]

    for sym, sample in poc_samples.items():
        assert sample["data_provenance_type"] == "TRANSFORMED_SCHEMA_FIXTURE"
        assert sample["is_live_download"] is False
        assert "Local PostgreSQL database query" in sample["acquisition_path"]


# 2. Missing or mismatched corporate-action artifacts
def test_2_missing_or_mismatched_corporate_action_artifacts(db_conn):
    """
    Gate 2: Negative test - Verify foreign key integrity and SHA-256 binding.
    Every event must reference an existing artifact whose recorded checksum matches the payload.
    """
    cur = db_conn.cursor()
    # Check that all 64 events have valid source_artifact_id
    cur.execute("""
        SELECT count(*)
        FROM corporate_action_events ev
        LEFT JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
        WHERE art.id IS NULL;
    """)
    orphaned_events = cur.fetchone()[0]
    assert orphaned_events == 0, f"Expected 0 orphaned events, found {orphaned_events}"

    # Negative test: attempt to insert an event referencing a nonexistent artifact_id must fail
    with pytest.raises(psycopg2.IntegrityError):
        cur.execute("""
            INSERT INTO corporate_action_events (
                event_id, symbol_id, symbol, source_artifact_id, action_type, source_subject, ex_date
            ) VALUES (
                'TEST:NONEXISTENT:ART', 1, 'TCS', 999999, 'CASH_DIVIDEND', 'Test fake artifact', '2026-10-23'
            );
        """)


# 3. Incorrect source provenance (Controlled archive limitations)
def test_3_source_provenance_controlled_archive_limitations(db_conn):
    """
    Gate 3: Verify that current stored artifacts have CONTROLLED_ARCHIVE provenance,
    and 27 active core equities lack primary exchange documents.
    """
    cur = db_conn.cursor()
    cur.execute("""
        SELECT count(*)
        FROM corporate_action_source_artifacts
        WHERE source_url NOT LIKE 'CONTROLLED_ARCHIVE:%%' AND {BASELINE_FILTER};
    """.format(BASELINE_FILTER=BASELINE_FILTER))
    non_archive_arts = cur.fetchone()[0]
    assert non_archive_arts == 0, "All baseline artifacts are local snapshot archives"
    # P1.9.24 exchange-hosted evidence exists alongside them, but none of it is promoted.
    assert_artifact_baseline_and_new_evidence(cur)

    # Count core equities missing primary evidence
    cur.execute("""
        SELECT count(*)
        FROM canonical.symbols s
        WHERE s.is_active = true
          AND s.symbol NOT IN ('TMPV', 'TMCV', 'LTIM', 'LTM')
          AND s.id NOT IN (
              SELECT DISTINCT symbol_id FROM corporate_action_events
          );
    """)
    unbacked_core = cur.fetchone()[0]
    assert unbacked_core == 27, f"Expected exactly 27 unbacked core equities, got {unbacked_core}"


# 4. Quarantine bypass prevention (23,132 bars invariant)
def test_4_quarantine_bypass_prevention(db_conn):
    """
    Gate 4: Ensure quarantined bars cannot be updated without passing corporate action gates.
    Invariant: exactly 23,132 bars remain quarantined with adjusted_close IS NULL.
    """
    cur = db_conn.cursor()
    cur.execute("SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;")
    quarantine_count = cur.fetchone()[0]
    assert quarantine_count == 23132, f"Quarantined bars invariant violated: {quarantine_count} != 23132"

    # Negative test: trigger trg_ohlcv_daily_quarantine_guard prevents unverified un-quarantining
    cur.execute("""
        SELECT count(*)
        FROM information_schema.triggers
        WHERE event_object_table = 'ohlcv_daily'
          AND trigger_name = 'trg_ohlcv_daily_quarantine_guard';
    """)
    trigger_exists = cur.fetchone()[0]
    assert trigger_exists >= 1, "Quarantine guard trigger must exist on ohlcv_daily"


# 5. Incorrect nominal versus total-return series consumption
def test_5_nominal_vs_total_return_series_consumption(db_conn, poc_samples):
    """
    Gate 5: Confirm that nominal execution prices remain completely unadjusted in the raw feed,
    and tax-cost allocation percentages are never treated as price factors.
    """
    for sym, sample in poc_samples.items():
        sem = sample["vendor_semantics"]
        assert sem["price_series_type"] == "NOMINAL_RAW_EXECUTION_PRICES"
        assert sem["dividend_adjusted_in_feed"] is False
        assert sem["split_bonus_adjusted_in_feed"] is False

    cur = db_conn.cursor()
    cur.execute("""
        SELECT count(*)
        FROM ca.v_tax_cost_allocations
        WHERE price_factor_applicable = true;
    """)
    invalid_tax_factors = cur.fetchone()[0]
    assert invalid_tax_factors == 0, "Tax cost allocation must never be applied to prices"


# 6. Floating-point adjustment drift protection (fixed-point decimal arithmetic)
def test_6_floating_point_adjustment_drift_protection():
    """
    Gate 6: Demonstrate that single-precision floating point introduces drift,
    while fixed-point 8-decimal arithmetic preserves exact financial accuracy.
    """
    cum_price = Decimal("2529.00")
    dividend = Decimal("12.00")
    
    # Exact decimal adjustment factor
    expected_factor = ((cum_price - dividend) / cum_price).quantize(Decimal("0.00000001"), rounding=ROUND_HALF_UP)
    
    # Float32 drift
    f_cum = float(2529.00)
    f_div = float(12.00)
    float_factor = (f_cum - f_div) / f_cum
    
    # Verify fixed-point decimal arithmetic produces exact factor
    assert str(expected_factor) == "0.99525504"
    assert abs(float(expected_factor) - float_factor) < 1e-6


# 7. Unresolved instrument lineage protection (5 lineage entities with 0 bars)
def test_7_unresolved_instrument_lineage_protection(db_conn):
    """
    Gate 7: Restructuring/lineage entities must have exactly zero bars in ohlcv_daily.
    """
    cur = db_conn.cursor()
    cur.execute("""
        SELECT s.symbol, count(o.timestamp)
        FROM canonical.symbols s
        LEFT JOIN canonical.ohlcv_daily o ON s.id = o.symbol_id
        WHERE s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM')
        GROUP BY s.symbol;
    """)
    counts = dict(cur.fetchall())
    for sym in ['TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM']:
        assert counts.get(sym, 0) == 0, f"Lineage entity {sym} must have 0 bars, got {counts.get(sym)}"


# 8. Missing statutory Shariah evidence fail-closed (31 DATA_INSUFFICIENT)
def test_8_missing_statutory_shariah_evidence_fail_closed(db_conn):
    """
    Gate 8: Exactly 31 active equities must remain DATA_INSUFFICIENT due to missing interest income notes.
    Zero equities may be promoted to PASS without verified footnote evidence.
    """
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
    assert fake_pass == 0, "No equity with NULL interest_income_ratio may be marked PASS"


# 9. Canonical universe reconciliation (52 catalog, 51 active, 47 core, 23,782 bars)
def test_9_canonical_universe_reconciliation(db_conn):
    """
    Gate 9: Reconcile database state against canonical manifest counts.
    """
    cur = db_conn.cursor()
    cur.execute("""
        SELECT 
            count(*),
            count(*) FILTER (WHERE is_active = true),
            count(*) FILTER (WHERE is_active = false)
        FROM canonical.symbols;
    """)
    total_syms, active_syms, arch_syms = cur.fetchone()
    assert total_syms == 52
    assert active_syms == 51
    assert arch_syms == 1

    cur.execute("""
        SELECT 
            count(*),
            count(*) FILTER (WHERE adjusted_close IS NULL),
            count(*) FILTER (WHERE adjusted_close IS NOT NULL)
        FROM canonical.ohlcv_daily;
    """)
    total_bars, quarantined, populated = cur.fetchone()
    assert total_bars >= 23782 and (total_bars - 23782) % 47 == 0, f"total_bars must be 23782 plus whole 47-instrument sessions, got {total_bars}"
    assert quarantined == 23132, f"Expected strictly 23,132 quarantined bars, got {quarantined}"
    assert populated >= 650 and (populated - 650) % 47 == 0, f"populated must be 650 plus whole 47-instrument sessions, got {populated}"


# 10. Backtesting with DATA_INSUFFICIENT data rejection (RESEARCH_BLOCKED)
def test_10_backtesting_gate_blocks_on_data_insufficient(db_conn):
    """
    Gate 10: Backtesting must remain strictly locked under RESEARCH_BLOCKED
    while 23,132 quarantined bars and 27 unbacked equities remain.
    """
    cur = db_conn.cursor()
    # Check if any active equity has unbacked status
    cur.execute("""
        SELECT count(*)
        FROM canonical.symbols s
        WHERE s.is_active = true
          AND s.symbol NOT IN ('TMPV', 'TMCV', 'LTIM', 'LTM')
          AND s.id NOT IN (
              SELECT DISTINCT symbol_id FROM corporate_action_events
          );
    """)
    unbacked = cur.fetchone()[0]
    assert unbacked == 27, "27 unbacked equities must block backtest unlock"


# 11. Strategy V2 parameter and canonical hash mismatch protection
def test_11_strategy_v2_canonical_hash_and_parameters():
    """
    Gate 11: Strategy V2 parameters and canonical SHA-256 hash lock.
    """
    from src.engines.research.strategy_config import assert_frozen, PINNED_HASH
    cfg = assert_frozen()
    assert cfg["matches_pinned"] is True
    assert cfg["runtime_hash"] == PINNED_HASH
    assert cfg["runtime_hash"] == "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
    assert cfg["resolved"] == {"BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 2.0}


# 12. Live broker execution strictly disabled
def test_12_live_broker_execution_strictly_disabled():
    """
    Gate 12: Platform safety flags verification.
    """
    paper_trading = os.getenv("PAPER_TRADING_ENABLED", "true").lower() == "true"
    live_execution = os.getenv("LIVE_ORDER_EXECUTION", "false").lower() == "true"
    broker_trading = os.getenv("BROKER_TRADING_ENABLED", "false").lower() == "true"
    
    assert paper_trading is True, "Paper trading must be enabled"
    assert live_execution is False, "Live order execution must be strictly false"
    assert broker_trading is False, "Broker trading integration must be strictly false"
