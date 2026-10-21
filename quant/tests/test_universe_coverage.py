"""
Tests for Universe Definition and Safe Coverage Management
"""

import pytest
from src.engines.research.universe import (
    UniverseManager,
    UniverseTier,
    TIER_1_CORE_SYMBOLS,
    CORPORATE_ACTION_TRACKED_SYMBOLS,
)


def test_tier_1_universe_definition():
    """Verify Tier 1 Core universe has exactly 47 curated symbols."""
    mgr = UniverseManager()
    symbols = mgr.get_tier_symbols(UniverseTier.TIER_1_CORE)

    assert len(symbols) == 47
    assert len(set(symbols)) == 47
    assert "INFY" in symbols
    assert "TCS" in symbols
    assert "RELIANCE" in symbols


def test_corporate_action_mapping_integrity():
    """Verify corporate actions for LTIM rename and TATAMOTORS demerger are explicitly tracked."""
    assert "LTIM" in CORPORATE_ACTION_TRACKED_SYMBOLS
    assert CORPORATE_ACTION_TRACKED_SYMBOLS["LTIM"]["event"] == "SYMBOL_RENAME"
    assert CORPORATE_ACTION_TRACKED_SYMBOLS["LTIM"]["successor"] == "LTM"

    assert "TATAMOTORS" in CORPORATE_ACTION_TRACKED_SYMBOLS
    assert CORPORATE_ACTION_TRACKED_SYMBOLS["TATAMOTORS"]["event"] == "DEMERGER"
    assert CORPORATE_ACTION_TRACKED_SYMBOLS["TATAMOTORS"]["successors"] == ["TMPV", "TMCV"]
    cost = CORPORATE_ACTION_TRACKED_SYMBOLS["TATAMOTORS"]["cost_apportionment"]
    assert cost["TMPV"] + cost["TMCV"] == pytest.approx(1.0, abs=1e-4)


def test_universe_coverage_audit():
    """Coverage numbers must equal independently computed (calendar x symbols) arithmetic, not constants."""
    from src.engines.research.data_audit import audit_daily, connect
    mgr = UniverseManager()
    try:
        report = mgr.audit_coverage(UniverseTier.TIER_1_CORE)
    except Exception as e:  # DB unreachable: must raise, never fabricate
        pytest.skip(f"database not reachable: {e}")
    audit = audit_daily(connect(), symbol_filter=mgr.get_tier_symbols(UniverseTier.TIER_1_CORE))

    assert report.selected_tier == "TIER_1_CORE"
    assert report.total_symbols_in_tier == 47
    assert report.total_clean_bars == audit["total_bars"]
    assert report.total_clean_bars + report.missing_sessions_count == audit["expected_bars"]
    assert report.holiday_phantom_bars_count == 0
    assert report.reconciled_clean_symbols <= 47
    assert len(report.corporate_action_issues) == 2
    assert "ohlcv_daily" in report.feed_provenance
    assert "adjusted_close is NULL" in report.disclosure


def test_audit_never_fabricates_on_db_failure():
    mgr = UniverseManager(db_url="postgres://x:y@127.0.0.1:1/none")
    with pytest.raises(Exception):
        mgr.audit_coverage(UniverseTier.TIER_1_CORE)
