"""
Unit tests for Candlestick Pattern Detection Engine.
"""

import pytest
from src.engines.candlestick.patterns import CandlestickEngine


class TestSingleCandlePatterns:
    def test_doji_detection(self):
        """Doji: open ≈ close with range."""
        engine = CandlestickEngine(
            open_prices=[100.0],
            high_prices=[102.0],
            low_prices=[98.0],
            close_prices=[100.05],
            volumes=[1000],
        )
        patterns = engine.detect_all()
        dojis = [p for p in patterns if p.name == "Doji"]
        assert len(dojis) == 1

    def test_hammer_detection(self):
        """Hammer: small body at top, long lower shadow."""
        # Last candle: open=99.0, high=99.6, low=96.0, close=99.5
        # body=0.5, lower_shadow=3.0(≥2*0.5), upper_shadow=0.1(≤0.3*0.5)
        engine = CandlestickEngine(
            open_prices=[100, 100, 100, 100, 100, 99.0],
            high_prices=[101, 101, 101, 101, 101, 99.6],
            low_prices=[99, 99, 99, 99, 99, 96.0],
            close_prices=[100.5, 100.5, 100.5, 100.5, 100.5, 99.5],
            volumes=[1000, 1000, 1000, 1000, 1000, 1000],
        )
        patterns = engine.detect_all()
        hammers = [p for p in patterns if p.name == "Hammer"]
        assert len(hammers) >= 1

    def test_marubozu_detection(self):
        """Marubozu: full body candle with minimal shadows."""
        engine = CandlestickEngine(
            open_prices=[100.0],
            high_prices=[105.0],
            low_prices=[100.0],
            close_prices=[105.0],
            volumes=[1000],
        )
        patterns = engine.detect_all()
        marubozus = [p for p in patterns if p.name == "Marubozu"]
        assert len(marubozus) == 1
        assert marubozus[0].pattern_type == "BULLISH"


class TestTwoCandlePatterns:
    def test_bullish_engulfing(self):
        """Bullish Engulfing: bearish candle engulfed by larger bullish."""
        engine = CandlestickEngine(
            open_prices=[105, 99],
            high_prices=[106, 107],
            low_prices=[99, 98],
            close_prices=[100, 106],
            volumes=[1000, 2000],
        )
        patterns = engine.detect_all()
        engulfing = [p for p in patterns if p.name == "Bullish Engulfing"]
        assert len(engulfing) == 1
        assert engulfing[0].pattern_type == "BULLISH"

    def test_bearish_engulfing(self):
        """Bearish Engulfing: bullish candle engulfed by larger bearish."""
        engine = CandlestickEngine(
            open_prices=[100, 106],
            high_prices=[106, 107],
            low_prices=[99, 98],
            close_prices=[105, 99],
            volumes=[1000, 2000],
        )
        patterns = engine.detect_all()
        engulfing = [p for p in patterns if p.name == "Bearish Engulfing"]
        assert len(engulfing) == 1
        assert engulfing[0].pattern_type == "BEARISH"


class TestThreeCandlePatterns:
    def test_three_white_soldiers(self):
        """Three White Soldiers: three consecutive bullish candles."""
        engine = CandlestickEngine(
            open_prices=[100, 103, 106],
            high_prices=[104, 107, 110],
            low_prices=[99, 102, 105],
            close_prices=[103, 106, 109],
            volumes=[1000, 1200, 1400],
        )
        patterns = engine.detect_all()
        soldiers = [p for p in patterns if p.name == "Three White Soldiers"]
        assert len(soldiers) == 1
        assert soldiers[0].pattern_type == "BULLISH"

    def test_three_black_crows(self):
        """Three Black Crows: three consecutive bearish candles."""
        engine = CandlestickEngine(
            open_prices=[110, 107, 104],
            high_prices=[111, 108, 105],
            low_prices=[106, 103, 100],
            close_prices=[107, 104, 101],
            volumes=[1000, 1200, 1400],
        )
        patterns = engine.detect_all()
        crows = [p for p in patterns if p.name == "Three Black Crows"]
        assert len(crows) == 1
        assert crows[0].pattern_type == "BEARISH"


class TestPatternContext:
    def test_pattern_has_confidence(self):
        """Every detected pattern must have a confidence score."""
        engine = CandlestickEngine(
            open_prices=[100.0],
            high_prices=[102.0],
            low_prices=[98.0],
            close_prices=[100.05],
            volumes=[1000],
        )
        patterns = engine.detect_all()
        for p in patterns:
            assert 0 <= p.confidence <= 100

    def test_pattern_has_type(self):
        """Every pattern must have a type (BULLISH, BEARISH, NEUTRAL)."""
        engine = CandlestickEngine(
            open_prices=[100.0],
            high_prices=[102.0],
            low_prices=[98.0],
            close_prices=[100.05],
            volumes=[1000],
        )
        patterns = engine.detect_all()
        for p in patterns:
            assert p.pattern_type in ("BULLISH", "BEARISH", "NEUTRAL")
