"""
Candlestick Pattern Detection Engine

Detects 17 candlestick patterns and evaluates them in context.
A pattern alone does NOT generate a BUY signal — context is required.

Patterns detected:
- Doji, Dragonfly Doji, Gravestone Doji
- Hammer, Inverted Hammer, Hanging Man, Shooting Star
- Marubozu, Spinning Top
- Bullish Engulfing, Bearish Engulfing
- Piercing Line, Dark Cloud Cover
- Morning Star, Evening Star
- Three White Soldiers, Three Black Crows
"""

import numpy as np
from dataclasses import dataclass
from typing import Optional


@dataclass
class CandlePattern:
    """Detected candlestick pattern with context."""
    name: str
    pattern_type: str  # BULLISH, BEARISH, NEUTRAL
    index: int  # position in the data
    confidence: float  # 0-100
    # Context evaluation
    trend_context: Optional[str] = None
    support_resistance: Optional[str] = None
    volume_context: Optional[str] = None
    atr_value: Optional[float] = None
    rsi_value: Optional[float] = None
    macd_context: Optional[str] = None
    market_regime: Optional[str] = None
    signal_contribution: float = 0.0


class CandlestickEngine:
    """
    Detects candlestick patterns and evaluates them with market context.

    IMPORTANT: A candlestick pattern alone does NOT generate a BUY signal.
    Context (trend, S/R, volume, ATR, RSI, MACD, regime) is always evaluated.
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

    def body(self, i: int) -> float:
        """Absolute size of candle body."""
        return abs(self.close[i] - self.open[i])

    def body_pct(self, i: int) -> float:
        """Body size as percentage of total range."""
        total_range = self.high[i] - self.low[i]
        if total_range == 0:
            return 0
        return self.body(i) / total_range

    def upper_shadow(self, i: int) -> float:
        """Upper shadow length."""
        return self.high[i] - max(self.open[i], self.close[i])

    def lower_shadow(self, i: int) -> float:
        """Lower shadow length."""
        return min(self.open[i], self.close[i]) - self.low[i]

    def is_bullish(self, i: int) -> bool:
        """True if close > open (green candle)."""
        return self.close[i] > self.open[i]

    def is_bearish(self, i: int) -> bool:
        """True if close < open (red candle)."""
        return self.close[i] < self.open[i]

    def avg_body(self, i: int, lookback: int = 14) -> float:
        """Average body size over lookback period."""
        start = max(0, i - lookback)
        bodies = [self.body(j) for j in range(start, i)]
        return np.mean(bodies) if bodies else 0

    def detect_all(self) -> list[CandlePattern]:
        """Detect all candlestick patterns in the data."""
        patterns = []

        for i in range(self.n):
            avg_b = self.avg_body(i)

            # Single candle patterns
            if self._is_doji(i, avg_b):
                patterns.append(CandlePattern("Doji", "NEUTRAL", i, 60))
            if self._is_dragonfly_doji(i, avg_b):
                patterns.append(CandlePattern("Dragonfly Doji", "BULLISH", i, 65))
            if self._is_gravestone_doji(i, avg_b):
                patterns.append(CandlePattern("Gravestone Doji", "BEARISH", i, 65))
            if self._is_hammer(i, avg_b):
                patterns.append(CandlePattern("Hammer", "BULLISH", i, 60))
            if self._is_inverted_hammer(i, avg_b):
                patterns.append(CandlePattern("Inverted Hammer", "BULLISH", i, 55))
            if self._is_hanging_man(i, avg_b):
                patterns.append(CandlePattern("Hanging Man", "BEARISH", i, 60))
            if self._is_shooting_star(i, avg_b):
                patterns.append(CandlePattern("Shooting Star", "BEARISH", i, 65))
            if self._is_marubozu(i, avg_b):
                ptype = "BULLISH" if self.is_bullish(i) else "BEARISH"
                patterns.append(CandlePattern("Marubozu", ptype, i, 70))
            if self._is_spinning_top(i, avg_b):
                patterns.append(CandlePattern("Spinning Top", "NEUTRAL", i, 40))

            # Two candle patterns (require i >= 1)
            if i >= 1:
                if self._is_bullish_engulfing(i):
                    patterns.append(CandlePattern("Bullish Engulfing", "BULLISH", i, 75))
                if self._is_bearish_engulfing(i):
                    patterns.append(CandlePattern("Bearish Engulfing", "BEARISH", i, 75))
                if self._is_piercing_line(i):
                    patterns.append(CandlePattern("Piercing Line", "BULLISH", i, 65))
                if self._is_dark_cloud_cover(i):
                    patterns.append(CandlePattern("Dark Cloud Cover", "BEARISH", i, 65))

            # Three candle patterns (require i >= 2)
            if i >= 2:
                if self._is_morning_star(i):
                    patterns.append(CandlePattern("Morning Star", "BULLISH", i, 80))
                if self._is_evening_star(i):
                    patterns.append(CandlePattern("Evening Star", "BEARISH", i, 80))
                if self._is_three_white_soldiers(i):
                    patterns.append(CandlePattern("Three White Soldiers", "BULLISH", i, 80))
                if self._is_three_black_crows(i):
                    patterns.append(CandlePattern("Three Black Crows", "BEARISH", i, 80))

        return patterns

    # ==================================================
    # Single Candle Patterns
    # ==================================================

    def _is_doji(self, i: int, avg_b: float) -> bool:
        """Doji: Very small body relative to range."""
        return self.body_pct(i) < 0.1 and (self.high[i] - self.low[i]) > 0

    def _is_dragonfly_doji(self, i: int, avg_b: float) -> bool:
        """Dragonfly Doji: Doji with long lower shadow, no upper shadow."""
        total = self.high[i] - self.low[i]
        if total == 0:
            return False
        return (
            self.body_pct(i) < 0.1
            and self.lower_shadow(i) / total > 0.6
            and self.upper_shadow(i) / total < 0.1
        )

    def _is_gravestone_doji(self, i: int, avg_b: float) -> bool:
        """Gravestone Doji: Doji with long upper shadow, no lower shadow."""
        total = self.high[i] - self.low[i]
        if total == 0:
            return False
        return (
            self.body_pct(i) < 0.1
            and self.upper_shadow(i) / total > 0.6
            and self.lower_shadow(i) / total < 0.1
        )

    def _is_hammer(self, i: int, avg_b: float) -> bool:
        """Hammer: Small body at top, long lower shadow (2x body)."""
        b = self.body(i)
        ls = self.lower_shadow(i)
        us = self.upper_shadow(i)
        return b > 0 and ls >= 2 * b and us <= b * 0.3

    def _is_inverted_hammer(self, i: int, avg_b: float) -> bool:
        """Inverted Hammer: Small body at bottom, long upper shadow."""
        b = self.body(i)
        ls = self.lower_shadow(i)
        us = self.upper_shadow(i)
        return b > 0 and us >= 2 * b and ls <= b * 0.3

    def _is_hanging_man(self, i: int, avg_b: float) -> bool:
        """Hanging Man: Same shape as hammer but appears in uptrend."""
        if not self._is_hammer(i, avg_b):
            return False
        # Check for prior uptrend (simple: last 5 closes rising)
        if i < 5:
            return False
        return all(self.close[j] < self.close[j + 1] for j in range(i - 5, i - 1))

    def _is_shooting_star(self, i: int, avg_b: float) -> bool:
        """Shooting Star: Same shape as inverted hammer but in uptrend."""
        if not self._is_inverted_hammer(i, avg_b):
            return False
        if i < 5:
            return False
        return all(self.close[j] < self.close[j + 1] for j in range(i - 5, i - 1))

    def _is_marubozu(self, i: int, avg_b: float) -> bool:
        """Marubozu: Full body candle with very small or no shadows."""
        total = self.high[i] - self.low[i]
        if total == 0:
            return False
        return self.body_pct(i) > 0.9

    def _is_spinning_top(self, i: int, avg_b: float) -> bool:
        """Spinning Top: Small body with upper and lower shadows."""
        total = self.high[i] - self.low[i]
        if total == 0:
            return False
        return (
            0.1 < self.body_pct(i) < 0.35
            and self.upper_shadow(i) > self.body(i) * 0.5
            and self.lower_shadow(i) > self.body(i) * 0.5
        )

    # ==================================================
    # Two Candle Patterns
    # ==================================================

    def _is_bullish_engulfing(self, i: int) -> bool:
        """Bullish Engulfing: Bearish candle followed by larger bullish candle."""
        return (
            self.is_bearish(i - 1)
            and self.is_bullish(i)
            and self.open[i] <= self.close[i - 1]
            and self.close[i] >= self.open[i - 1]
            and self.body(i) > self.body(i - 1)
        )

    def _is_bearish_engulfing(self, i: int) -> bool:
        """Bearish Engulfing: Bullish candle followed by larger bearish candle."""
        return (
            self.is_bullish(i - 1)
            and self.is_bearish(i)
            and self.open[i] >= self.close[i - 1]
            and self.close[i] <= self.open[i - 1]
            and self.body(i) > self.body(i - 1)
        )

    def _is_piercing_line(self, i: int) -> bool:
        """Piercing Line: Bearish candle, then bullish opening below and closing above midpoint."""
        if not (self.is_bearish(i - 1) and self.is_bullish(i)):
            return False
        midpoint = (self.open[i - 1] + self.close[i - 1]) / 2
        return self.open[i] < self.close[i - 1] and self.close[i] > midpoint

    def _is_dark_cloud_cover(self, i: int) -> bool:
        """Dark Cloud Cover: Bullish candle, then bearish opening above and closing below midpoint."""
        if not (self.is_bullish(i - 1) and self.is_bearish(i)):
            return False
        midpoint = (self.open[i - 1] + self.close[i - 1]) / 2
        return self.open[i] > self.close[i - 1] and self.close[i] < midpoint

    # ==================================================
    # Three Candle Patterns
    # ==================================================

    def _is_morning_star(self, i: int) -> bool:
        """Morning Star: Large bearish, small body, large bullish."""
        avg_b = self.avg_body(i)
        return (
            self.is_bearish(i - 2)
            and self.body(i - 2) > avg_b
            and self.body(i - 1) < avg_b * 0.3
            and self.is_bullish(i)
            and self.body(i) > avg_b
            and self.close[i] > (self.open[i - 2] + self.close[i - 2]) / 2
        )

    def _is_evening_star(self, i: int) -> bool:
        """Evening Star: Large bullish, small body, large bearish."""
        avg_b = self.avg_body(i)
        return (
            self.is_bullish(i - 2)
            and self.body(i - 2) > avg_b
            and self.body(i - 1) < avg_b * 0.3
            and self.is_bearish(i)
            and self.body(i) > avg_b
            and self.close[i] < (self.open[i - 2] + self.close[i - 2]) / 2
        )

    def _is_three_white_soldiers(self, i: int) -> bool:
        """Three White Soldiers: Three consecutive bullish candles with higher closes."""
        return (
            self.is_bullish(i - 2)
            and self.is_bullish(i - 1)
            and self.is_bullish(i)
            and self.close[i - 1] > self.close[i - 2]
            and self.close[i] > self.close[i - 1]
            and self.open[i - 1] > self.open[i - 2]
            and self.open[i] > self.open[i - 1]
        )

    def _is_three_black_crows(self, i: int) -> bool:
        """Three Black Crows: Three consecutive bearish candles with lower closes."""
        return (
            self.is_bearish(i - 2)
            and self.is_bearish(i - 1)
            and self.is_bearish(i)
            and self.close[i - 1] < self.close[i - 2]
            and self.close[i] < self.close[i - 1]
            and self.open[i - 1] < self.open[i - 2]
            and self.open[i] < self.open[i - 1]
        )
