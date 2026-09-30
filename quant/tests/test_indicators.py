"""
Unit tests for Technical Indicators Engine.

Uses known-value test fixtures to verify deterministic correctness.
Every indicator calculation must produce the exact expected result.
"""

import pytest
import numpy as np
from src.engines.technical.indicators import TechnicalIndicators


# ==================================================
# Test Fixtures — Known-value data
# ==================================================

@pytest.fixture
def simple_prices():
    """Simple ascending price series for basic tests."""
    return {
        "open_prices":  [10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24],
        "high_prices":  [11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25],
        "low_prices":   [9,  10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23],
        "close_prices": [10.5, 11.5, 12.5, 13.5, 14.5, 15.5, 16.5, 17.5, 18.5, 19.5, 20.5, 21.5, 22.5, 23.5, 24.5],
        "volumes": [1000, 1200, 1100, 1300, 1500, 1400, 1600, 1200, 1800, 2000, 1500, 1700, 1900, 2100, 1600],
    }


@pytest.fixture
def rsi_test_prices():
    """Price series with known RSI values for verification."""
    # 14-period RSI test data
    close = [
        44.34, 44.09, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84,
        46.08, 45.89, 46.03, 45.61, 46.28, 46.28, 46.00, 46.03,
        46.41, 46.22, 45.64, 46.21, 46.25, 45.71, 46.45, 45.78,
        45.35, 44.03, 44.18, 44.22, 44.57, 43.42, 42.66,
    ]
    n = len(close)
    return {
        "open_prices": close,
        "high_prices": [c + 0.5 for c in close],
        "low_prices": [c - 0.5 for c in close],
        "close_prices": close,
        "volumes": [1000] * n,
    }


@pytest.fixture
def ti(simple_prices):
    """TechnicalIndicators instance with simple prices."""
    return TechnicalIndicators(**simple_prices)


@pytest.fixture
def ti_rsi(rsi_test_prices):
    """TechnicalIndicators instance for RSI tests."""
    return TechnicalIndicators(**rsi_test_prices)


# ==================================================
# SMA Tests
# ==================================================

class TestSMA:
    def test_sma_basic(self, ti):
        """SMA(5) of [10.5, 11.5, 12.5, 13.5, 14.5] = 12.5"""
        result = ti.sma(5)
        assert result[4] == pytest.approx(12.5, abs=0.001)

    def test_sma_first_values_nan(self, ti):
        """First period-1 values should be NaN."""
        result = ti.sma(5)
        assert all(np.isnan(result[i]) for i in range(4))

    def test_sma_period_larger_than_data(self, ti):
        """SMA with period > data length should be all NaN."""
        result = ti.sma(100)
        assert all(np.isnan(v) for v in result)

    def test_sma_calculation(self, ti):
        """Verify SMA calculation at multiple points."""
        result = ti.sma(3)
        # SMA(3) at index 2 = (10.5 + 11.5 + 12.5) / 3 = 11.5
        assert result[2] == pytest.approx(11.5, abs=0.001)
        # SMA(3) at index 3 = (11.5 + 12.5 + 13.5) / 3 = 12.5
        assert result[3] == pytest.approx(12.5, abs=0.001)


# ==================================================
# EMA Tests
# ==================================================

class TestEMA:
    def test_ema_first_value_equals_sma(self, ti):
        """First EMA value should equal SMA of the same period."""
        ema = ti.ema(5)
        sma = ti.sma(5)
        assert ema[4] == pytest.approx(sma[4], abs=0.001)

    def test_ema_reacts_faster(self, ti):
        """EMA should react faster to price changes than SMA."""
        ema = ti.ema(5)
        sma = ti.sma(5)
        # In an uptrend, EMA should be closer to current price
        last_idx = len(ti.close) - 1
        assert abs(ti.close[last_idx] - ema[last_idx]) <= abs(ti.close[last_idx] - sma[last_idx])


# ==================================================
# RSI Tests
# ==================================================

class TestRSI:
    def test_rsi_range(self, ti_rsi):
        """RSI should always be between 0 and 100."""
        result = ti_rsi.rsi(14)
        valid = result[~np.isnan(result)]
        assert all(0 <= v <= 100 for v in valid)

    def test_rsi_known_value(self, ti_rsi):
        """Verify RSI against known Wilder's method values."""
        result = ti_rsi.rsi(14)
        # After 14 periods, RSI should be calculable
        assert not np.isnan(result[14])
        # RSI(14) for the Wilder test data at index 14 should be ~66.94
        assert result[14] == pytest.approx(66.94, abs=1.0)

    def test_rsi_period_check(self, ti_rsi):
        """First 'period' values should be NaN."""
        result = ti_rsi.rsi(14)
        for i in range(14):
            assert np.isnan(result[i])


