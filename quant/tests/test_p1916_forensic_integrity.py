"""
P1.9.16 Forensic Integrity Regression Test Suite:
1. P0: Strategy V2 Core Parameters (3-param) vs Composite Breakout/Risk (12-param) SHA-256 identity and drift detection.
2. P0: TCS Dividend Arithmetic (₹12 on ₹2269.30 vs erroneous ₹10 on ₹1891) and unauthenticated quarantine persistence.
3. P1: Go / Python cross-language deterministic serialization contract.
"""
import hashlib
import json
import os
import pytest
import psycopg2
from decimal import Decimal, ROUND_HALF_UP

from src.engines.research.strategy_config import (
    PINNED_HASH,
    STRATEGY_V2_CONFIG_ID,
    FROZEN,
    COMPOSITE_BREAKOUT_RISK_ID,
    COMPOSITE_BREAKOUT_RISK_FROZEN,
    COMPOSITE_COMPACT_HASH,
    COMPOSITE_STANDARD_HASH,
    assert_frozen,
    canonical_json,
    compute_hash,
)


@pytest.fixture(scope="module")
def db_conn():
    db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    conn = psycopg2.connect(db_url)
    conn.autocommit = True
    yield conn
    conn.close()


class TestP1916ForensicIntegrity:

    def test_strategy_v2_canonical_serialization_and_hash(self):
        """
        Verify that the 3-parameter Strategy V2 frozen configuration
        reproduces PINNED_HASH to the exact byte.
        """
        # Exact canonical serialization format:
        # sorted keys, ': ' key-val separator, ', ' item separator, trailing .0 float format, no newline
        expected_bytes = b'{"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}'
        actual_json = canonical_json(FROZEN)
        actual_bytes = actual_json.encode("utf-8")

        assert actual_bytes == expected_bytes, f"Expected {expected_bytes}, got {actual_bytes}"

        h = hashlib.sha256(actual_bytes).hexdigest()
        assert h == PINNED_HASH
        assert PINNED_HASH == "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"

        # Parameter Drift Detection: Modifying BUY_THRESHOLD from 80.0 to 80.1 must fail
        drifted_cfg1 = {"BUY_THRESHOLD": 80.1, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 2.0}
        assert compute_hash(drifted_cfg1) != PINNED_HASH

        # Parameter Drift Detection: Modifying ATR_MULTIPLIER from 2.0 to 2.1 must fail
        drifted_cfg2 = {"BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 2.1}
        assert compute_hash(drifted_cfg2) != PINNED_HASH

        # Serialization Drift Detection: Compact JSON (no spaces) must fail
        compact_bytes = b'{"ATR_MULTIPLIER":2.0,"BUY_THRESHOLD":80.0,"SELL_THRESHOLD":45.0}'
        assert hashlib.sha256(compact_bytes).hexdigest() != PINNED_HASH

        # Serialization Drift Detection: Trailing newline must fail
        newline_bytes = expected_bytes + b"\n"
        assert hashlib.sha256(newline_bytes).hexdigest() != PINNED_HASH

    def test_composite_breakout_risk_disambiguation(self):
        """
        Disambiguate the 12-parameter composite breakout/risk payload from the 3-parameter core strategy.
        Prove that neither compact nor standard serialization yields 92ab27c2...
        """
        # Compact serialization: separators=(',', ':')
        compact_json = json.dumps(COMPOSITE_BREAKOUT_RISK_FROZEN, sort_keys=True, separators=(",", ":"))
        compact_hash = hashlib.sha256(compact_json.encode("utf-8")).hexdigest()

        assert compact_hash == COMPOSITE_COMPACT_HASH
        assert compact_hash == "0daac86130da4bf6e0fba7b2a9e8f7ff76c309dee4702b4a0a23c33e0a42f79f"
        assert compact_hash != PINNED_HASH, "12-parameter composite hash CANNOT match 3-parameter core hash 92ab27c2..."

        # Standard Python serialization: separators=(', ', ': ')
        standard_json = json.dumps(COMPOSITE_BREAKOUT_RISK_FROZEN, sort_keys=True)
        standard_hash = hashlib.sha256(standard_json.encode("utf-8")).hexdigest()

        assert standard_hash == COMPOSITE_STANDARD_HASH
        assert standard_hash == "deb24826865c3eefb2a1c7126ccf64f1955904af8631859042de93e980e6b5ba"
        assert standard_hash != PINNED_HASH, "Standard 12-parameter composite hash CANNOT match 92ab27c2..."

    def test_tcs_corporate_action_factor_reconciliation(self, db_conn):
        """
        Reconcile the TCS dividend calculation:
        True: (2269.30 - 12.00) / 2269.30 -> 0.99471203 (8-dec ROUND_HALF_UP).
        Erroneous claim in P1.9.15: (1891.00 - 10.00) / 1891.00 -> 0.99471179.
        """
        # 1. Decimal arithmetic test
        cum_close_true = Decimal("2269.30")
        dividend_true = Decimal("12.00")
        factor_raw = (cum_close_true - dividend_true) / cum_close_true
        factor_8dec = factor_raw.quantize(Decimal("0.00000001"), rounding=ROUND_HALF_UP)

        assert float(factor_8dec) == 0.99471203

        # 2. Erroneous P1.9.15 claim test
        cum_close_err = Decimal("1891.00")
        dividend_err = Decimal("10.00")
        factor_err_raw = (cum_close_err - dividend_err) / cum_close_err
        factor_err_8dec = factor_err_raw.quantize(Decimal("0.00000001"), rounding=ROUND_HALF_UP)

        assert float(factor_err_8dec) == 0.99471179
        assert float(factor_err_8dec) != 0.99471203, "Erroneous ₹10 on ₹1891 close diverges from 0.99471203"

        # 3. Database event verification
        cur = db_conn.cursor()
        cur.execute("""
            SELECT dividend_amount, verification_status
            FROM corporate_action_events
            WHERE symbol = 'TCS' AND ex_date = '2026-10-14' AND action_type = 'CASH_DIVIDEND';
        """)
        ev_div, ev_status = cur.fetchone()
        assert float(ev_div) == 12.00, f"Expected dividend ₹12.00 in database, got {ev_div}"
        assert ev_status == "ARCHIVE_UNAUTHENTICATED", f"Expected ARCHIVE_UNAUTHENTICATED, got {ev_status}"

        # 4. Database factor verification (Revision 3)
        cur.execute("""
            SELECT total_return_factor, cum_price_reference
            FROM corporate_action_adjustment_factors
            WHERE symbol_id = (SELECT id FROM canonical.symbols WHERE symbol = 'TCS')
              AND revision_number = 3;
        """)
        f_tr, f_ref = cur.fetchone()
        assert float(f_tr) == 0.99471203
        assert float(f_ref) == 2269.30

        # 5. Database 7 bars source check
        cur.execute("""
            SELECT count(*)
            FROM canonical.ohlcv_daily o
            JOIN canonical.symbols s ON o.symbol_id = s.id
            WHERE s.symbol = 'TCS' AND o.source = 'NSE_CORPORATE_ACTION';
        """)
        tcs_bars = cur.fetchone()[0]
        assert tcs_bars == 7, f"Expected 7 bars for TCS with source NSE_CORPORATE_ACTION, got {tcs_bars}"

    def test_runtime_introspected_parameters_match_frozen(self):
        """Verify that live runtime parameters loaded from code paths strictly match frozen Strategy V2."""
        res = assert_frozen()
        assert res["matches_pinned"] is True
        assert res["runtime_hash"] == PINNED_HASH
        assert res["resolved"]["BUY_THRESHOLD"] == 80.0
        assert res["resolved"]["SELL_THRESHOLD"] == 45.0
        assert res["resolved"]["ATR_MULTIPLIER"] == 2.0
