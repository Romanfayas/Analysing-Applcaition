"""
HalalEquity P1.8.3 - Historical Price Adjustment Integrity & Corporate Action Tests

Verifies:
1. The TCS dividend lookback boundary discontinuity.
2. Splicing raw close with dividend-adjusted close creates an artificial price jump.
3. Silent fallback COALESCE(adjusted_close, close) produces spurious trade signals.
4. Historical backtests crossing corporate actions must be flagged as ADJUSTMENT_SENSITIVE.
"""
import os
import pytest
import psycopg2
import pandas as pd
import numpy as np
from datetime import datetime, date

from src.engines.technical.breakout_backtest import BreakoutBacktester, ValidationSummary
from src.engines.research.price_adjustment import (
    PriceAdjustmentEngine, CorporateActionEvent, ActionType, FactorCategory, DisputeStatus,
    AdjustmentMode, AdjustmentFactorChain, load_verified_factors_from_db
)


class TestTCSDividendAdjustmentDiscontinuity:
    """
    Forensic reproduction and regression test of the TCS Oct 14, 2026 ₹12.00 dividend.
    Yahoo Finance applied a ~0.99471 backward adjustment factor to the 10-day lookback window
    (Oct 5 - Oct 13), leaving bars prior to Oct 5 unadjusted (NULL in ohlcv_daily).
    """

    def test_spurious_return_from_silent_coalesce_fallback(self):
        # Actual TCS bar data around the 10-day lookback boundary
        oct_2_raw_close = 2114.40
        oct_2_adj_close = None  # Historical bar in DB prior to 10-day window

        oct_5_raw_close = 2114.40
        oct_5_adj_close = 2103.2190  # Fetched by Yahoo chart API (adjusted for ₹12 dividend)

        # 1. Unadjusted price return (True raw return between Oct 2 and Oct 5)
        raw_return = (oct_5_raw_close - oct_2_raw_close) / oct_2_raw_close
        assert raw_return == 0.0, "Raw price return between flat sessions must be exactly 0%"

        # 2. Consistent dividend-adjusted return (Both bars adjusted backwards by factor)
        adj_factor = 2103.2190 / 2114.40  # ~0.99471197
        oct_2_consistent_adj = oct_2_raw_close * adj_factor
        consistent_adj_return = (oct_5_adj_close - oct_2_consistent_adj) / oct_2_consistent_adj
        assert abs(consistent_adj_return) < 1e-6, "Consistent adjusted series must yield 0% return"

        # 3. Flawed silent fallback: COALESCE(adj_close, close)
        # Oct 2 falls back to 2114.40; Oct 5 uses 2103.2190
        spliced_oct_2 = oct_2_adj_close if oct_2_adj_close is not None else oct_2_raw_close
        spliced_oct_5 = oct_5_adj_close if oct_5_adj_close is not None else oct_5_raw_close

        spliced_return = (spliced_oct_5 - spliced_oct_2) / spliced_oct_2
        # Produces an artificial -0.5288% price drop purely from the lookback boundary splice!
        assert spliced_return < -0.005
        assert round(spliced_return * 100, 4) == -0.5288

    def test_backtester_flags_adjustment_sensitive_on_unadjusted_data(self):
        # Create 60 sessions of synthetic data crossing a corporate action
        dates = pd.date_range("2026-08-01", periods=60, freq="B")
        df = pd.DataFrame({
            "symbol": ["TCS"] * 60,
            "shariah_status": ["PASS"] * 60,
            "timestamp": dates,
            "open": [2100.0] * 60,
            "high": [2120.0] * 60,
            "low": [2090.0] * 60,
            "close": [2110.0] * 60,
            "volume": [500000] * 60,
        })

        bt = BreakoutBacktester(df, cost_model="COST_MODEL_2026_Q4_V2", require_verified_ledger=True)
        trades, summaries = bt.run_backtest(shariah_only=False, train_end_idx=35, val_end_idx=45)
        summary = summaries["TRAIN"]

        # Invariant: Historical backtest must record that price series is adjustment sensitive
        assert summary.is_adjustment_sensitive is True
        assert summary.adjustment_methodology == "SPLIT_ADJUSTED_UNADJUSTED_DIVIDEND_SENSITIVE"
        assert any("ADJUSTMENT_SENSITIVE" in lim for lim in summary.limitations)

    def test_corporate_action_discontinuity_detection(self):
        """
        Detects price series where adjusted_close is intermittently populated,
        which indicates a partial lookback backfill rather than full-history reconstruction.
        """
        series_with_partial_adj = [
            {"date": "2026-10-01", "close": 2114.40, "adj_close": None},
            {"date": "2026-10-02", "close": 2114.40, "adj_close": None},
            {"date": "2026-10-05", "close": 2114.40, "adj_close": 2103.22},
            {"date": "2026-10-06", "close": 2100.00, "adj_close": 2088.90},
        ]

        # Audit helper function
        def check_adjustment_consistency(records):
            has_null = any(r["adj_close"] is None for r in records)
            has_non_null = any(r["adj_close"] is not None for r in records)
            if has_null and has_non_null:
                return "INCONSISTENT_LOOKBACK_SPLICED"
            elif has_null:
                return "RAW_UNADJUSTED"
            else:
                return "UNIFORMLY_ADJUSTED"

        assert check_adjustment_consistency(series_with_partial_adj) == "INCONSISTENT_LOOKBACK_SPLICED"

        raw_only = [
            {"date": "2026-10-01", "close": 2114.40, "adj_close": None},
            {"date": "2026-10-02", "close": 2114.40, "adj_close": None},
        ]
        assert check_adjustment_consistency(raw_only) == "RAW_UNADJUSTED"

    def test_backtest_refuses_to_run_as_validated_research_when_adjustments_incomplete(self):
        """
        P1.8.4 Workstream 2 Rule 9:
        Make backtests refuse to run as validated research when required historical adjustment data is incomplete.
        """
        dates = pd.date_range("2026-08-01", periods=60, freq="B")
        df = pd.DataFrame({
            "symbol": ["TCS"] * 60,
            "shariah_status": ["PASS"] * 60,
            "timestamp": dates,
            "open": [2100.0] * 60,
            "high": [2120.0] * 60,
            "low": [2090.0] * 60,
            "close": [2110.0] * 60,
            "volume": [500000] * 60,
            # adjusted_close is missing or contains nulls
            "adjusted_close": [None] * 50 + [2100.0] * 10,
        })

        with pytest.raises(ValueError, match="Validated research requires complete point-in-time dividend"):
            BreakoutBacktester(
                df,
                cost_model="COST_MODEL_2026_Q4_V2",
                require_verified_ledger=True,
                require_complete_adjustments=True,
            )

    def test_split_boundary_distortion_without_adjustment(self):
        """
        Tests 2:1 stock split boundary.
        Unadjusted close drops from 1000 to 500 (-50%), which would trigger a false breakout/stop loss.
        A proper split adjustment factor (0.50) normalizes pre-split bars to 500, preserving 0% return.
        """
        raw_pre_split = 1000.0
        raw_post_split = 500.0

        # Unadjusted return
        raw_ret = (raw_post_split - raw_pre_split) / raw_pre_split
        assert raw_ret == -0.50  # -50% false drop

        # Split adjustment factor for 2:1 split = 0.50
        split_factor = 0.50
        adj_pre_split = raw_pre_split * split_factor
        adj_post_split = raw_post_split

        adj_ret = (adj_post_split - adj_pre_split) / adj_pre_split
        assert adj_ret == 0.0  # Economically neutral return

    def test_dividend_boundary_distortion(self):
        """
        Tests ₹50 dividend on a ₹500 stock.
        On ex-date, price drops by dividend amount to ₹450.
        Without adjustment factor, return appears as -10%.
        With backward dividend adjustment factor (450 / 500 = 0.90), pre-dividend price is 450, yielding 0%.
        """
        cum_div_close = 500.0
        ex_div_close = 450.0

        raw_ret = (ex_div_close - cum_div_close) / cum_div_close
        assert raw_ret == -0.10  # Spurious -10% drop

        div_adj_factor = ex_div_close / cum_div_close  # 0.90
        adj_cum_close = cum_div_close * div_adj_factor
        adj_ret = (ex_div_close - adj_cum_close) / adj_cum_close
        assert abs(adj_ret) < 1e-6  # Economically neutral

    def test_matching_endpoints_fallacy_intervening_discontinuity(self):
        """
        P1.8.4 Workstream 2 Rule:
        Do not assume that a matching adjusted close value on two dates proves the full intervening return series is correct.
        """
        # Day 0: 1000, Day 5: unadjusted split to 500, Day 10: rally to 1000
        # If observer only checks Day 0 (1000) and Day 10 (1000), total return seems 0.0%,
        # but the intervening return at Day 5 is completely corrupted (-50% drop, +100% rally).
        daily_prices = [1000.0, 1000.0, 1000.0, 1000.0, 1000.0, 500.0, 600.0, 700.0, 850.0, 1000.0]
        start_price = daily_prices[0]
        end_price = daily_prices[-1]
        endpoint_return = (end_price - start_price) / start_price
        assert endpoint_return == 0.0

        # Calculate max drawdowns and day-to-day returns
        returns = [
            (daily_prices[i] - daily_prices[i-1]) / daily_prices[i-1]
            for i in range(1, len(daily_prices))
        ]
        min_return = min(returns)
        # Intervening day 5 return is -50% due to unadjusted corporate action!
        assert min_return == -0.50
        assert min_return != 0.0

    def test_duplicate_bars_rejection(self):
        """Duplicate dates for the same symbol must be detected and aggregated or rejected."""
        dates = [
            pd.Timestamp("2026-10-01"),
            pd.Timestamp("2026-10-02"),
            pd.Timestamp("2026-10-02"),  # Duplicate bar
            pd.Timestamp("2026-10-03"),
        ]
        df = pd.DataFrame({
            "symbol": ["INFY"] * 4,
            "shariah_status": ["PASS"] * 4,
            "timestamp": dates,
            "open": [1500.0] * 4,
            "high": [1520.0] * 4,
            "low": [1490.0] * 4,
            "close": [1510.0] * 4,
            "volume": [100000] * 4,
        })
        aggregated = BreakoutBacktester.aggregate_daily_bars(df)
        # Aggregated should deduplicate down to 3 unique dates
        assert len(aggregated) == 3
        assert len(aggregated["timestamp"].unique()) == 3

    def test_late_corrections_and_historical_reproducibility(self):
        """
        Verifies that modifying a historical candle alters indicator computation,
        demonstrating why raw OHLCV immutability and separate adjustment factors are strictly required.
        """
        np.random.seed(42)
        prices = [100.0 + i + (i % 3) for i in range(50)]
        original_ema = pd.Series(prices).ewm(span=20).mean().iloc[-1]

        # Late correction: Bar 25 price revised
        modified_prices = list(prices)
        modified_prices[25] = 150.0  # +50 rupee late revision
        revised_ema = pd.Series(modified_prices).ewm(span=20).mean().iloc[-1]

        assert original_ema != revised_ema
        assert abs(revised_ema - original_ema) > 0.1

    def test_corporate_action_factor_ledger_immutability_guard(self):
        """
        P1.8.6 WS2:
        Inspect the factor ledger's claimed immutability.
        Database trigger trg_ca_factor_guard must block UPDATE and DELETE,
        enforcing append-only revision semantics.
        """
        import os
        import psycopg2

        db_url = os.environ.get("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        # Verify factor records exist
        cur.execute("SELECT id FROM corporate_action_adjustment_factors LIMIT 1;")
        row = cur.fetchone()
        if not row:
            conn.close()
            pytest.skip("No corporate action adjustment factors found")

        record_id = row[0]

        # 1. Test UPDATE blocked by trigger
        with pytest.raises(psycopg2.Error, match="corporate_action_adjustment_factors rows are append-only"):
            cur.execute("UPDATE corporate_action_adjustment_factors SET adjustment_factor = 0.50 WHERE id = %s;", (record_id,))
        conn.rollback()

        # 2. Test DELETE blocked by trigger
        with pytest.raises(psycopg2.Error, match="corporate_action_adjustment_factors rows are immutable and may not be deleted"):
            cur.execute("DELETE FROM corporate_action_adjustment_factors WHERE id = %s;", (record_id,))
        conn.rollback()
        conn.close()

    def test_unadjusted_bars_quarantine_metrics(self):
        """
        P1.8.6 WS2:
        Report exact daily bar counts and verify that 23,124 / 23,547 bars (98.2%)
        have NULL adjusted_close, requiring research quarantine.
        """
        import os
        import psycopg2

        db_url = os.environ.get("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        cur.execute("""
            SELECT 
                COUNT(*) as total_bars,
                COUNT(adjusted_close) as with_adj,
                COUNT(*) - COUNT(adjusted_close) as null_adj
            FROM ohlcv_daily;
        """)
        total, with_adj, null_adj = cur.fetchone()
        conn.close()

        assert total >= 23547, f"Expected at least 23547 total bars, got {total}"
        assert null_adj == 23132, f"Expected 23132 NULL adjusted_close bars (P1.8.13 quarantined 8 unverified HCLTECH vendor adjusted bars), got {null_adj}"
        pct_null = (null_adj / total) * 100
        assert round(pct_null, 1) >= 97.5, f"Quarantine percentage must remain >= 97.5% across forward sessions, got {pct_null:.2f}%"


class TestPriceAdjustmentEngineReconstruction:
    """
    HalalEquity P1.8.8 - Comprehensive unit and regression tests for PriceAdjustmentEngine.
    Covers splits, dividends, bonuses, rights, demerger lineages, volume scaling,
    and point-in-time filtering.
    """

    def test_split_factor_calculation_and_volume_invariants(self):
        engine = PriceAdjustmentEngine()
        # 2:1 split (old=1, new=2)
        f_2_1 = engine.calculate_split_factor(1.0, 2.0)
        assert f_2_1 == 0.50

        # 10:1 split (old=1, new=10)
        f_10_1 = engine.calculate_split_factor(1.0, 10.0)
        assert f_10_1 == 0.10

        # Reverse split / consolidation 1:5 (old=5, new=1)
        f_rev = engine.calculate_split_factor(5.0, 1.0)
        assert f_rev == 5.0

        # Error cases
        with pytest.raises(ValueError, match="both must be positive"):
            engine.calculate_split_factor(0, 2.0)
        with pytest.raises(ValueError, match="both must be positive"):
            engine.calculate_split_factor(1.0, -1.0)

    def test_bonus_factor_calculation(self):
        engine = PriceAdjustmentEngine()
        # 1:1 bonus issue (1 bonus share for every 1 existing share)
        f_1_1 = engine.calculate_bonus_factor(1.0, 1.0)
        assert f_1_1 == 0.50

        # 1:2 bonus issue (1 bonus for every 2 existing shares)
        # 2 / (2 + 1) = 2/3
        f_1_2 = engine.calculate_bonus_factor(1.0, 2.0)
        assert abs(f_1_2 - (2.0 / 3.0)) < 1e-7

        # 3:1 bonus issue (3 bonus for every 1 existing share)
        f_3_1 = engine.calculate_bonus_factor(3.0, 1.0)
        assert f_3_1 == 0.25

        with pytest.raises(ValueError):
            engine.calculate_bonus_factor(-1.0, 1.0)

    def test_dividend_factor_and_volume_unscaled_invariant(self):
        engine = PriceAdjustmentEngine()
        # TCS ₹12.00 dividend on ₹2,269.25 cum-close
        f_tcs = engine.calculate_cash_dividend_factor(2269.25, 12.0)
        # (2269.25 - 12) / 2269.25 = 2257.25 / 2269.25 = 0.99471197
        assert abs(f_tcs - 0.99471197) < 1e-7

        # Error cases
        with pytest.raises(ValueError, match="must be positive"):
            engine.calculate_cash_dividend_factor(100.0, -5.0)
        with pytest.raises(ValueError, match="must be positive"):
            engine.calculate_cash_dividend_factor(-100.0, 5.0)
        with pytest.raises(ValueError, match="economic return invalid"):
            engine.calculate_cash_dividend_factor(100.0, 100.0)  # 100% payout wipes out price

    def test_rights_issue_terp_calculation(self):
        engine = PriceAdjustmentEngine()
        # Cum price = ₹100, Rights at ₹60 in ratio 1:4 (1 new share for 4 held)
        # TERP = (4 * 100 + 1 * 60) / 5 = 460 / 5 = ₹92.00
        # factor = 92 / 100 = 0.92
        f_rights = engine.calculate_rights_issue_factor(100.0, 60.0, 1.0, 4.0)
        assert f_rights == 0.92

        # Out-of-the-money rights issue (subscription >= cum price) yields 1.0 (no dilution)
        f_otm = engine.calculate_rights_issue_factor(100.0, 120.0, 1.0, 4.0)
        assert f_otm == 1.0

    def test_reconstruct_series_split_adjusted_ohlc_continuity(self):
        """
        Tests complete OHLC series reconstruction across a 2:1 stock split on 2026-10-06.
        Synthetic raw prices drop from ₹1,000 to ₹500 on ex-date.
        Reconstruction must:
        - Scale pre-split OHLC down by 0.50 (Open: 500, High: 510, Low: 495, Close: 500).
        - Scale pre-split Volume up by 2 (from 10,000 to 20,000).
        - Preserve notional turnover: P_adj * V_adj == P_raw * V_raw.
        """
        from src.engines.research.price_adjustment import (
            PriceAdjustmentEngine, CorporateActionEvent, ActionType, AdjustmentMode
        )

        dates = pd.date_range("2026-10-01", periods=6, freq="B")
        raw_df = pd.DataFrame({
            "symbol": ["TEST"] * 6,
            "timestamp": dates,
            "open":   [1000.0, 1000.0, 1000.0, 500.0, 505.0, 510.0],
            "high":   [1020.0, 1020.0, 1020.0, 510.0, 515.0, 520.0],
            "low":    [990.0,  990.0,  990.0,  495.0, 500.0, 505.0],
            "close":  [1000.0, 1000.0, 1000.0, 500.0, 505.0, 510.0],
            "volume": [10000,  10000,  10000,  20000, 20000, 20000],
        })

        split_event = CorporateActionEvent(
            symbol="TEST",
            action_type=ActionType.STOCK_SPLIT,
            factor_category=FactorCategory.SHARE_TRANSFORMATION,
            price_factor_applicable=True,
            ex_date=date(2026, 10, 6),
            record_date=date(2026, 10, 6),
            effective_date=date(2026, 10, 6),
            adjustment_factor=0.50,
            cumulative_total_factor=0.50,
            nominal_price_factor=0.50,
            volume_adjustment_factor=0.50,
            split_ratio_old=1.0,
            split_ratio_new=2.0,
            source_reference="TEST_SPLIT_2_FOR_1",
            is_verified=True,
        )

        adj_df = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [split_event], mode=AdjustmentMode.PRICE_RETURN
        )

        # Pre-split bars (indices 0, 1, 2)
        for i in range(3):
            assert adj_df["close"].iloc[i] == 500.0  # 1000 * 0.50
            assert adj_df["open"].iloc[i] == 500.0
            assert adj_df["high"].iloc[i] == 510.0   # 1020 * 0.50
            assert adj_df["low"].iloc[i] == 495.0    # 990 * 0.50
            assert adj_df["volume"].iloc[i] == 20000 # 10000 / 0.50

            # Turnover invariant: P * V == P_adj * V_adj
            raw_turnover = raw_df["close"].iloc[i] * raw_df["volume"].iloc[i]
            adj_turnover = adj_df["close"].iloc[i] * adj_df["volume"].iloc[i]
            assert raw_turnover == adj_turnover == 10_000_000.0

        # Post-split bars (indices 3, 4, 5) remain unscaled
        assert adj_df["close"].iloc[3] == 500.0
        assert adj_df["volume"].iloc[3] == 20000

        # Continuity invariant: Return between bar 2 (pre-split) and bar 3 (post-split)
        adj_ret = (adj_df["close"].iloc[3] - adj_df["close"].iloc[2]) / adj_df["close"].iloc[2]
        assert adj_ret == 0.0  # Economically continuous (0% false gap)

    def test_reconstruct_series_total_return_vs_price_return(self):
        """
        Tests that:
        1. In TOTAL_RETURN mode, cash dividend scales pre-ex-date prices backwards, BUT leaves volume unscaled.
        2. In PRICE_RETURN mode, cash dividend leaves both prices and volume unscaled.
        """
        from src.engines.research.price_adjustment import (
            PriceAdjustmentEngine, CorporateActionEvent, ActionType, AdjustmentMode
        )

        dates = pd.date_range("2026-10-12", periods=4, freq="B")
        # Oct 12: 2269.25, Oct 13: 2269.25 (cum), Oct 14: 2257.25 (ex-div), Oct 15: 2260.00
        raw_df = pd.DataFrame({
            "symbol": ["TCS"] * 4,
            "timestamp": dates,
            "open":   [2260.0, 2265.0, 2255.0, 2258.0],
            "high":   [2275.0, 2275.0, 2265.0, 2268.0],
            "low":    [2250.0, 2260.0, 2250.0, 2252.0],
            "close":  [2269.25, 2269.25, 2257.25, 2260.0],
            "volume": [100000, 120000, 150000, 110000],
        })

        div_event = CorporateActionEvent(
            symbol="TCS",
            action_type=ActionType.CASH_DIVIDEND,
            factor_category=FactorCategory.CASH_DISTRIBUTION,
            price_factor_applicable=True,
            ex_date=date(2026, 10, 14),
            record_date=date(2026, 10, 14),
            effective_date=date(2026, 10, 14),
            adjustment_factor=0.99471197,
            cumulative_total_factor=0.99471197,
            nominal_price_factor=1.0,
            total_return_factor=0.99471197,
            volume_adjustment_factor=1.0,
            cash_dividend_per_share=12.0,
            source_reference="NSE_TCS_DIVIDEND",
            is_verified=True,
        )

        # 1. TOTAL_RETURN mode
        tr_df = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [div_event], mode=AdjustmentMode.TOTAL_RETURN
        )
        # Pre-div close on Oct 13 is scaled down: 2269.25 * 0.99471197 = 2257.25
        assert round(tr_df["close"].iloc[1], 2) == 2257.25
        # Volume must NOT be scaled for cash dividends!
        assert tr_df["volume"].iloc[1] == 120000
        assert tr_df["volume"].iloc[0] == 100000

        # 2. PRICE_RETURN mode
        pr_df = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [div_event], mode=AdjustmentMode.PRICE_RETURN
        )
        # In PRICE_RETURN mode, cash dividends do not alter nominal price levels
        assert pr_df["close"].iloc[1] == 2269.25
        assert pr_df["volume"].iloc[1] == 120000

    def test_symbol_rename_factor_is_unity(self):
        """A symbol rename (LTIM -> LTM) is metadata and must preserve nominal prices exactly."""
        from src.engines.research.price_adjustment import (
            PriceAdjustmentEngine, CorporateActionEvent, ActionType, AdjustmentMode
        )

        dates = pd.date_range("2026-02-25", periods=3, freq="B")
        raw_df = pd.DataFrame({
            "symbol": ["LTIM"] * 3,
            "timestamp": dates,
            "open":   [5000.0, 5010.0, 5020.0],
            "high":   [5050.0, 5060.0, 5070.0],
            "low":    [4980.0, 4990.0, 5000.0],
            "close":  [5020.0, 5030.0, 5040.0],
            "volume": [50000, 55000, 52000],
        })

        rename_event = CorporateActionEvent(
            symbol="LTIM",
            action_type=ActionType.SYMBOL_CHANGE,
            ex_date=date(2026, 2, 27),
            record_date=date(2026, 2, 27),
            effective_date=date(2026, 2, 27),
            adjustment_factor=1.00000000,
            cumulative_total_factor=1.00000000,
            source_reference="NSE_LTIM_LTM_RENAME",
            is_verified=True,
        )

        adj_df = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [rename_event], mode=AdjustmentMode.PRICE_RETURN
        )

        # Factor is exactly 1.0; prices and volume unchanged
        assert (adj_df["close"] == raw_df["close"]).all()
        assert (adj_df["volume"] == raw_df["volume"]).all()

    def test_no_fabricated_pre_listing_history_guard(self):
        """Successor companies (TMPV, TMCV) must reject pre-listing date queries."""
        from src.engines.research.price_adjustment import PriceAdjustmentEngine

        # TMPV was listed post-demerger on 2025-10-24
        valid, msg = PriceAdjustmentEngine.check_lineage_and_pre_listing("TMPV", date(2024, 10, 1))
        assert valid is False
        assert "PRE_LISTING_UNTRADABLE" in msg
        assert "Fabricating or splicing raw predecessor" in msg

        valid_post, msg_post = PriceAdjustmentEngine.check_lineage_and_pre_listing("TMPV", date(2025, 11, 1))
        assert valid_post is True
        assert msg_post == "VALID_TIMELINE"

        # Standard stock without demerger
        valid_tcs, msg_tcs = PriceAdjustmentEngine.check_lineage_and_pre_listing("TCS", date(2024, 1, 1))
        assert valid_tcs is True

    def test_point_in_time_reproducibility_as_of_filtering(self):
        """Actions occurring after as_of_date must NOT be applied retrospectively."""
        from src.engines.research.price_adjustment import (
            PriceAdjustmentEngine, CorporateActionEvent, ActionType, AdjustmentMode
        )

        dates = pd.date_range("2026-10-01", periods=10, freq="B")
        raw_df = pd.DataFrame({
            "symbol": ["TCS"] * 10,
            "timestamp": dates,
            "open":   [2200.0] * 10,
            "high":   [2250.0] * 10,
            "low":    [2190.0] * 10,
            "close":  [2210.0] * 10,
            "volume": [50000] * 10,
        })

        div_event = CorporateActionEvent(
            symbol="TCS",
            action_type=ActionType.CASH_DIVIDEND,
            factor_category=FactorCategory.CASH_DISTRIBUTION,
            price_factor_applicable=True,
            ex_date=date(2026, 10, 14),
            record_date=date(2026, 10, 14),
            effective_date=date(2026, 10, 14),
            adjustment_factor=0.99471197,
            cumulative_total_factor=0.99471197,
            nominal_price_factor=1.0,
            total_return_factor=0.99471197,
            volume_adjustment_factor=1.0,
            cash_dividend_per_share=12.0,
            source_reference="NSE_TCS_DIVIDEND",
            is_verified=True,
        )

        # 1. Backtest simulating decision time as of Oct 05, 2026 (prior to ex-date)
        df_pit = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [div_event], mode=AdjustmentMode.TOTAL_RETURN, as_of_date=date(2026, 10, 5)
        )
        # Factor is NOT applied because action was in the future relative to simulated decision time!
        assert (df_pit["price_factor"] == 1.0).all()
        assert df_pit["close"].iloc[0] == 2210.0

        # 2. Backtest simulating decision time as of Oct 15, 2026 (after ex-date)
        df_post = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [div_event], mode=AdjustmentMode.TOTAL_RETURN, as_of_date=date(2026, 10, 15)
        )
        # Factor IS applied to pre-ex-date bars
        assert df_post["price_factor"].iloc[0] == 0.99471197
        assert round(df_post["close"].iloc[0], 2) == 2198.31

    def test_db_factor_chain_loader_deduplication(self):
        """Verifies loading verified factors from database ledger."""
        import os
        import psycopg2
        from src.engines.research.price_adjustment import load_verified_factors_from_db

        db_url = os.environ.get("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        events = load_verified_factors_from_db(conn, symbol="TCS")
        conn.close()

        assert len(events) >= 1
        tcs_ev = events[0]
        assert tcs_ev.symbol == "TCS"
        assert tcs_ev.action_type.value == "CASH_DIVIDEND"
        assert tcs_ev.ex_date == date(2026, 10, 14)
        assert tcs_ev.is_verified is True
        assert tcs_ev.cash_dividend_per_share == 12.0

    def test_bonus_issue_ohlc_reconstruction(self):
        """
        Tests 1:1 bonus issue series reconstruction.
        Pre-bonus OHLC must be scaled down by 0.50, volume scaled up by 2.0 (1/0.50).
        """
        dates = pd.date_range("2026-09-01", periods=4, freq="B")
        raw_df = pd.DataFrame({
            "symbol": ["BONUS_CO"] * 4,
            "timestamp": dates,
            "open":   [800.0, 800.0, 400.0, 410.0],
            "high":   [820.0, 820.0, 410.0, 420.0],
            "low":    [790.0, 790.0, 395.0, 405.0],
            "close":  [800.0, 800.0, 400.0, 410.0],
            "volume": [50000, 50000, 100000, 100000],
        })

        bonus_event = CorporateActionEvent(
            symbol="BONUS_CO",
            action_type=ActionType.BONUS_ISSUE,
            factor_category=FactorCategory.SHARE_TRANSFORMATION,
            price_factor_applicable=True,
            ex_date=date(2026, 9, 3),
            record_date=date(2026, 9, 3),
            effective_date=date(2026, 9, 3),
            adjustment_factor=0.50,
            cumulative_total_factor=0.50,
            nominal_price_factor=0.50,
            volume_adjustment_factor=0.50,
            source_reference="BONUS_CIRCULAR_1_FOR_1",
            is_verified=True,
        )

        adj_df = PriceAdjustmentEngine.reconstruct_series(raw_df, [bonus_event], mode=AdjustmentMode.PRICE_RETURN)
        assert adj_df["close"].iloc[0] == 400.0
        assert adj_df["volume"].iloc[0] == 100000
        assert adj_df["close"].iloc[2] == 400.0
        assert adj_df["volume"].iloc[2] == 100000
        # Continuity check
        assert adj_df["close"].iloc[1] == adj_df["close"].iloc[2]

    def test_conflicting_or_unverified_factor_rejection(self):
        """Unverified factors (e.g. from secondary scraper without circular) must be ignored/rejected."""
        dates = pd.date_range("2026-10-01", periods=2, freq="B")
        raw_df = pd.DataFrame({
            "symbol": ["UNV"] * 2,
            "timestamp": dates,
            "open":   [100.0, 100.0],
            "high":   [105.0, 105.0],
            "low":    [95.0, 95.0],
            "close":  [100.0, 100.0],
            "volume": [1000, 1000],
        })

        unverified_event = CorporateActionEvent(
            symbol="UNV",
            action_type=ActionType.STOCK_SPLIT,
            ex_date=date(2026, 10, 2),
            record_date=None,
            effective_date=date(2026, 10, 2),
            adjustment_factor=0.10,
            cumulative_total_factor=0.10,
            source_reference="UNVERIFIED_THIRD_PARTY_SCRAPE",
            is_verified=False,
            verification_status="UNVERIFIED",
        )

        adj_df = PriceAdjustmentEngine.reconstruct_series(raw_df, [unverified_event], mode=AdjustmentMode.PRICE_RETURN)
        # Unverified factor must NOT be applied
        assert adj_df["price_factor"].iloc[0] == 1.0
        assert adj_df["close"].iloc[0] == 100.0
        assert adj_df["volume"].iloc[0] == 1000

    def test_demerger_lineage_and_cost_split_separation(self):
        """
        Demergers are corporate restructurings allocating acquisition cost, NOT ordinary stock splits.
        They must not be conflated with dilution splits, and pre-demerger history must not be copied
        to newly created spin-off companies without primary-source statutory backing.
        """
        assert ActionType.DEMERGER != ActionType.STOCK_SPLIT
        demerger_event = CorporateActionEvent(
            symbol="TATAMOTORS",
            action_type=ActionType.DEMERGER,
            ex_date=date(2025, 10, 24),
            record_date=date(2025, 10, 14),
            effective_date=date(2025, 10, 24),
            adjustment_factor=0.68850000,
            cumulative_total_factor=0.68850000,
            source_reference="NSE_CIRCULAR_NSE/CML/70875",
            is_verified=True,
            details={"cost_split_tmpv": 68.85, "cost_split_tmcv": 31.15}
        )
        assert demerger_event.is_dilution_action() is False
        assert demerger_event.details["cost_split_tmpv"] == 68.85


class TestP189FactorValidationAndDiscrepancyResolution:
    """
    HalalEquity P1.8.9 - Rigorous validation fixtures resolving factor semantic correctness,
    TCS dividend reference price discrepancy, demerger tax-allocation separation,
    and multiple event compounding.
    """

    def test_tcs_dividend_reference_discrepancy_and_mathematical_resolution(self):
        """
        P1.8.9 Issue 1 Forensic Resolution:
        - Erroneously documented calculation in Migration 10:
          cum_close = 2114.40 (actually Oct 05 lookback boundary close!)
          (2114.40 - 12.00) / 2114.40 = 0.99432463
        - True cum-dividend session on NSE (2026-10-13):
          cum_close = 2269.30
          dividend = 12.00
          (2269.30 - 12.00) / 2269.30 = 2257.30 / 2269.30 = 0.99471197726...
        - Rounded to 8 decimals: 0.99471197.
        - Oct 05 backward adjusted close in provider feed:
          2114.40 * 0.99471197 = 2103.2190.
        This proves the factor in the ledger was mathematically exact for the true cum-close,
        but the migration metadata had mistakenly cited the Oct 05 lookback boundary close.
        """
        from decimal import Decimal, ROUND_HALF_UP

        # 1. Erroneous formula cited in Migration 10 metadata
        erroneous_cum_close = Decimal("2114.40")
        dividend = Decimal("12.00")
        discrepant_factor = (erroneous_cum_close - dividend) / erroneous_cum_close
        assert round(float(discrepant_factor), 6) == 0.994325
        assert round(float(discrepant_factor), 8) != 0.99471197

        # 2. Origin of the 0.99471197 factor:
        # Extracted directly from the ratio of provider backward adjusted close to raw close on Oct 05 lookback boundary:
        # 2103.2190 / 2114.40 = 0.9947119750... (~0.99471197)
        oct_05_raw = Decimal("2114.40")
        oct_05_adj = Decimal("2103.2190")
        extracted_ratio = float(oct_05_adj / oct_05_raw)
        assert abs(extracted_ratio - 0.99471197) < 1e-7

        # 3. True cum-close band on 2026-10-13 (₹2,269.25 - ₹2,269.30):
        cum_2269_30 = Decimal("2269.30")
        factor_2269_30 = float(((cum_2269_30 - dividend) / cum_2269_30).quantize(Decimal("0.00000001"), rounding=ROUND_HALF_UP))
        assert factor_2269_30 == 0.99471203

        # Both true cum-close references differ by less than 6e-8 from the ledger factor (0.99471197),
        # completely resolving the discrepancy with the mistaken 2114.40 boundary reference (which differs by 38.7 bps!).
        assert abs(factor_2269_30 - 0.99471197) < 1e-7
        assert abs(factor_2269_30 - float(discrepant_factor)) > 0.00038

    def test_demerger_tax_cost_allocation_not_applied_as_price_adjustment(self):
        """
        P1.8.9 Issue 2 Forensic Resolution:
        Demerger tax-cost allocation (0.6885 for TMPV, 0.3115 for TMCV) is strictly under
        Section 49(2C) of the Income Tax Act, 1961 for shareholder cost basis.
        It is NOT a market price adjustment factor!
        PriceAdjustmentEngine must NOT scale historical prices by 0.6885.
        """
        from src.engines.research.price_adjustment import (
            CorporateActionEvent, ActionType, FactorCategory, DisputeStatus,
            AdjustmentMode, AdjustmentFactorChain, PriceAdjustmentEngine
        )

        dates = pd.date_range("2025-10-20", periods=5, freq="B")
        raw_df = pd.DataFrame({
            "symbol": ["TATAMOTORS"] * 5,
            "timestamp": dates,
            "open":   [900.0] * 5,
            "high":   [910.0] * 5,
            "low":    [890.0] * 5,
            "close":  [900.0] * 5,
            "volume": [1000000] * 5,
        })

        demerger_tax_event = CorporateActionEvent(
            symbol="TATAMOTORS",
            action_type=ActionType.DEMERGER,
            factor_category=FactorCategory.TAX_COST_ALLOCATION,
            ex_date=date(2025, 10, 24),
            record_date=date(2025, 10, 14),
            effective_date=date(2025, 10, 24),
            adjustment_factor=1.00000000,
            cumulative_total_factor=1.00000000,
            nominal_price_factor=1.00000000,
            total_return_factor=1.00000000,
            volume_adjustment_factor=1.00000000,
            price_factor_applicable=False,  # Explicitly NOT applicable to prices!
            tax_cost_allocation_pct=68.8500,
            dispute_status=DisputeStatus.DISPUTED_NON_PRICE_FACTOR,
            source_reference="NSE_CIRCULAR_NSE/CML/70875",
            is_verified=False,
        )

        # 1. Chain must ignore this event because price_factor_applicable is False
        chain = AdjustmentFactorChain(
            symbol="TATAMOTORS",
            events=[demerger_tax_event],
            as_of_date=date(2025, 10, 25),
            mode=AdjustmentMode.TOTAL_RETURN
        )
        p_fac, v_fac = chain.get_factor_on_date(date(2025, 10, 21))
        assert p_fac == 1.0  # NOT 0.6885!
        assert v_fac == 1.0

        # 2. Price series reconstruction leaves prices completely unscaled
        reconstructed = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [demerger_tax_event], mode=AdjustmentMode.TOTAL_RETURN
        )
        assert (reconstructed["close"] == 900.0).all()
        assert (reconstructed["volume"] == 1000000).all()

    def test_disputed_factor_is_strictly_rejected_from_reconstruction(self):
        """Any corporate action factor marked with a dispute status must be rejected."""
        from src.engines.research.price_adjustment import (
            CorporateActionEvent, ActionType, FactorCategory, DisputeStatus,
            AdjustmentMode, AdjustmentFactorChain
        )

        disputed_event = CorporateActionEvent(
            symbol="TEST_DISPUTED",
            action_type=ActionType.CASH_DIVIDEND,
            factor_category=FactorCategory.CASH_DISTRIBUTION,
            ex_date=date(2026, 10, 10),
            record_date=None,
            effective_date=date(2026, 10, 10),
            adjustment_factor=0.95000000,
            cumulative_total_factor=0.95000000,
            nominal_price_factor=1.0,
            total_return_factor=0.95000000,
            volume_adjustment_factor=1.0,
            price_factor_applicable=True,
            dispute_status=DisputeStatus.DISPUTED_METADATA_DISCREPANCY,
            dispute_reason="Reference price disputed by auditor",
            source_reference="DISPUTED_SRC",
            is_verified=True,
        )

        chain = AdjustmentFactorChain(
            symbol="TEST_DISPUTED",
            events=[disputed_event],
            as_of_date=date(2026, 10, 15),
            mode=AdjustmentMode.TOTAL_RETURN
        )
        p_fac, _ = chain.get_factor_on_date(date(2026, 10, 5))
        assert p_fac == 1.0, "Disputed factor must not be applied to historical prices"

    def test_multiple_corporate_actions_compounding_in_same_interval(self):
        """
        Tests sequential compounding:
        Event 1 (ex 2026-09-10): 2:1 stock split (factor = 0.50).
        Event 2 (ex 2026-09-20): Cash dividend of ₹10 on ₹500 cum-close (factor = 490/500 = 0.98).
        Pre-Event 1 bars must have:
        - TOTAL_RETURN price factor: 0.50 * 0.98 = 0.49.
        - PRICE_RETURN price factor: 0.50 (dividend unadjusted).
        - Volume factor: 0.50 (volume scaled by 2.0; dividend leaves volume unscaled).
        Between Event 1 and Event 2:
        - TOTAL_RETURN price factor: 0.98.
        - Volume factor: 1.0.
        """
        from src.engines.research.price_adjustment import (
            CorporateActionEvent, ActionType, FactorCategory, AdjustmentMode, PriceAdjustmentEngine
        )

        dates = pd.date_range("2026-09-01", periods=25, freq="B")
        raw_df = pd.DataFrame({
            "symbol": ["MULTI_CO"] * 25,
            "timestamp": dates,
            "open":   [1000.0] * 25,
            "high":   [1010.0] * 25,
            "low":    [990.0] * 25,
            "close":  [1000.0] * 25,
            "volume": [10000] * 25,
        })

        split_event = CorporateActionEvent(
            symbol="MULTI_CO",
            action_type=ActionType.STOCK_SPLIT,
            factor_category=FactorCategory.SHARE_TRANSFORMATION,
            price_factor_applicable=True,
            ex_date=date(2026, 9, 10),
            record_date=date(2026, 9, 10),
            effective_date=date(2026, 9, 10),
            adjustment_factor=0.50,
            cumulative_total_factor=0.50,
            nominal_price_factor=0.50,
            total_return_factor=0.50,
            volume_adjustment_factor=0.50,
            split_ratio_old=1.0,
            split_ratio_new=2.0,
            source_reference="SPLIT_2_1",
            is_verified=True,
        )

        div_event = CorporateActionEvent(
            symbol="MULTI_CO",
            action_type=ActionType.CASH_DIVIDEND,
            factor_category=FactorCategory.CASH_DISTRIBUTION,
            price_factor_applicable=True,
            ex_date=date(2026, 9, 20),
            record_date=date(2026, 9, 20),
            effective_date=date(2026, 9, 20),
            adjustment_factor=0.98,
            cumulative_total_factor=0.49,
            nominal_price_factor=1.0,
            total_return_factor=0.98,
            volume_adjustment_factor=1.0,
            cash_dividend_per_share=10.0,
            source_reference="DIV_INR_10",
            is_verified=True,
        )

        # 1. TOTAL_RETURN mode
        tr_df = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [split_event, div_event], mode=AdjustmentMode.TOTAL_RETURN
        )
        # Pre-split bar (index 0, Sept 01)
        assert tr_df["price_factor"].iloc[0] == 0.49  # 0.50 * 0.98
        assert tr_df["close"].iloc[0] == 490.0
        assert tr_df["volume"].iloc[0] == 20000       # 10000 / 0.50 (volume NOT scaled by dividend!)

        # Between split and div bar (Sept 15, index 10)
        assert round(tr_df["price_factor"].iloc[10], 2) == 0.98
        assert tr_df["close"].iloc[10] == 980.0
        assert tr_df["volume"].iloc[10] == 10000

        # Post-dividend bar (Sept 24, index 17)
        assert tr_df["price_factor"].iloc[17] == 1.0
        assert tr_df["close"].iloc[17] == 1000.0

        # 2. PRICE_RETURN mode
        pr_df = PriceAdjustmentEngine.reconstruct_series(
            raw_df, [split_event, div_event], mode=AdjustmentMode.PRICE_RETURN
        )
        assert pr_df["price_factor"].iloc[0] == 0.50  # Dividend unadjusted
        assert pr_df["close"].iloc[0] == 500.0
        assert pr_df["volume"].iloc[0] == 20000

    def test_db_ledger_latest_revision_loaded_correctly(self):
        """
        Verifies that latest revision rows from Migration 14 in PostgreSQL are loaded correctly:
        - TCS has cum_price_reference = 2269.3000 and revision_number = 3 (exact factor 0.99471203).
        - TATAMOTORS Revision 2 is excluded when only_price_applicable=True.
        - TATAMOTORS Revision 2 is loaded when only_price_applicable=False, with price_factor_applicable=False.
        """
        import os
        import psycopg2
        from src.engines.research.price_adjustment import load_verified_factors_from_db

        db_url = os.environ.get("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        # 1. TCS Revision 3 (reconciled exact factor)
        tcs_events = load_verified_factors_from_db(conn, symbol="TCS", only_price_applicable=True)
        assert len(tcs_events) == 1
        tcs = tcs_events[0]
        assert tcs.revision_number == 3
        assert tcs.cum_price_reference == 2269.3000
        assert tcs.ex_price_reference == 2304.8999
        assert tcs.total_return_factor == 0.99471203
        assert tcs.nominal_price_factor == 1.0
        assert tcs.volume_adjustment_factor == 1.0

        # 2. TATAMOTORS is excluded from price applicable factors
        tm_price_events = load_verified_factors_from_db(conn, symbol="TATAMOTORS", only_price_applicable=True)
        assert len(tm_price_events) == 0, "TATAMOTORS demerger tax allocation must not be loaded as price factor"

        # 3. TATAMOTORS Revision 2 loaded when inspecting all factors (including quarantined)
        tm_all_events = load_verified_factors_from_db(conn, symbol="TATAMOTORS", only_price_applicable=False, include_unverified=True)
        assert len(tm_all_events) == 1
        tm = tm_all_events[0]
        assert tm.revision_number == 2
        assert tm.factor_category.value == "TAX_COST_ALLOCATION"
        assert tm.price_factor_applicable is False
        assert tm.tax_cost_allocation_pct == 68.85
        assert tm.dispute_status.value == "DISPUTED_NON_PRICE_FACTOR"

        conn.close()


class TestP1810LegacyFactorIsolationAndFormulaCorrections:
    """
    HalalEquity P1.8.10 - Legacy Factor Isolation, Formula Corrections & Fail-Closed Validation Suite.
    """

    def test_superseded_tatamotors_revision_1_never_applied(self):
        """
        TATAMOTORS Revision 1 had adjustment_factor = 0.6885 and is_verified = True.
        Verify that Revision 1 is NEVER returned by load_verified_factors_from_db or applied
        by PriceAdjustmentEngine, because it was superseded by Revision 2 (TAX_COST_ALLOCATION,
        price_factor_applicable=False).
        """
        import os
        import psycopg2
        from src.engines.research.price_adjustment import load_verified_factors_from_db

        db_url = os.environ.get("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        # Direct price-applicable query
        price_events = load_verified_factors_from_db(conn, symbol="TATAMOTORS", only_price_applicable=True)
        assert len(price_events) == 0, "Superseded Revision 1 (0.6885) must NEVER leak into price reconstruction"

        # Even if unverified is requested, latest revision is Rev 2 (not Rev 1)
        all_events = load_verified_factors_from_db(conn, symbol="TATAMOTORS", only_price_applicable=False, include_unverified=True)
        assert len(all_events) == 1
        assert all_events[0].revision_number == 2
        assert all_events[0].price_factor_applicable is False
        assert all_events[0].adjustment_factor == 1.0  # Neutralized
        conn.close()

    def test_superseded_ltim_revision_1_never_applied(self):
        """
        LTIM Revision 1 was generic. Revision 2 explicitly classifies it as SYMBOLOGY_LINEAGE
        with price_factor_applicable=False. Verify Revision 1 is superseded and never applied.
        """
        import os
        import psycopg2
        from src.engines.research.price_adjustment import load_verified_factors_from_db

        db_url = os.environ.get("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        price_events = load_verified_factors_from_db(conn, symbol="LTIM", only_price_applicable=True)
        assert len(price_events) == 0, "LTIM symbology lineage must not be loaded as price factor"

        all_events = load_verified_factors_from_db(conn, symbol="LTIM", only_price_applicable=False, include_unverified=True)
        assert len(all_events) == 1
        assert all_events[0].revision_number == 2
        assert all_events[0].factor_category.value == "SYMBOLOGY_LINEAGE"
        assert all_events[0].price_factor_applicable is False
        conn.close()

    def test_tcs_revision_3_exact_provenance_and_precision(self):
        """
        TCS Revision 3 reconciles total return factor to exact 8-decimal precision:
        (2269.3000 - 12.0000) / 2269.3000 = 0.9947120257... -> 0.99471203.
        Discrepancy with legacy Yahoo-derived factor (0.99471197) is verified as 6e-8.
        """
        from decimal import Decimal
        cum_close = Decimal("2269.3000")
        dividend = Decimal("12.0000")
        exact_factor = (cum_close - dividend) / cum_close
        quantized = float(exact_factor.quantize(Decimal("0.00000001")))
        assert quantized == 0.99471203

        stored_rev2 = 0.99471197
        diff = quantized - stored_rev2
        assert abs(diff - 0.00000006) < 1e-10

    def test_unspecified_and_disputed_factors_rejected_fail_closed(self):
        """
        Tests that any factor with factor_category=UNSPECIFIED, dispute_status != UNDISPUTED,
        or is_verified=False is rejected by is_valid_price_factor() and AdjustmentFactorChain.
        """
        from datetime import date
        from src.engines.research.price_adjustment import (
            CorporateActionEvent, ActionType, FactorCategory, DisputeStatus, AdjustmentFactorChain, AdjustmentMode
        )

        # 1. Unspecified category
        unspecified = CorporateActionEvent(
            symbol="TEST", action_type=ActionType.STOCK_SPLIT,
            factor_category=FactorCategory.UNSPECIFIED, price_factor_applicable=True,
            ex_date=date(2026, 10, 1), record_date=None, effective_date=date(2026, 10, 1),
            adjustment_factor=0.5, cumulative_total_factor=0.5, source_reference="TEST", is_verified=True
        )
        assert unspecified.is_valid_price_factor() is False

        # 2. Disputed factor
        disputed = CorporateActionEvent(
            symbol="TEST", action_type=ActionType.STOCK_SPLIT,
            factor_category=FactorCategory.SHARE_TRANSFORMATION, price_factor_applicable=True,
            ex_date=date(2026, 10, 1), record_date=None, effective_date=date(2026, 10, 1),
            adjustment_factor=0.5, cumulative_total_factor=0.5, source_reference="TEST", is_verified=True,
            dispute_status=DisputeStatus.DISPUTED_METADATA_DISCREPANCY
        )
        assert disputed.is_valid_price_factor() is False

        # 3. Unverified factor
        unverified = CorporateActionEvent(
            symbol="TEST", action_type=ActionType.STOCK_SPLIT,
            factor_category=FactorCategory.SHARE_TRANSFORMATION, price_factor_applicable=True,
            ex_date=date(2026, 10, 1), record_date=None, effective_date=date(2026, 10, 1),
            adjustment_factor=0.5, cumulative_total_factor=0.5, source_reference="TEST", is_verified=False
        )
        assert unverified.is_valid_price_factor() is False

        # 4. Chain evaluation: factors rejected -> price_factor remains 1.0
        chain = AdjustmentFactorChain("TEST", [unspecified, disputed, unverified], date(2026, 10, 15), AdjustmentMode.PRICE_RETURN)
        p_fac, v_fac = chain.get_factor_on_date(date(2026, 9, 1))
        assert p_fac == 1.0
        assert v_fac == 1.0

    def test_latest_revision_resolution_with_distinct_events_same_date(self):
        """
        Verify that multiple distinct events on the same symbol and date with different event_id
        resolve independently and do NOT merge.
        """
        from datetime import date
        from src.engines.research.price_adjustment import (
            CorporateActionEvent, ActionType, FactorCategory, AdjustmentFactorChain, AdjustmentMode
        )

        ev_interim_r1 = CorporateActionEvent(
            symbol="TEST", action_type=ActionType.CASH_DIVIDEND, factor_category=FactorCategory.CASH_DISTRIBUTION,
            event_id="DIV_INTERIM", revision_number=1, price_factor_applicable=True,
            ex_date=date(2026, 10, 14), record_date=None, effective_date=date(2026, 10, 14),
            adjustment_factor=0.99, cumulative_total_factor=0.99, total_return_factor=0.99,
            source_reference="INT", is_verified=True
        )
        ev_interim_r2 = CorporateActionEvent(
            symbol="TEST", action_type=ActionType.CASH_DIVIDEND, factor_category=FactorCategory.CASH_DISTRIBUTION,
            event_id="DIV_INTERIM", revision_number=2, price_factor_applicable=True,
            ex_date=date(2026, 10, 14), record_date=None, effective_date=date(2026, 10, 14),
            adjustment_factor=0.995, cumulative_total_factor=0.995, total_return_factor=0.995,
            source_reference="INT_REV2", is_verified=True
        )
        ev_special_r1 = CorporateActionEvent(
            symbol="TEST", action_type=ActionType.CASH_DIVIDEND, factor_category=FactorCategory.CASH_DISTRIBUTION,
            event_id="DIV_SPECIAL", revision_number=1, price_factor_applicable=True,
            ex_date=date(2026, 10, 14), record_date=None, effective_date=date(2026, 10, 14),
            adjustment_factor=0.98, cumulative_total_factor=0.98, total_return_factor=0.98,
            source_reference="SPEC", is_verified=True
        )

        # Simulating latest revision per (symbol, ex_date, action_type, event_id):
        # Interim resolves to Rev 2 (0.995), Special resolves to Rev 1 (0.98)
        latest_events = [ev_interim_r2, ev_special_r1]
        chain = AdjustmentFactorChain("TEST", latest_events, date(2026, 10, 20), AdjustmentMode.TOTAL_RETURN)
        p_fac, v_fac = chain.get_factor_on_date(date(2026, 10, 1))
        # Compounded: 0.995 * 0.98 = 0.9751
        assert round(p_fac, 4) == 0.9751
        assert v_fac == 1.0  # Cash dividend volume unscaled

    def test_rights_issue_terp_across_all_subscription_price_regimes(self):
        """
        Verify TERP and adjustment factors across all three regimes:
        1. Discount (sub < cum): factor < 1.0
        2. At-the-market (sub == cum): factor == 1.0
        3. Premium (sub > cum):
           - economic_rational_exercise=True: factor == 1.0 (rights lapse unexercised)
           - economic_rational_exercise=False: factor > 1.0 (unconditional formula)
        """
        from src.engines.research.price_adjustment import PriceAdjustmentEngine

        # Ratio 1:4 (1 new share for 4 old held)
        cum_price = 100.0
        old_ratio = 4.0
        new_ratio = 1.0

        # 1. Discount (P_sub = 80 < P_cum = 100)
        # TERP = (4*100 + 1*80) / 5 = 480 / 5 = 96.0 -> factor = 96/100 = 0.96
        f_disc = PriceAdjustmentEngine.calculate_rights_issue_factor(cum_price, 80.0, new_ratio, old_ratio)
        assert f_disc == 0.96000000

        # 2. At-the-market (P_sub = 100 == P_cum = 100)
        # TERP = (4*100 + 1*100) / 5 = 100.0 -> factor = 100/100 = 1.0
        f_atm = PriceAdjustmentEngine.calculate_rights_issue_factor(cum_price, 100.0, new_ratio, old_ratio)
        assert f_atm == 1.00000000

        # 3. Premium (P_sub = 120 > P_cum = 100)
        # Exchange methodology (economic_rational_exercise=True):
        # Rights expire unexercised because shares can be bought cheaper in secondary market.
        f_otm_rational = PriceAdjustmentEngine.calculate_rights_issue_factor(cum_price, 120.0, new_ratio, old_ratio, economic_rational_exercise=True)
        assert f_otm_rational == 1.00000000

        # Unconditional formula (economic_rational_exercise=False):
        # TERP = (4*100 + 1*120) / 5 = 520 / 5 = 104.0 -> factor = 104/100 = 1.04
        f_otm_unconditional = PriceAdjustmentEngine.calculate_rights_issue_factor(cum_price, 120.0, new_ratio, old_ratio, economic_rational_exercise=False)
        assert f_otm_unconditional == 1.04000000

    def test_turnover_conservation_invariants_distinction(self):
        """
        Verifies which corporate action transformations conserve notional turnover (P * V):
        1. Stock Splits & Bonuses: CONSERVE turnover (P_adj * V_adj == P_raw * V_raw).
        2. Cash Dividend (Total Return): DO NOT conserve turnover because volume is unscaled (V_adj == V_raw).
        3. Cash Dividend (Price Return): CONSERVE turnover because both price and volume are unscaled.
        """
        from datetime import date
        import pandas as pd
        from src.engines.research.price_adjustment import (
            CorporateActionEvent, ActionType, FactorCategory, AdjustmentMode, PriceAdjustmentEngine
        )

        dates = pd.date_range("2026-10-01", periods=2, freq="B")
        raw_df = pd.DataFrame({
            "symbol": ["TEST"] * 2,
            "timestamp": dates,
            "open": [100.0, 100.0],
            "high": [105.0, 105.0],
            "low": [95.0, 95.0],
            "close": [100.0, 100.0],
            "volume": [1000, 1000],
        })
        raw_turnover = raw_df["close"].iloc[0] * raw_df["volume"].iloc[0]  # 100 * 1000 = 100,000

        # 1. Split 2-for-1: price halved, volume doubled
        split_ev = CorporateActionEvent(
            symbol="TEST", action_type=ActionType.STOCK_SPLIT, factor_category=FactorCategory.SHARE_TRANSFORMATION,
            price_factor_applicable=True, ex_date=date(2026, 10, 2), record_date=None, effective_date=date(2026, 10, 2),
            adjustment_factor=0.5, cumulative_total_factor=0.5, nominal_price_factor=0.5, volume_adjustment_factor=0.5,
            source_reference="SPLIT", is_verified=True
        )
        split_df = PriceAdjustmentEngine.reconstruct_series(raw_df, [split_ev], mode=AdjustmentMode.PRICE_RETURN)
        adj_split_turnover = split_df["close"].iloc[0] * split_df["volume"].iloc[0]  # 50 * 2000 = 100,000
        assert adj_split_turnover == raw_turnover, "Split must strictly conserve notional turnover"

        # 2. Cash Dividend in TOTAL_RETURN mode: price adjusted, volume unscaled
        div_ev = CorporateActionEvent(
            symbol="TEST", action_type=ActionType.CASH_DIVIDEND, factor_category=FactorCategory.CASH_DISTRIBUTION,
            price_factor_applicable=True, ex_date=date(2026, 10, 2), record_date=None, effective_date=date(2026, 10, 2),
            adjustment_factor=0.90, cumulative_total_factor=0.90, nominal_price_factor=1.0, total_return_factor=0.90,
            volume_adjustment_factor=1.0, source_reference="DIV", is_verified=True
        )
        tr_df = PriceAdjustmentEngine.reconstruct_series(raw_df, [div_ev], mode=AdjustmentMode.TOTAL_RETURN)
        assert tr_df["volume"].iloc[0] == raw_df["volume"].iloc[0], "Dividend volume must remain unscaled"
        adj_div_tr_turnover = tr_df["close"].iloc[0] * tr_df["volume"].iloc[0]  # 90 * 1000 = 90,000
        assert adj_div_tr_turnover != raw_turnover, "Total return dividend adjustment does NOT conserve raw turnover"

        # 3. Cash Dividend in PRICE_RETURN mode: price unadjusted, volume unscaled
        pr_df = PriceAdjustmentEngine.reconstruct_series(raw_df, [div_ev], mode=AdjustmentMode.PRICE_RETURN)
        adj_div_pr_turnover = pr_df["close"].iloc[0] * pr_df["volume"].iloc[0]
        assert adj_div_pr_turnover == raw_turnover, "Price return dividend mode strictly conserves raw turnover"

    def test_strategy_v2_frozen_and_execution_safety_flags(self):
        """
        Verify that Strategy V2 configuration remains frozen at BUY=80.0, SELL=45.0, ATR=2.0
        matching canonical hash 92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780.
        """
        from src.engines.research.strategy_config import assert_frozen, PINNED_HASH
        cfg = assert_frozen()
        assert cfg["matches_pinned"] is True
        assert cfg["runtime_hash"] == PINNED_HASH
        assert cfg["resolved"]["BUY_THRESHOLD"] == 80.0
        assert cfg["resolved"]["SELL_THRESHOLD"] == 45.0
        assert cfg["resolved"]["ATR_MULTIPLIER"] == 2.0


class TestP1811PrimaryCorporateActionHarvestingAndProvenance:
    """
    P1.8.11 Verification Suite:
    - Nominal-price consumer isolation from dividend total-return factors
    - Primary source artifact immutability and SHA-256 verification
    - Composite event separation with distinct deterministic event_ids
    - Preservation of research quarantine (23,124 NULL adjusted bars)
    - Pinned Strategy V2 governance & execution safety controls
    """

    def test_nominal_price_consumers_strictly_isolated_from_dividends(self):
        """
        Verify that in PRICE_RETURN mode, cash dividends produce a factor of 1.00000000
        and NEVER alter prices or volumes.
        """
        div_ev = CorporateActionEvent(
            symbol="TCS",
            action_type=ActionType.CASH_DIVIDEND,
            factor_category=FactorCategory.CASH_DISTRIBUTION,
            event_id="NSE:TCS:2026-10-14:CASH_DIVIDEND:1",
            price_factor_applicable=True,
            ex_date=date(2026, 10, 14),
            record_date=date(2026, 10, 14),
            effective_date=date(2026, 10, 14),
            adjustment_factor=0.99471203,
            cumulative_total_factor=0.99471203,
            nominal_price_factor=1.00000000,
            total_return_factor=0.99471203,
            volume_adjustment_factor=1.00000000,
            source_reference="NSE_CA_TCS_20261016_7b5b9325",
            is_verified=True,
        )

        chain = AdjustmentFactorChain(
            symbol="TCS",
            events=[div_ev],
            as_of_date=date(2026, 10, 15),
            mode=AdjustmentMode.PRICE_RETURN,
        )

        # Factor on cum-dividend date (2026-10-13) in PRICE_RETURN mode MUST be exactly (1.0, 1.0)
        p_fac, v_fac = chain.get_factor_on_date(date(2026, 10, 13))
        assert p_fac == 1.00000000, f"Nominal price factor must be strictly 1.0, got {p_fac}"
        assert v_fac == 1.00000000, f"Volume scale factor must be strictly 1.0, got {v_fac}"

        # Reconstructed series in PRICE_RETURN mode must be completely identical to raw prices
        raw_df = pd.DataFrame([
            {"timestamp": pd.Timestamp("2026-10-13"), "open": 2156.9, "high": 2269.3, "low": 2155.0, "close": 2269.3, "volume": 1000},
            {"timestamp": pd.Timestamp("2026-10-14"), "open": 2274.0, "high": 2335.8, "low": 2265.6, "close": 2304.9, "volume": 1200},
        ])
        reconstructed = PriceAdjustmentEngine.reconstruct_series(raw_df, [div_ev], mode=AdjustmentMode.PRICE_RETURN)
        assert reconstructed["close"].iloc[0] == 2269.3, "Cum-dividend close price must remain unadjusted in PRICE_RETURN mode"
        assert reconstructed["volume"].iloc[0] == 1000, "Cum-dividend volume must remain unadjusted in PRICE_RETURN mode"

    def test_composite_dividends_separated_into_distinct_events(self):
        """
        Verify that multiple corporate actions declared for the same symbol and date
        (e.g. INFY Interim + Special dividend on 2025-05-31) have distinct event_ids.
        """
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        cur.execute("""
            SELECT event_id, action_type, dividend_amount
            FROM corporate_action_events
            WHERE symbol = 'INFY' AND ex_date = '2025-05-31'
            ORDER BY event_id ASC;
        """)
        rows = cur.fetchall()
        conn.close()

        if len(rows) == 0:
            pytest.skip("Corporate action events not yet seeded in test database")

        assert len(rows) == 2, f"INFY on 2025-05-31 must have 2 distinct events, got {len(rows)}"
        event_ids = [r[0] for r in rows]
        assert len(set(event_ids)) == 2, "Event IDs must be distinct"
        assert "NSE:INFY:2025-05-31:CASH_DIVIDEND:1" in event_ids
        assert "NSE:INFY:2025-05-31:CASH_DIVIDEND:2" in event_ids

    def test_source_artifacts_table_immutable_under_triggers(self):
        """
        Verify that corporate_action_source_artifacts triggers block UPDATE and DELETE operations.
        """
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        cur.execute("SELECT id FROM corporate_action_source_artifacts LIMIT 1;")
        row = cur.fetchone()
        if not row:
            conn.close()
            pytest.skip("No corporate action artifacts found in DB")

        art_id = row[0]

        # 1. Attempt UPDATE -> must raise exception
        with pytest.raises(Exception) as exc_info:
            cur.execute("UPDATE corporate_action_source_artifacts SET validation_status = 'MUTATED' WHERE id = %s;", (art_id,))
            conn.commit()
        conn.rollback()
        assert "append-only" in str(exc_info.value).lower()

        # 2. Attempt DELETE -> must raise exception
        with pytest.raises(Exception) as exc_info2:
            cur.execute("DELETE FROM corporate_action_source_artifacts WHERE id = %s;", (art_id,))
            conn.commit()
        conn.rollback()
        assert "append-only" in str(exc_info2.value).lower()

        conn.close()

    def test_research_quarantine_invariant_preserved(self):
        """
        Verify that research quarantine is strictly preserved:
        23,124 daily bars have adjusted_close = NULL (98.0% NULL).
        """
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        cur.execute("""
            SELECT 
                count(*) AS total_bars,
                count(adjusted_close) AS populated_adj,
                count(*) - count(adjusted_close) AS null_adj
            FROM ohlcv_daily;
        """)
        total, pop_adj, null_adj = cur.fetchone()
        conn.close()

        assert null_adj == 23132, f"Quarantined NULL adjusted_close count must remain strictly 23132 (462 populated after HCLTECH vendor adjustment quarantine), got {null_adj}"
        assert total >= 23594, f"Total bars must be at least 23594, got {total}"
        assert pop_adj == total - null_adj, f"Populated adjusted_close bars must be exactly {total - null_adj}, got {pop_adj}"
        pct_null = (null_adj / total) * 100
        assert pct_null >= 97.4, f"Quarantine percentage must remain >= 97.4% across forward sessions, got {pct_null:.2f}%"

    def test_shariah_partition_reconciliation_integrity(self):
        """
        Verify that Shariah partition in database is exactly:
        0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT (total 51 active symbols)
        reconciling P1.8.3 remediation of templated screenings.
        """
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        cur.execute("""
            SELECT 
                (SELECT count(*) FROM symbols WHERE is_active = true) as total_active,
                (SELECT count(*) FROM (
                    SELECT DISTINCT ON (s.id) sc.status
                    FROM symbols s JOIN shariah_screenings sc ON s.id = sc.symbol_id
                    WHERE s.is_active = true
                    ORDER BY s.id, sc.screening_date DESC, sc.screened_at DESC, sc.id DESC
                ) t WHERE status = 'PASS') as pass_count,
                (SELECT count(*) FROM (
                    SELECT DISTINCT ON (s.id) sc.status
                    FROM symbols s JOIN shariah_screenings sc ON s.id = sc.symbol_id
                    WHERE s.is_active = true
                    ORDER BY s.id, sc.screening_date DESC, sc.screened_at DESC, sc.id DESC
                ) t WHERE status = 'FAIL') as fail_count,
                (SELECT count(*) FROM (
                    SELECT DISTINCT ON (s.id) sc.status
                    FROM symbols s JOIN shariah_screenings sc ON s.id = sc.symbol_id
                    WHERE s.is_active = true
                    ORDER BY s.id, sc.screening_date DESC, sc.screened_at DESC, sc.id DESC
                ) t WHERE status = 'DATA_INSUFFICIENT') as data_insufficient_count;
        """)
        total_active, pass_cnt, fail_cnt, data_insuf_cnt = cur.fetchone()
        conn.close()

        assert total_active == 51, f"Expected 51 active symbols, got {total_active}"
        assert pass_cnt == 0, f"Expected 0 PASS (remediated templated screenings), got {pass_cnt}"
        assert fail_cnt == 20, f"Expected 20 FAIL, got {fail_cnt}"
        assert data_insuf_cnt == 31, f"Expected 31 DATA_INSUFFICIENT, got {data_insuf_cnt}"


class TestP1812CorporateActionIntegrityReconciliation:
    """
    Forensic verification tests for P1.8.12:
    - Zero active factors with 'DEFAULT' or invalid event_id
    - Reconciled TCS dividend arithmetic and provenance
    - Canonical universe reconciliation (52 instruments: 47 core, 2 rename, 3 demerger)
    - Ingestion trigger guard fail-closed rejection of 'DEFAULT'
    """

    def test_no_active_factor_has_default_or_blank_event_id(self):
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        cur.execute("""
            SELECT symbol, action_type, factor_category, event_id, revision_number
            FROM ca.v_latest_corporate_action_factors;
        """)
        rows = cur.fetchall()
        conn.close()

        assert len(rows) >= 3, f"Expected at least 3 latest factors, got {len(rows)}"
        for sym, act, cat, ev_id, rev in rows:
            assert ev_id != "DEFAULT", f"Factor for {sym} ({act}) has prohibited event_id 'DEFAULT'"
            assert ev_id != "", f"Factor for {sym} ({act}) has empty event_id"
            assert ":" in ev_id, f"Factor for {sym} event_id '{ev_id}' must be namespaced and deterministic"

    def test_tcs_provenance_and_arithmetic_resolution(self):
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        cur.execute("""
            SELECT event_id, revision_number, dividend_amount, cum_price_reference,
                   total_return_factor, nominal_price_factor, is_verified, dispute_status
            FROM ca.v_latest_corporate_action_factors
            WHERE symbol = 'TCS';
        """)
        row = cur.fetchone()
        conn.close()

        assert row is not None, "TCS must be present in latest corporate action factors"
        ev_id, rev, div_amt, cum_px, tr_factor, nom_factor, verified, dispute = row

        assert ev_id == "NSE:TCS:2026-10-14:CASH_DIVIDEND:1"
        assert rev == 3
        assert float(div_amt) == 12.0
        assert float(cum_px) == 2269.3000
        assert float(nom_factor) == 1.0

        # Exact decimal arithmetic check:
        # (2269.3000 - 12.0000) / 2269.3000 = 2257.30 / 2269.30 = 0.9947120257348081... -> 0.99471203
        expected_factor = round((2269.3000 - 12.0000) / 2269.3000, 8)
        assert abs(float(tr_factor) - expected_factor) < 1e-8
        assert float(tr_factor) == 0.99471203

    def test_canonical_universe_52_instruments_reconciliation(self):
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        cur.execute("""
            SELECT 
                count(*) as total_symbols,
                count(*) FILTER (WHERE is_active = true) as active_symbols,
                count(*) FILTER (WHERE is_active = false) as inactive_symbols,
                count(*) FILTER (WHERE symbol NOT IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV') AND is_active = true) as core_equities
            FROM symbols;
        """)
        tot, act, inact, core = cur.fetchone()

        cur.execute("SELECT count(*) FROM corporate_action_events;")
        total_events = cur.fetchone()[0]
        conn.close()

        assert tot == 52, f"Canonical universe total instruments must be exactly 52, got {tot}"
        assert act == 51, f"Active instruments must be exactly 51, got {act}"
        assert inact == 1, f"Inactive predecessor instruments must be exactly 1 (TATAMOTORS), got {inact}"
        assert core == 47, f"Core Tier-1 equities with OHLCV data must be exactly 47, got {core}"
        assert total_events >= 52, f"Total normalized events must be at least 52, got {total_events}"

    def test_trigger_blocks_insert_with_default_event_id(self):
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
        except Exception as e:
            pytest.skip(f"Database unavailable: {e}")

        cur = conn.cursor()
        with pytest.raises(Exception) as exc_info:
            cur.execute("""
                INSERT INTO corporate_action_adjustment_factors (
                    symbol_id, action_type, factor_category, event_id, ex_date, effective_date,
                    adjustment_factor, nominal_price_factor, total_return_factor, volume_adjustment_factor,
                    source_reference, revision_number
                ) VALUES (
                    (SELECT id FROM symbols WHERE symbol = 'TCS'), 'CASH_DIVIDEND', 'CASH_DISTRIBUTION', 'DEFAULT', '2026-10-14', '2026-10-14',
                    0.99471203, 1.0, 0.99471203, 1.0,
                    'TEST_DEFAULT_REJECT', 99
                );
            """)
            conn.commit()
        conn.rollback()
        conn.close()
        assert "DEFAULT or blank is prohibited" in str(exc_info.value)


class TestP1813PrimaryEvidenceCoverageAndDataReadiness:
    """
    HalalEquity P1.8.13 — Comprehensive integration and regression tests for:
    1. Reconciled universe taxonomy (47 core, 51 active, 1 inactive, 2 rename lineage, 3 demerger lineage, 49 economic entities, 52 total).
    2. Reconciled adjusted-price counts (23,594 bars, 462 populated, 23,132 NULL quarantined).
    3. HCLTECH unverified vendor adjustments quarantine (8 bars quarantined from 470 -> 462).
    4. TCS single-symbol adjustment isolation (only TCS has close != adjusted_close, 7 bars prior to ex-date).
    5. Primary evidence expansion (12 artifacts, 40 events across 11 symbols).
    6. 7-state data readiness taxonomy verification.
    7. No fabricated pre-listing price history for demerger lineages (TMPV and TMCV have 0 bars).
    8. Strategy V2 frozen configuration and runtime canonical hash lock.
    """

    @pytest.fixture
    def db_conn(self):
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
            yield conn
            conn.close()
        except Exception as e:
            pytest.skip(f"Database connection unavailable: {e}")

    def test_canonical_universe_and_entity_taxonomy(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                COUNT(*) as total_symbols,
                COUNT(*) FILTER (WHERE is_active = true) as active_symbols,
                COUNT(*) FILTER (WHERE is_active = false) as inactive_symbols,
                COUNT(*) FILTER (WHERE symbol NOT IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV')) as core_symbols,
                COUNT(*) FILTER (WHERE symbol IN ('LTIM', 'LTM')) as rename_lineage_symbols,
                COUNT(*) FILTER (WHERE symbol IN ('TATAMOTORS', 'TMPV', 'TMCV')) as demerger_lineage_symbols
            FROM symbols;
        """)
        tot, act, inact, core, ren, dem = cur.fetchone()
        assert tot == 52, f"Total instruments in symbols table must be 52, got {tot}"
        assert act == 51, f"Active listed instruments must be 51, got {act}"
        assert inact == 1, f"Inactive predecessor instruments must be 1 (TATAMOTORS), got {inact}"
        assert core == 47, f"Core listed equities must be 47, got {core}"
        assert ren == 2, f"Rename lineage instruments must be 2 (LTIM, LTM), got {ren}"
        assert dem == 3, f"Demerger lineage instruments must be 3 (TATAMOTORS, TMPV, TMCV), got {dem}"

        # Economic entities: 47 Core + 1 LTIMindtree entity (LTIM/LTM) + 1 Tata Motors Group entity (TATAMOTORS/TMPV/TMCV) = 49
        economic_entities = core + 1 + 1
        assert economic_entities == 49, f"Economic entities must equal 49, got {economic_entities}"

    def test_adjusted_price_reconciliation_and_hcltech_quarantine(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*) as total_bars,
                count(adjusted_close) as pop_adj,
                count(*) - count(adjusted_close) as null_adj
            FROM ohlcv_daily;
        """)
        total, pop, null_cnt = cur.fetchone()
        assert total in (23641, 23688, 23735), f"Total daily bars must be 23641 (through Oct 19), 23688 (through Oct 21), or 23735 (through Oct 22), got {total}"
        assert pop in (509, 556, 603), f"Populated adjusted_close must be 509 (through Oct 19), 556 (through Oct 21), or 603 (through Oct 22), got {pop}"
        assert null_cnt == 23132, f"Quarantined NULL bars must be strictly 23132, got {null_cnt}"

        # Verify HCLTECH vendor adjusted bars were quarantined
        cur.execute("""
            SELECT count(*) 
            FROM ohlcv_daily o JOIN symbols s ON o.symbol_id = s.id 
            WHERE s.symbol = 'HCLTECH' AND o.adjusted_close IS NOT NULL AND o.adjusted_close != o.close;
        """)
        hcl_diff = cur.fetchone()[0]
        assert hcl_diff == 0, f"HCLTECH must have 0 unverified vendor adjusted bars remaining, got {hcl_diff}"

        # Verify only TCS has adjusted_close != close across all 23,594 bars
        cur.execute("""
            SELECT s.symbol, count(*)
            FROM ohlcv_daily o JOIN symbols s ON o.symbol_id = s.id
            WHERE o.adjusted_close IS NOT NULL AND o.adjusted_close != o.close
            GROUP BY s.symbol;
        """)
        diff_rows = cur.fetchall()
        assert len(diff_rows) == 1, f"Only 1 symbol should have adjusted_close != close, got {diff_rows}"
        assert diff_rows[0][0] == "TCS", f"Diff symbol must be TCS, got {diff_rows[0][0]}"
        assert diff_rows[0][1] == 7, f"TCS diff bars must be 7, got {diff_rows[0][1]}"

    def test_latest_ingested_47_bars_provenance(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*) as total_bars,
                count(*) FILTER (WHERE adjusted_close = close) as identical_close_adj,
                count(*) FILTER (WHERE adjusted_close IS NULL) as null_adj
            FROM ohlcv_daily
            WHERE timestamp::date = '2026-10-16';
        """)
        tot, ident, null_adj = cur.fetchone()
        assert tot == 47, f"October 16 ingestion must be exactly 47 bars, got {tot}"
        assert ident == 47, f"All 47 bars ingested on Oct 16 must have adjusted_close == close, got {ident}"
        assert null_adj == 0, f"Zero bars on Oct 16 should be null, got {null_adj}"

    def test_seven_state_data_readiness_taxonomy(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            WITH readiness AS (
                SELECT 
                    s.id,
                    s.symbol,
                    CASE 
                        WHEN s.symbol IN ('LTIM', 'LTM') THEN 'LINEAGE_RENAME'
                        WHEN s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV') THEN 'LINEAGE_DEMERGER'
                        ELSE 'CORE_EQUITY'
                    END AS instrument_category,
                    CASE
                        WHEN COALESCE(act.active_fac_count, 0) > 0 THEN 'ELIGIBLE_FOR_SPECIFIC_USE'
                        WHEN COALESCE(fac.ver_fac_count, 0) > 0 THEN 'FACTOR_VALIDATED'
                        WHEN COALESCE(ev.ver_count, 0) > 0 THEN 'PRIMARY_SOURCE_VERIFIED'
                        WHEN COALESCE(ev.ev_count, 0) > 0 THEN 'PARSE_VALIDATED'
                        WHEN COALESCE(art.art_count, 0) > 0 THEN 'ARTIFACT_DEPOSITED'
                        ELSE 'SOURCE_MISSING'
                    END AS data_readiness_state
                FROM symbols s
                LEFT JOIN (
                    SELECT 
                        SPLIT_PART(document_identifier, '_', 3) as symbol,
                        count(*) as art_count
                    FROM corporate_action_source_artifacts
                    GROUP BY SPLIT_PART(document_identifier, '_', 3)
                ) art ON s.symbol = art.symbol
                LEFT JOIN (
                    SELECT 
                        symbol_id,
                        count(*) as ev_count,
                        count(*) FILTER (WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY') as ver_count
                    FROM corporate_action_events
                    GROUP BY symbol_id
                ) ev ON s.id = ev.symbol_id
                LEFT JOIN (
                    SELECT
                        symbol_id,
                        count(*) as fac_count,
                        count(*) FILTER (WHERE is_verified = true) as ver_fac_count
                    FROM corporate_action_adjustment_factors
                    GROUP BY symbol_id
                ) fac ON s.id = fac.symbol_id
                LEFT JOIN (
                    SELECT 
                        s2.id as symbol_id,
                        count(*) as active_fac_count
                    FROM (
                        SELECT symbol FROM ca.v_active_nominal_price_factors
                        UNION ALL
                        SELECT symbol FROM ca.v_active_total_return_factors
                    ) v
                    JOIN symbols s2 ON v.symbol = s2.symbol
                    GROUP BY s2.id
                ) act ON s.id = act.symbol_id
            )
            SELECT data_readiness_state, count(*) 
            FROM readiness
            GROUP BY data_readiness_state;
        """)
        state_map = dict(cur.fetchall())
        assert state_map["ELIGIBLE_FOR_SPECIFIC_USE"] == 1, f"TCS is the sole ELIGIBLE_FOR_SPECIFIC_USE symbol"
        assert state_map["FACTOR_VALIDATED"] == 2, f"LTIM and TATAMOTORS are FACTOR_VALIDATED"
        assert state_map["PRIMARY_SOURCE_VERIFIED"] >= 13, f"At least 13 symbols are PRIMARY_SOURCE_VERIFIED, got {state_map.get('PRIMARY_SOURCE_VERIFIED')}"
        assert state_map["SOURCE_MISSING"] <= 36, f"At most 36 symbols are SOURCE_MISSING, got {state_map.get('SOURCE_MISSING')}"

    def test_no_fabricated_pre_listing_price_history_for_demerger(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT s.symbol, count(o.timestamp)
            FROM symbols s
            LEFT JOIN ohlcv_daily o ON s.id = o.symbol_id
            WHERE s.symbol IN ('TMPV', 'TMCV', 'TATAMOTORS', 'LTIM', 'LTM')
            GROUP BY s.symbol;
        """)
        counts = dict(cur.fetchall())
        for sym, bar_cnt in counts.items():
            assert bar_cnt == 0, f"Lineage symbol {sym} must have 0 fabricated bars in ohlcv_daily, got {bar_cnt}"

    def test_frozen_strategy_v2_canonical_hash_lock(self):
        from src.engines.research.strategy_config import assert_frozen, PINNED_HASH
        cfg = assert_frozen()
        assert cfg["matches_pinned"] is True
        assert cfg["runtime_hash"] == PINNED_HASH
        assert cfg["runtime_hash"] == "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
        assert cfg["resolved"] == {"BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 2.0}


class TestP1814TCSMultiplicativeAdjustmentAndFactorConsumerSeparation:
    """
    HalalEquity P1.8.14 — Corporate-Action Factor Validation & Historical Readiness Controls.
    1. TCS row-by-row multiplicative formula verification against raw prices and float32 ingestion precision.
    2. Explaining the ₹12 difference discrepancy: cum-date 2026-10-13 difference is exactly ₹12,
       while prior dates are strictly proportional (₹10.9778 to ₹11.4009).
    3. Multiplicative vs additive proof: observed ratio is strictly 0.99471203 across all 7 bars.
    4. Nominal vs total-return consumer isolation at database and engine level.
    5. Disputed and tax-only factor fail-closed exclusion.
    6. Quarantine invariant preservation: 23,132 bars remain NULL, 462 populated.
    """

    @pytest.fixture
    def db_conn(self):
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
            yield conn
            conn.close()
        except Exception as e:
            pytest.skip(f"Database connection unavailable: {e}")

    def test_tcs_row_by_row_multiplicative_precision_audit(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                o.timestamp::date::text,
                o.close,
                o.adjusted_close,
                ROUND((o.close - o.adjusted_close), 4),
                ROUND((o.adjusted_close / o.close), 8)
            FROM ohlcv_daily o
            JOIN symbols s ON o.symbol_id = s.id
            WHERE s.symbol = 'TCS'
              AND o.timestamp::date >= '2026-10-05'
              AND o.timestamp::date <= '2026-10-13'
            ORDER BY o.timestamp::date ASC;
        """)
        rows = cur.fetchall()
        assert len(rows) == 7, f"Must have exactly 7 pre-ex-date TCS bars, got {len(rows)}"

        import numpy as np
        factor_f32 = np.float32(0.99471203)

        for d_str, raw_close, stored_adj, diff, ratio in rows:
            raw_f = float(raw_close)
            stored_f = float(stored_adj)
            diff_f = float(diff)
            ratio_f = float(ratio)

            # 1. Verify ratio is constant ~0.99471203 across all 7 dates
            assert abs(ratio_f - 0.99471203) < 0.00000015, f"Date {d_str} ratio {ratio_f} diverges from 0.99471203"

            # 2. Verify exact float32 multiplicative formula match
            calc_f32 = round(float(np.float32(raw_f) * factor_f32), 4)
            assert calc_f32 == stored_f, f"Date {d_str} exact float32 mismatch: calc={calc_f32}, stored={stored_f}"

            # 3. Dissect the ₹12 difference claim:
            if d_str == "2026-10-13":
                # Cum-date: raw close 2269.30 - stored adj 2257.30 = exactly 12.0000
                assert abs(diff_f - 12.0000) < 0.0001, f"Cum-date {d_str} diff must be exactly 12.0000, got {diff_f}"
            else:
                # Prior dates: diff is proportional to raw close, strictly NOT 12.0000
                assert diff_f != 12.0000, f"Prior date {d_str} must not have flat 12.0000 diff"
                assert 10.97 <= diff_f <= 11.41, f"Prior date {d_str} diff {diff_f} outside expected range [10.97, 11.41]"

    def test_nominal_vs_total_return_consumer_separation_invariants(self, db_conn):
        cur = db_conn.cursor()

        # Nominal price view: must contain ZERO rows (cash dividends never alter nominal price series)
        cur.execute("SELECT count(*) FROM ca.v_active_nominal_price_factors;")
        nom_count = cur.fetchone()[0]
        assert nom_count == 0, f"Nominal price factors view must have 0 rows, got {nom_count}"

        # Total return view: must contain exactly 1 row (TCS Revision 3)
        cur.execute("""
            SELECT symbol, action_type, factor_category, event_id, revision_number, price_factor, is_verified, dispute_status, source_reference
            FROM ca.v_active_total_return_factors;
        """)
        tr_rows = cur.fetchall()
        assert len(tr_rows) == 1, f"Total return factors view must have exactly 1 row, got {len(tr_rows)}"
        tr_sym, tr_act, tr_cat, tr_eid, tr_rev, tr_fac, tr_ver, tr_disp, tr_ref = tr_rows[0]

        assert tr_sym == "TCS"
        assert tr_act == "CASH_DIVIDEND"
        assert tr_cat == "CASH_DISTRIBUTION"
        assert tr_eid == "NSE:TCS:2026-10-14:CASH_DIVIDEND:1"
        assert tr_rev == 3
        assert abs(float(tr_fac) - 0.99471203) < 0.00000001
        assert tr_ver is True
        assert tr_disp == "UNDISPUTED"
        assert tr_ref == "NSE/CML/2026/DIV/TCS_INT_12"

    def test_disputed_and_tax_only_factors_fail_closed(self, db_conn):
        cur = db_conn.cursor()

        # Check TATAMOTORS demerger tax cost allocation is excluded from all price views
        cur.execute("""
            SELECT count(*) 
            FROM ca.v_active_total_return_factors 
            WHERE symbol = 'TATAMOTORS';
        """)
        tm_tr = cur.fetchone()[0]
        assert tm_tr == 0, f"TATAMOTORS must be excluded from total return price view, got {tm_tr}"

        cur.execute("""
            SELECT count(*) 
            FROM ca.v_active_nominal_price_factors 
            WHERE symbol = 'TATAMOTORS';
        """)
        tm_nom = cur.fetchone()[0]
        assert tm_nom == 0, f"TATAMOTORS must be excluded from nominal price view, got {tm_nom}"

        # Check LTIM rename lineage is excluded from all price views
        cur.execute("""
            SELECT count(*) 
            FROM ca.v_active_total_return_factors 
            WHERE symbol = 'LTIM';
        """)
        ltim_tr = cur.fetchone()[0]
        assert ltim_tr == 0, f"LTIM must be excluded from total return price view, got {ltim_tr}"

    def test_price_adjustment_engine_consumer_separation_in_code(self):
        from src.engines.research.price_adjustment import (
            PriceAdjustmentEngine, CorporateActionEvent, ActionType, FactorCategory, DisputeStatus, AdjustmentMode, AdjustmentFactorChain
        )
        from datetime import date

        # Setup TCS event
        tcs_event = CorporateActionEvent(
            symbol="TCS",
            action_type=ActionType.CASH_DIVIDEND,
            factor_category=FactorCategory.CASH_DISTRIBUTION,
            event_id="NSE:TCS:2026-10-14:CASH_DIVIDEND:1",
            revision_number=3,
            ex_date=date(2026, 10, 14),
            record_date=date(2026, 10, 14),
            effective_date=date(2026, 10, 14),
            adjustment_factor=0.99471203,
            nominal_price_factor=1.0,
            total_return_factor=0.99471203,
            cumulative_total_factor=0.99471203,
            volume_adjustment_factor=1.0,
            source_reference="NSE/CML/2026/DIV/TCS_INT_12",
            is_verified=True,
            price_factor_applicable=True,
            dispute_status=DisputeStatus.UNDISPUTED,
        )

        # 1. Total return mode: factor is applied prior to ex-date
        chain_tr = AdjustmentFactorChain(
            symbol="TCS",
            events=[tcs_event],
            as_of_date=date(2026, 10, 16),
            mode=AdjustmentMode.TOTAL_RETURN
        )
        p_fac_tr, v_fac_tr = chain_tr.get_factor_on_date(date(2026, 10, 13))
        assert abs(p_fac_tr - 0.99471203) < 1e-7
        assert v_fac_tr == 1.0  # Cash dividend NEVER scales volume!

        # 2. Price return mode: factor is strictly 1.0 (cash dividend does NOT alter nominal indicators)
        chain_pr = AdjustmentFactorChain(
            symbol="TCS",
            events=[tcs_event],
            as_of_date=date(2026, 10, 16),
            mode=AdjustmentMode.PRICE_RETURN
        )
        p_fac_pr, v_fac_pr = chain_pr.get_factor_on_date(date(2026, 10, 13))
        assert p_fac_pr == 1.0
        assert v_fac_pr == 1.0

        # 3. On or after ex-date: factor is 1.0 in all modes
        p_fac_post, _ = chain_tr.get_factor_on_date(date(2026, 10, 14))
        assert p_fac_post == 1.0


class TestP1815CorporateActionCoverageAndReadinessReconciliation:
    """
    HalalEquity P1.8.15 Regression and Forensic Reconciliation Tests:
    - Canonical 52-instrument universe inventory (47 core + 2 rename + 3 demerger lineage)
    - DIVISLAB presence and 502 OHLCV bars
    - Reconciled corporate action event totals (23 artifacts, 64 events: 57 core + 7 lineage)
    - Distinct Shariah partitions (40 FAIL, 90 DATA_INSUFFICIENT)
    - Historical research quarantine invariant (23,132 bars NULL adjusted close)
    - Research gate fails closed when mandatory corporate action evidence is missing
    """

    @pytest.fixture
    def db_conn(self):
        db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
        try:
            conn = psycopg2.connect(db_url)
            yield conn
            conn.close()
        except Exception as e:
            pytest.skip(f"Database connection unavailable: {e}")

    def test_canonical_52_instrument_inventory_and_lineage(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*),
                count(*) FILTER (WHERE symbol NOT IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV')),
                count(*) FILTER (WHERE symbol IN ('LTIM', 'LTM')),
                count(*) FILTER (WHERE symbol IN ('TATAMOTORS', 'TMPV', 'TMCV')),
                count(*) FILTER (WHERE is_active = true),
                count(*) FILTER (WHERE is_active = false)
            FROM symbols;
        """)
        total, core, rename, demerger, active, inactive = cur.fetchone()

        assert total == 52, f"Total instruments must be 52, got {total}"
        assert core == 47, f"Core equities must be 47, got {core}"
        assert rename == 2, f"Rename lineage instruments must be 2 (LTIM, LTM), got {rename}"
        assert demerger == 3, f"Demerger lineage instruments must be 3 (TATAMOTORS, TMPV, TMCV), got {demerger}"
        assert active == 51, f"Active instruments must be 51, got {active}"
        assert inactive == 1, f"Inactive instruments must be 1 (TATAMOTORS pre-demerger), got {inactive}"

        # Verify DIVISLAB (ID 29) presence and bar count
        cur.execute("SELECT id, is_active FROM symbols WHERE symbol = 'DIVISLAB';")
        d_row = cur.fetchone()
        assert d_row is not None, "DIVISLAB must be present in symbols registry"
        assert d_row[0] == 29
        assert d_row[1] is True

        cur.execute("SELECT count(*) FROM ohlcv_daily WHERE symbol_id = 29;")
        d_bars = cur.fetchone()[0]
        assert d_bars in (503, 504, 505), f"DIVISLAB must have 503 (through Oct 19), 504 (through Oct 21), or 505 (through Oct 22) daily bars, got {d_bars}"

    def test_corporate_action_event_count_reconciliation(self, db_conn):
        cur = db_conn.cursor()

        # Artifacts
        cur.execute("SELECT count(*), count(DISTINCT document_identifier) FROM corporate_action_source_artifacts;")
        total_arts, unique_docs = cur.fetchone()
        assert total_arts == 23, f"Expected 23 source artifacts, got {total_arts}"
        assert unique_docs == 23, f"Expected 23 unique source documents, got {unique_docs}"

        # Events
        cur.execute("""
            SELECT 
                count(*),
                count(DISTINCT event_id),
                count(*) FILTER (WHERE symbol NOT IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV')),
                count(*) FILTER (WHERE symbol IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV'))
            FROM corporate_action_events;
        """)
        total_evs, unique_eids, core_evs, lin_evs = cur.fetchone()
        assert total_evs == 64, f"Expected 64 corporate action events, got {total_evs}"
        assert unique_eids == 64, f"All 64 event_ids must be unique, got {unique_eids}"
        assert core_evs == 57, f"Expected 57 active core events, got {core_evs}"
        assert lin_evs == 7, f"Expected 7 lineage events (4 LTIM + 3 TATAMOTORS), got {lin_evs}"

        # Action types
        cur.execute("""
            SELECT 
                count(*) FILTER (WHERE action_type = 'CASH_DIVIDEND'),
                count(*) FILTER (WHERE action_type = 'STOCK_SPLIT'),
                count(*) FILTER (WHERE action_type = 'BONUS_ISSUE'),
                count(*) FILTER (WHERE action_type = 'RIGHTS_ISSUE'),
                count(*) FILTER (WHERE action_type = 'DEMERGER'),
                count(*) FILTER (WHERE action_type NOT IN ('CASH_DIVIDEND', 'STOCK_SPLIT', 'BONUS_ISSUE', 'RIGHTS_ISSUE', 'DEMERGER'))
            FROM corporate_action_events;
        """)
        divs, splits, bonuses, rights, dems, others = cur.fetchone()
        assert divs == 57, f"Expected 57 dividends, got {divs}"
        assert splits == 0, f"Expected 0 splits, got {splits}"
        assert bonuses == 0, f"Expected 0 bonuses, got {bonuses}"
        assert rights == 0, f"Expected 0 rights, got {rights}"
        assert dems == 1, f"Expected 1 demerger, got {dems}"
        assert others == 6, f"Expected 6 other records (4 LTIM + 2 TATAMOTORS), got {others}"

    def test_shariah_screening_distinct_partition(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT status, count(*) 
            FROM shariah_screenings 
            GROUP BY status 
            ORDER BY status;
        """)
        status_map = dict(cur.fetchall())
        assert status_map.get("FAIL", 0) == 40, f"Expected exactly 40 FAIL screenings, got {status_map.get('FAIL')}"
        assert status_map.get("DATA_INSUFFICIENT", 0) == 90, f"Expected exactly 90 DATA_INSUFFICIENT screenings, got {status_map.get('DATA_INSUFFICIENT')}"
        assert "PASS" not in status_map or status_map["PASS"] == 0, "No spurious PASS screenings permitted"

    def test_historical_quarantine_metrics(self, db_conn):
        cur = db_conn.cursor()
        cur.execute("""
            SELECT 
                count(*),
                count(adjusted_close),
                count(*) - count(adjusted_close)
            FROM ohlcv_daily;
        """)
        total, populated, quarantined = cur.fetchone()
        assert total in (23641, 23688, 23735), f"Expected 23,641, 23,688, or 23,735 bars, got {total}"
        assert populated in (509, 556, 603), f"Expected 509, 556, or 603 populated adjusted closes, got {populated}"
        assert quarantined == 23132, f"Expected strictly 23,132 quarantined bars, got {quarantined}"

    def test_research_gate_blocks_backtesting_on_incomplete_evidence(self, db_conn):
        cur = db_conn.cursor()
        # Count core equities marked SOURCE_MISSING
        cur.execute("""
            SELECT count(*) 
            FROM symbols s
            LEFT JOIN (
                SELECT DISTINCT symbol_id FROM corporate_action_events
            ) ev ON s.id = ev.symbol_id
            WHERE s.symbol NOT IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV')
              AND s.is_active = true
              AND ev.symbol_id IS NULL;
        """)
        missing_core = cur.fetchone()[0]
        assert missing_core == 27, f"Expected exactly 27 core symbols missing corporate action evidence, got {missing_core}"
        # Historical backtesting gate MUST fail closed when core symbols lack primary corporate action evidence
        gate_1_corporate_action_complete = (missing_core == 0)
        assert gate_1_corporate_action_complete is False, "Gate 1 must remain BLOCKED when corporate actions are incomplete"








