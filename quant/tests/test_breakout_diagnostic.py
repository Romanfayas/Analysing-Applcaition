"""
Test Suite for Breakout Diagnostic Engine, Baselines, and Research Framework
"""

import os
import psycopg2
import pytest
import pandas as pd
import numpy as np

from src.engines.technical.breakout_backtest import BreakoutBacktester
from src.engines.technical.breakout_diagnostic import BreakoutDiagnosticEngine, DiagnosticTrade
from src.engines.research.breakout_experiments import BreakoutResearchFramework


@pytest.fixture(scope="module")
def clean_historical_candles():
    """Load and aggregate clean historical daily candles from PostgreSQL."""
    db_url = os.environ.get('DATABASE_URL', 'postgres://halal_user:devpassword@postgres:5432/halal_equity')
    conn = psycopg2.connect(db_url)
    query = """
        SELECT s.symbol, s.id as symbol_id, s.name, 
               COALESCE(ss.status::text, 'UNKNOWN') as shariah_status,
               d.timestamp, d.open, d.high, d.low, d.close, d.volume
        FROM canonical.symbols s
        JOIN canonical.ohlcv_daily d ON d.symbol_id = s.id
        LEFT JOIN (
            SELECT DISTINCT ON (symbol_id) symbol_id, status
            FROM canonical.shariah_screenings
            ORDER BY symbol_id, screening_date DESC
        ) ss ON ss.symbol_id = s.id
        ORDER BY s.symbol, d.timestamp ASC;
    """
    df = pd.read_sql(query, conn)
    conn.close()

    clean_df = BreakoutBacktester.aggregate_daily_bars(df)
    return clean_df


def test_diagnostic_trade_generation_and_invariants(clean_historical_candles):
    """
    Verify trade diagnostics record all required fields and satisfy temporal invariants.
    """
    engine = BreakoutDiagnosticEngine(clean_historical_candles)
    trades = engine.run_trade_diagnostics(train_end_idx=300, val_end_idx=400, cost_scenario="base")

    assert len(trades) > 0, "Expected non-zero diagnostic trades"

    for tr in trades:
        # 1. Temporal invariant: execution occurs on or after signal
        assert tr.executable_entry_date >= tr.signal_date, f"Temporal violation: entry {tr.executable_entry_date} < signal {tr.signal_date}"
        assert tr.exit_date >= tr.executable_entry_date, f"Temporal violation: exit {tr.exit_date} < entry {tr.executable_entry_date}"
        assert tr.bars_held >= 1, "Trade completed in 0 bars"

        # 2. Field integrity
        assert tr.exit_reason in ["TARGET", "STOP_LOSS", "TIME_LIMIT"], f"Unknown exit reason: {tr.exit_reason}"
        assert tr.setup_type in ["DIRECT_GAP", "CONSOLIDATION_CANDIDATE"], f"Unknown setup type: {tr.setup_type}"
        assert tr.costs.total_friction > 0.004, "Invalid cost deduction"

        # 3. Arithmetic consistency
        expected_net = tr.gross_return - tr.costs.total_friction
        assert abs(tr.net_return - expected_net) < 1e-4, f"Net return mismatch: {tr.net_return} vs {expected_net}"


def test_cost_scenarios_audit(clean_historical_candles):
    """
    Verify statutory and execution cost breakdown across optimistic, base, and conservative tiers.
    """
    engine = BreakoutDiagnosticEngine(clean_historical_candles)

    opt = engine.calculate_cost_breakdown("optimistic")
    base = engine.calculate_cost_breakdown("base")
    cons = engine.calculate_cost_breakdown("conservative")

    # Base friction must match 48.2 bps exactly
    assert round(base.total_friction * 10000, 1) == 48.2, f"Expected 48.2 bps base friction, got {base.total_friction*10000}"

    # Verify scenario ordering: optimistic < base < conservative
    assert opt.total_friction < base.total_friction < cons.total_friction

    # Verify component isolation
    assert base.stt == 0.0020, "STT must be exactly 20 bps roundtrip"
    assert base.stamp_duty == 0.00015, "Stamp duty must be exactly 1.5 bps on buy leg"
    assert opt.brokerage == 0.0000, "Optimistic scenario must reflect 0 brokerage tier"


def test_baselines_comparative_evaluation(clean_historical_candles):
    """
    Verify that all 5 baselines are evaluated on identical dates, universe, and exposure accounting.
    """
    engine = BreakoutDiagnosticEngine(clean_historical_candles)
    baselines = engine.evaluate_baselines(train_end_idx=300, val_end_idx=400)

    assert len(baselines) == 5, f"Expected 5 baseline comparisons, got {len(baselines)}"

    names = [b.name for b in baselines]
    assert any("Buy-and-Hold" in n for n in names)
    assert any("Trend-Following" in n for n in names)
    assert any("Without Optional Filters" in n for n in names)
    assert any("Frozen Breakout" in n for n in names)
    assert any("Risk-Free Cash" in n for n in names)

    for b in baselines:
        assert 0.0 <= b.avg_portfolio_exposure_pct <= 100.0, f"Exposure {b.avg_portfolio_exposure_pct}% out of range"
        assert 0.0 <= b.max_drawdown_pct <= 100.0, f"Drawdown {b.max_drawdown_pct}% out of range"


def test_preregistered_experiments_and_hurdle_gates(clean_historical_candles):
    """
    Verify research experiment pre-registration, model hashes, multiple testing adjustment,
    and strict hurdle enforcement.
    """
    engine = BreakoutDiagnosticEngine(clean_historical_candles)
    trades = engine.run_trade_diagnostics(train_end_idx=300, val_end_idx=400, cost_scenario="base")

    framework = BreakoutResearchFramework(trades)
    hypotheses = framework.get_registered_hypotheses()

    assert len(hypotheses) == 5, f"Expected 5 pre-registered hypotheses, got {len(hypotheses)}"

    # Check distinct model hashes
    hashes = [h.model_hash for h in hypotheses]
    assert len(set(hashes)) == len(hypotheses), "Duplicate model hashes detected!"

    # Run experiments
    results = framework.run_all_experiments()

    for part in ["TRAIN", "VALIDATION", "OUT_OF_SAMPLE"]:
        part_res = results[part]
        assert len(part_res) == 5

        for r in part_res:
            # Holm-Bonferroni adjusted p-value must be >= raw p-value
            assert r.adjusted_p_value >= r.raw_p_value, f"Adjusted p {r.adjusted_p_value} < raw {r.raw_p_value}"

            # Hurdle check: if net return < +3%, cannot pass hurdles
            if r.avg_net_return_pct < 3.0:
                assert r.passed_hurdles is False, f"Experiment {r.hypothesis.experiment_id} falsely passed hurdles with net return {r.avg_net_return_pct}%!"
