"""
Technical Indicators Engine

Deterministic calculation of all technical indicators.
Every calculation is documented with its formula and is unit-tested.

Supported indicators:
- SMA (20, 50, 100, 200)
- EMA (9, 20, 50, 200)
- RSI (14)
- MACD (12, 26, 9)
- ADX (14)
- Stochastic (14, 3, 3)
- ROC (12)
- ATR (14)
- Bollinger Bands (20, 2)
- Supertrend (10, 3)
- OBV
- VWAP
"""

import numpy as np
from typing import Optional


class TechnicalIndicators:
    """
    Calculates technical indicators from OHLCV data.

    All calculations are deterministic — same input always produces same output.
    Missing values are represented as None/NaN, NEVER replaced with zero.
    """

    def __init__(
        self,
        open_prices: list[float],
        high_prices: list[float],
        low_prices: list[float],
        close_prices: list[float],
        volumes: list[int],
    ):
        self.open = np.array(open_prices, dtype=np.float64)
        self.high = np.array(high_prices, dtype=np.float64)
        self.low = np.array(low_prices, dtype=np.float64)
        self.close = np.array(close_prices, dtype=np.float64)
        self.volume = np.array(volumes, dtype=np.float64)
        self.n = len(close_prices)

    def calculate(self, indicator_spec: str) -> dict:
        """
        Parse indicator spec (e.g., 'sma_20', 'rsi_14', 'macd') and calculate.

        Returns dict with: indicator, parameters, values, signal
        """
        parts = indicator_spec.lower().split("_")
        name = parts[0]

        dispatch = {
            "sma": self._calc_sma,
            "ema": self._calc_ema,
            "rsi": self._calc_rsi,
            "macd": self._calc_macd,
            "adx": self._calc_adx,
            "stochastic": self._calc_stochastic,
            "roc": self._calc_roc,
            "atr": self._calc_atr,
            "bollinger": self._calc_bollinger,
            "bb": self._calc_bollinger,
            "supertrend": self._calc_supertrend,
            "obv": self._calc_obv,
            "vwap": self._calc_vwap,
        }

        if name not in dispatch:
            raise ValueError(f"Unknown indicator: {name}")

        period = int(parts[1]) if len(parts) > 1 else None
        return dispatch[name](period)

    # ==================================================
    # Simple Moving Average (SMA)
    # Formula: SMA = sum(close[i-period+1:i+1]) / period
    # ==================================================
    def sma(self, period: int) -> np.ndarray:
        """Calculate Simple Moving Average."""
        if period > self.n:
            return np.full(self.n, np.nan)

        result = np.full(self.n, np.nan)
        cumsum = np.cumsum(self.close)
        result[period - 1:] = (cumsum[period - 1:] - np.concatenate(([0], cumsum[:-period]))) / period
        return result

    def _calc_sma(self, period: Optional[int] = None) -> dict:
        period = period or 20
        values = self.sma(period)
        signal = self._trend_signal(values)
        return {
            "indicator": "sma",
            "parameters": {"period": period},
            "values": self._to_list(values),
            "signal": signal,
        }

    # ==================================================
    # Exponential Moving Average (EMA)
    # Formula: EMA_t = close_t * k + EMA_(t-1) * (1-k)
    # where k = 2 / (period + 1)
    # ==================================================
    def ema(self, period: int) -> np.ndarray:
        """Calculate Exponential Moving Average."""
        if period > self.n:
            return np.full(self.n, np.nan)

        result = np.full(self.n, np.nan)
        k = 2.0 / (period + 1)

        # Initialize with SMA
        result[period - 1] = np.mean(self.close[:period])

        for i in range(period, self.n):
            result[i] = self.close[i] * k + result[i - 1] * (1 - k)

        return result

    def _calc_ema(self, period: Optional[int] = None) -> dict:
        period = period or 20
        values = self.ema(period)
        signal = self._trend_signal(values)
        return {
            "indicator": "ema",
            "parameters": {"period": period},
            "values": self._to_list(values),
            "signal": signal,
        }

    # ==================================================
    # Relative Strength Index (RSI)
    # Formula:
    #   RS = avg_gain / avg_loss
    #   RSI = 100 - (100 / (1 + RS))
    # Uses Wilder's smoothing method
    # ==================================================
    def rsi(self, period: int = 14) -> np.ndarray:
        """Calculate Relative Strength Index using Wilder's smoothing."""
        if period >= self.n:
            return np.full(self.n, np.nan)

        deltas = np.diff(self.close)
        gains = np.where(deltas > 0, deltas, 0.0)
        losses = np.where(deltas < 0, -deltas, 0.0)

        result = np.full(self.n, np.nan)

        # First average
        avg_gain = np.mean(gains[:period])
        avg_loss = np.mean(losses[:period])

        if avg_loss == 0:
            result[period] = 100.0
        else:
            rs = avg_gain / avg_loss
            result[period] = 100.0 - (100.0 / (1.0 + rs))

        # Wilder's smoothing
        for i in range(period, len(deltas)):
            avg_gain = (avg_gain * (period - 1) + gains[i]) / period
            avg_loss = (avg_loss * (period - 1) + losses[i]) / period

            if avg_loss == 0:
                result[i + 1] = 100.0
            else:
                rs = avg_gain / avg_loss
                result[i + 1] = 100.0 - (100.0 / (1.0 + rs))

        return result

    def _calc_rsi(self, period: Optional[int] = None) -> dict:
        period = period or 14
        values = self.rsi(period)
        last_val = self._last_valid(values)
        signal = "NEUTRAL"
        if last_val is not None:
            if last_val > 70:
                signal = "BEARISH"  # Overbought
            elif last_val < 30:
                signal = "BULLISH"  # Oversold
        return {
            "indicator": "rsi",
            "parameters": {"period": period},
            "values": self._to_list(values),
            "signal": signal,
        }

    # ==================================================
    # MACD (Moving Average Convergence Divergence)
    # Formula:
    #   MACD Line = EMA(fast) - EMA(slow)
    #   Signal Line = EMA(MACD Line, signal_period)
    #   Histogram = MACD Line - Signal Line
    # ==================================================
    def macd(
        self, fast: int = 12, slow: int = 26, signal_period: int = 9
    ) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
        """Calculate MACD line, signal line, and histogram."""
        ema_fast = self.ema(fast)
        ema_slow = self.ema(slow)

        macd_line = ema_fast - ema_slow

        # Signal line: EMA of MACD line
        signal_line = np.full(self.n, np.nan)
        valid_mask = ~np.isnan(macd_line)
        valid_indices = np.where(valid_mask)[0]

        if len(valid_indices) >= signal_period:
            k = 2.0 / (signal_period + 1)
            start = valid_indices[signal_period - 1]
            signal_line[start] = np.mean(
                macd_line[valid_indices[:signal_period]]
            )
            for i in range(start + 1, self.n):
                if not np.isnan(macd_line[i]):
                    signal_line[i] = macd_line[i] * k + signal_line[i - 1] * (1 - k)

        histogram = macd_line - signal_line

        return macd_line, signal_line, histogram

    def _calc_macd(self, _period: Optional[int] = None) -> dict:
        macd_line, signal_line, histogram = self.macd()
        last_hist = self._last_valid(histogram)
        signal = "NEUTRAL"
        if last_hist is not None:
            signal = "BULLISH" if last_hist > 0 else "BEARISH"
        return {
            "indicator": "macd",
            "parameters": {"fast": 12, "slow": 26, "signal": 9},
            "values": self._to_list(macd_line),
            "signal_line": self._to_list(signal_line),
            "histogram": self._to_list(histogram),
            "signal": signal,
        }

    # ==================================================
    # Average Directional Index (ADX)
    # Measures trend strength (not direction)
    # ==================================================
    def adx(self, period: int = 14) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
        """Calculate ADX, +DI, and -DI."""
        result_adx = np.full(self.n, np.nan)
        plus_di = np.full(self.n, np.nan)
        minus_di = np.full(self.n, np.nan)

        if self.n < period + 1:
            return result_adx, plus_di, minus_di

        # True Range
        tr = np.zeros(self.n)
        plus_dm = np.zeros(self.n)
        minus_dm = np.zeros(self.n)

        for i in range(1, self.n):
            h_l = self.high[i] - self.low[i]
            h_pc = abs(self.high[i] - self.close[i - 1])
            l_pc = abs(self.low[i] - self.close[i - 1])
            tr[i] = max(h_l, h_pc, l_pc)

            up_move = self.high[i] - self.high[i - 1]
            down_move = self.low[i - 1] - self.low[i]

            plus_dm[i] = up_move if (up_move > down_move and up_move > 0) else 0
            minus_dm[i] = down_move if (down_move > up_move and down_move > 0) else 0

        # Wilder's smoothing
        atr_smooth = np.sum(tr[1 : period + 1])
        plus_dm_smooth = np.sum(plus_dm[1 : period + 1])
        minus_dm_smooth = np.sum(minus_dm[1 : period + 1])

        dx_values = []

        for i in range(period, self.n):
            if i > period:
                atr_smooth = atr_smooth - atr_smooth / period + tr[i]
                plus_dm_smooth = plus_dm_smooth - plus_dm_smooth / period + plus_dm[i]
                minus_dm_smooth = minus_dm_smooth - minus_dm_smooth / period + minus_dm[i]

            if atr_smooth > 0:
                plus_di[i] = 100 * plus_dm_smooth / atr_smooth
                minus_di[i] = 100 * minus_dm_smooth / atr_smooth
            else:
                plus_di[i] = 0
                minus_di[i] = 0

            di_sum = plus_di[i] + minus_di[i]
            if di_sum > 0:
                dx = 100 * abs(plus_di[i] - minus_di[i]) / di_sum
            else:
                dx = 0
            dx_values.append(dx)

            if len(dx_values) == period:
                result_adx[i] = np.mean(dx_values)
            elif len(dx_values) > period:
                result_adx[i] = (result_adx[i - 1] * (period - 1) + dx) / period

        return result_adx, plus_di, minus_di

    def _calc_adx(self, period: Optional[int] = None) -> dict:
        period = period or 14
        adx_val, plus_di, minus_di = self.adx(period)
        last_adx = self._last_valid(adx_val)
        signal = "NEUTRAL"
        if last_adx is not None:
            if last_adx > 25:
                signal = "TRENDING"
            else:
                signal = "RANGING"
        return {
            "indicator": "adx",
            "parameters": {"period": period},
            "values": self._to_list(adx_val),
            "plus_di": self._to_list(plus_di),
            "minus_di": self._to_list(minus_di),
            "signal": signal,
        }

    # ==================================================
    # Stochastic Oscillator
    # Formula:
    #   %K = (Close - Low_n) / (High_n - Low_n) * 100
    #   %D = SMA(%K, d_period)
    # ==================================================
    def stochastic(
        self, k_period: int = 14, d_period: int = 3, smooth: int = 3
    ) -> tuple[np.ndarray, np.ndarray]:
        """Calculate Stochastic %K and %D."""
        k = np.full(self.n, np.nan)
        d = np.full(self.n, np.nan)

        for i in range(k_period - 1, self.n):
            high_n = np.max(self.high[i - k_period + 1 : i + 1])
            low_n = np.min(self.low[i - k_period + 1 : i + 1])
            if high_n - low_n > 0:
                k[i] = (self.close[i] - low_n) / (high_n - low_n) * 100
            else:
                k[i] = 50.0

        # Smooth %K
        if smooth > 1:
            k_smooth = np.full(self.n, np.nan)
            for i in range(k_period - 1 + smooth - 1, self.n):
                vals = k[i - smooth + 1 : i + 1]
                if not np.any(np.isnan(vals)):
                    k_smooth[i] = np.mean(vals)
            k = k_smooth

        # %D = SMA of %K
        for i in range(self.n):
            if i >= d_period - 1:
                vals = k[i - d_period + 1 : i + 1]
                if not np.any(np.isnan(vals)):
                    d[i] = np.mean(vals)

        return k, d

    def _calc_stochastic(self, period: Optional[int] = None) -> dict:
        period = period or 14
        k, d = self.stochastic(period)
        last_k = self._last_valid(k)
        signal = "NEUTRAL"
        if last_k is not None:
            if last_k > 80:
                signal = "BEARISH"
            elif last_k < 20:
                signal = "BULLISH"
        return {
            "indicator": "stochastic",
            "parameters": {"k_period": period, "d_period": 3, "smooth": 3},
            "values": self._to_list(k),
            "d_values": self._to_list(d),
            "signal": signal,
        }

    # ==================================================
    # Rate of Change (ROC)
    # Formula: ROC = ((Close - Close_n) / Close_n) * 100
    # ==================================================
    def roc(self, period: int = 12) -> np.ndarray:
        """Calculate Rate of Change."""
        result = np.full(self.n, np.nan)
        for i in range(period, self.n):
            if self.close[i - period] != 0:
                result[i] = ((self.close[i] - self.close[i - period]) / self.close[i - period]) * 100
        return result

    def _calc_roc(self, period: Optional[int] = None) -> dict:
        period = period or 12
        values = self.roc(period)
        last_val = self._last_valid(values)
        signal = "NEUTRAL"
        if last_val is not None:
            signal = "BULLISH" if last_val > 0 else "BEARISH"
        return {
            "indicator": "roc",
            "parameters": {"period": period},
            "values": self._to_list(values),
            "signal": signal,
        }

    # ==================================================
    # Average True Range (ATR)
    # Formula:
    #   TR = max(High-Low, |High-PrevClose|, |Low-PrevClose|)
    #   ATR = Wilder's smoothed average of TR
    # ==================================================
    def atr(self, period: int = 14) -> np.ndarray:
        """Calculate Average True Range."""
        if self.n < 2:
            return np.full(self.n, np.nan)

        tr = np.zeros(self.n)
        tr[0] = self.high[0] - self.low[0]

        for i in range(1, self.n):
            h_l = self.high[i] - self.low[i]
            h_pc = abs(self.high[i] - self.close[i - 1])
            l_pc = abs(self.low[i] - self.close[i - 1])
            tr[i] = max(h_l, h_pc, l_pc)

        result = np.full(self.n, np.nan)
        if self.n >= period:
            result[period - 1] = np.mean(tr[:period])
            for i in range(period, self.n):
                result[i] = (result[i - 1] * (period - 1) + tr[i]) / period

        return result

    def _calc_atr(self, period: Optional[int] = None) -> dict:
        period = period or 14
        values = self.atr(period)
        return {
            "indicator": "atr",
            "parameters": {"period": period},
            "values": self._to_list(values),
            "signal": "NEUTRAL",
        }

    # ==================================================
    # Bollinger Bands
    # Formula:
    #   Middle = SMA(period)
    #   Upper = Middle + (std_dev * num_std)
    #   Lower = Middle - (std_dev * num_std)
    # ==================================================
    def bollinger_bands(
        self, period: int = 20, num_std: float = 2.0
    ) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
        """Calculate Bollinger Bands (upper, middle, lower)."""
        middle = self.sma(period)
        upper = np.full(self.n, np.nan)
        lower = np.full(self.n, np.nan)

        for i in range(period - 1, self.n):
            std = np.std(self.close[i - period + 1 : i + 1], ddof=0)
            upper[i] = middle[i] + num_std * std
            lower[i] = middle[i] - num_std * std

        return upper, middle, lower

    def _calc_bollinger(self, period: Optional[int] = None) -> dict:
        period = period or 20
        upper, middle, lower = self.bollinger_bands(period)
        last_close = self.close[-1] if self.n > 0 else None
        last_upper = self._last_valid(upper)
        last_lower = self._last_valid(lower)
        signal = "NEUTRAL"
        if last_close is not None and last_upper is not None and last_lower is not None:
            if last_close >= last_upper:
                signal = "BEARISH"  # At upper band
            elif last_close <= last_lower:
                signal = "BULLISH"  # At lower band
        return {
            "indicator": "bollinger_bands",
            "parameters": {"period": period, "std_dev": 2.0},
            "upper": self._to_list(upper),
            "values": self._to_list(middle),
            "lower": self._to_list(lower),
            "signal": signal,
        }

    # ==================================================
    # Supertrend
    # Uses ATR for dynamic support/resistance
    # ==================================================
    def supertrend(
        self, period: int = 10, multiplier: float = 3.0
    ) -> tuple[np.ndarray, np.ndarray]:
        """Calculate Supertrend indicator. Returns (supertrend_values, direction)."""
        atr_values = self.atr(period)
        hl2 = (self.high + self.low) / 2

        upper_band = np.full(self.n, np.nan)
        lower_band = np.full(self.n, np.nan)
        supertrend = np.full(self.n, np.nan)
        direction = np.zeros(self.n)  # 1 = uptrend, -1 = downtrend

        for i in range(period - 1, self.n):
            if np.isnan(atr_values[i]):
                continue

            basic_upper = hl2[i] + multiplier * atr_values[i]
            basic_lower = hl2[i] - multiplier * atr_values[i]

            if i == period - 1:
                upper_band[i] = basic_upper
                lower_band[i] = basic_lower
                direction[i] = 1 if self.close[i] > basic_upper else -1
            else:
                # Upper band
                if basic_upper < upper_band[i - 1] or self.close[i - 1] > upper_band[i - 1]:
                    upper_band[i] = basic_upper
                else:
                    upper_band[i] = upper_band[i - 1]

                # Lower band
                if basic_lower > lower_band[i - 1] or self.close[i - 1] < lower_band[i - 1]:
                    lower_band[i] = basic_lower
                else:
                    lower_band[i] = lower_band[i - 1]

                # Direction
                if direction[i - 1] == 1:
                    if self.close[i] < lower_band[i]:
                        direction[i] = -1
                    else:
                        direction[i] = 1
                else:
                    if self.close[i] > upper_band[i]:
                        direction[i] = 1
                    else:
                        direction[i] = -1

            supertrend[i] = lower_band[i] if direction[i] == 1 else upper_band[i]

        return supertrend, direction

    def _calc_supertrend(self, period: Optional[int] = None) -> dict:
        period = period or 10
        values, direction = self.supertrend(period)
        last_dir = direction[-1] if self.n > 0 else 0
        signal = "BULLISH" if last_dir == 1 else "BEARISH"
        return {
            "indicator": "supertrend",
            "parameters": {"period": period, "multiplier": 3.0},
            "values": self._to_list(values),
            "direction": direction.tolist(),
            "signal": signal,
        }

    # ==================================================
    # On-Balance Volume (OBV)
    # Formula: OBV += volume if close > prev_close, else OBV -= volume
    # ==================================================
    def obv(self) -> np.ndarray:
        """Calculate On-Balance Volume."""
        result = np.zeros(self.n)
        result[0] = self.volume[0]

        for i in range(1, self.n):
            if self.close[i] > self.close[i - 1]:
                result[i] = result[i - 1] + self.volume[i]
            elif self.close[i] < self.close[i - 1]:
                result[i] = result[i - 1] - self.volume[i]
            else:
                result[i] = result[i - 1]

        return result

    def _calc_obv(self, _period: Optional[int] = None) -> dict:
        values = self.obv()
        return {
            "indicator": "obv",
            "parameters": {},
            "values": self._to_list(values),
            "signal": "NEUTRAL",
        }

    # ==================================================
    # VWAP (Volume Weighted Average Price)
    # Formula: VWAP = cumsum(typical_price * volume) / cumsum(volume)
    # ==================================================
    def vwap(self) -> np.ndarray:
        """Calculate Volume Weighted Average Price (intraday use)."""
        typical_price = (self.high + self.low + self.close) / 3
        cum_tp_vol = np.cumsum(typical_price * self.volume)
        cum_vol = np.cumsum(self.volume)

        result = np.full(self.n, np.nan)
        mask = cum_vol > 0
        result[mask] = cum_tp_vol[mask] / cum_vol[mask]
        return result

    def _calc_vwap(self, _period: Optional[int] = None) -> dict:
        values = self.vwap()
        return {
            "indicator": "vwap",
            "parameters": {},
            "values": self._to_list(values),
            "signal": "NEUTRAL",
        }

    # ==================================================
    # 52-Week High/Low Distance
    # ==================================================
    def week52_distance(self) -> dict:
        """Calculate distance from 52-week high and low."""
        if self.n < 252:
            lookback = self.n
        else:
            lookback = 252

        recent = self.close[-lookback:]
        high_52w = np.max(self.high[-lookback:])
        low_52w = np.min(self.low[-lookback:])
        current = self.close[-1]

        high_dist = ((current - high_52w) / high_52w) * 100 if high_52w > 0 else None
        low_dist = ((current - low_52w) / low_52w) * 100 if low_52w > 0 else None

        return {
            "high_52w": float(high_52w),
            "low_52w": float(low_52w),
            "current": float(current),
            "distance_from_high_pct": round(high_dist, 2) if high_dist else None,
            "distance_from_low_pct": round(low_dist, 2) if low_dist else None,
        }

    # ==================================================
    # Helpers
    # ==================================================
    def _to_list(self, arr: np.ndarray) -> list:
        """Convert numpy array to list, replacing NaN with None."""
        return [None if np.isnan(v) else round(float(v), 6) for v in arr]

    def _last_valid(self, arr: np.ndarray) -> Optional[float]:
        """Get the last non-NaN value from an array."""
        valid = arr[~np.isnan(arr)]
        return float(valid[-1]) if len(valid) > 0 else None

    def _trend_signal(self, indicator_values: np.ndarray) -> str:
        """Determine trend signal based on price vs indicator."""
        last_val = self._last_valid(indicator_values)
        if last_val is None or self.n == 0:
            return "NEUTRAL"
        if self.close[-1] > last_val:
            return "BULLISH"
        elif self.close[-1] < last_val:
            return "BEARISH"
        return "NEUTRAL"