# ==================================================
# ATR Tests
# ==================================================

class TestATR:
    def test_atr_positive(self, ti):
        """ATR should always be positive."""
        result = ti.atr(14)
        valid = result[~np.isnan(result)]
        assert all(v > 0 for v in valid)

    def test_atr_first_value(self, ti):
        """First ATR value should be the mean of first 'period' true ranges."""
        result = ti.atr(5)
        # TR for simple ascending data: High-Low = 2 for all candles
        # First ATR(5) = mean of first 5 TRs
        assert not np.isnan(result[4])
        assert result[4] == pytest.approx(2.0, abs=0.1)


# ==================================================
# MACD Tests
# ==================================================

class TestMACD:
    def test_macd_components(self, ti):
        """MACD should return three components of correct length."""
        macd_line, signal_line, histogram = ti.macd()
        assert len(macd_line) == len(ti.close)
        assert len(signal_line) == len(ti.close)
        assert len(histogram) == len(ti.close)

    def test_macd_histogram_equals_difference(self, ti):
        """Histogram = MACD Line - Signal Line."""
        macd_line, signal_line, histogram = ti.macd()
        for i in range(len(ti.close)):
            if not np.isnan(macd_line[i]) and not np.isnan(signal_line[i]):
                assert histogram[i] == pytest.approx(macd_line[i] - signal_line[i], abs=0.0001)


# ==================================================
# Bollinger Bands Tests
# ==================================================

class TestBollingerBands:
    def test_bb_upper_above_middle(self, ti):
        """Upper band should always be above middle band."""
        upper, middle, lower = ti.bollinger_bands(5)
        for i in range(len(ti.close)):
            if not np.isnan(upper[i]):
                assert upper[i] >= middle[i]

    def test_bb_lower_below_middle(self, ti):
        """Lower band should always be below middle band."""
        upper, middle, lower = ti.bollinger_bands(5)
        for i in range(len(ti.close)):
            if not np.isnan(lower[i]):
                assert lower[i] <= middle[i]

    def test_bb_middle_equals_sma(self, ti):
        """Middle band should equal SMA."""
        upper, middle, lower = ti.bollinger_bands(5)
        sma = ti.sma(5)
        for i in range(len(ti.close)):
            if not np.isnan(middle[i]):
                assert middle[i] == pytest.approx(sma[i], abs=0.0001)


# ==================================================
# OBV Tests
# ==================================================

class TestOBV:
    def test_obv_direction(self):
        """OBV should increase when price goes up with volume."""
        ti = TechnicalIndicators(
            open_prices=[10, 10, 10],
            high_prices=[12, 12, 12],
            low_prices=[9, 9, 9],
            close_prices=[10, 11, 12],
            volumes=[1000, 2000, 3000],
        )
        obv = ti.obv()
        # Price going up, so OBV should increase
        assert obv[1] == 1000 + 2000  # 3000
        assert obv[2] == 3000 + 3000  # 6000


# ==================================================
# Indicator Dispatch Tests
# ==================================================

class TestDispatch:
    def test_calculate_sma(self, ti):
        """calculate('sma_20') should work."""
        result = ti.calculate("sma_5")
        assert result["indicator"] == "sma"
        assert result["parameters"]["period"] == 5

    def test_calculate_rsi(self, ti):
        """calculate('rsi_14') should work."""
        result = ti.calculate("rsi_14")
        assert result["indicator"] == "rsi"

    def test_calculate_unknown(self, ti):
        """Unknown indicator should raise ValueError."""
        with pytest.raises(ValueError):
            ti.calculate("unknown_indicator")

    def test_calculate_macd(self, ti):
        """calculate('macd') should work without period."""
        result = ti.calculate("macd")
        assert result["indicator"] == "macd"
        assert "signal_line" in result
        assert "histogram" in result


# ==================================================
# NaN Handling Tests
# ==================================================

class TestNaNHandling:
    def test_no_silent_zero_fill(self, ti):
        """Missing values must be None/NaN, never silently filled with 0."""
        result = ti.calculate("sma_5")
        # First 4 values should be None
        assert result["values"][0] is None
        assert result["values"][1] is None
        assert result["values"][2] is None
        assert result["values"][3] is None
        # 5th value should be a number
        assert result["values"][4] is not None
