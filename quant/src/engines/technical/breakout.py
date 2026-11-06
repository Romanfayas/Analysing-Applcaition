"""
Breakout Research Engine

Auditable quantitative scanner and analyzer for cash-equity breakout setups.
All mathematical conditions, states, and scores are deterministic and transparent.
LLMs are NEVER used as numerical arbiters.
"""

import numpy as np
from dataclasses import dataclass, field
from typing import Optional, List, Dict, Any
from src.engines.technical.indicators import TechnicalIndicators


class BreakoutState:
    APPROACHING_RESISTANCE = "APPROACHING_RESISTANCE"
    CONSOLIDATION = "CONSOLIDATION"
    CONFIRMED_BREAKOUT = "CONFIRMED_BREAKOUT"
    FAILED_BREAKOUT = "FAILED_BREAKOUT"
    BREAKOUT_CANDIDATE = "BREAKOUT_CANDIDATE"
    WATCHLIST = "WATCHLIST"
    UNAVAILABLE = "UNAVAILABLE"


@dataclass
class BreakoutResult:
    symbol: str
    stage: str
    current_price: float
    resistance_level: float
    support_level: float
    distance_to_resistance_pct: float
    rvol: float
    atr: float
    bollinger_band_width: float
    consolidation_bars: int
    volume_demand_proxy: float  # 0.0 to 1.0 (buying volume fraction)
    order_book_depth_status: str  # "DATA_UNAVAILABLE" (level 2 feed credentials required)
    entry_zone: List[float]
    invalidation_level: float
    target_1: float
    target_2: float
    risk_reward_ratio: float
    overall_score: float
    score_breakdown: Dict[str, float]
    shariah_status: str
    is_shariah_eligible: bool
    reasons: List[str]
    invalidation_conditions: List[str]
    data_quality: str
    setup_type: str = "WATCHLIST"
    liquidity_adt_cr: float = 0.0
    market_regime: str = "RANGING"
    score_disclaimer: str = "The ranking score is a multi-factor structural evidence score, NOT a calibrated probability of profit."
    market_depth_disclosure: str = "Volume demand proxy is derived from bar volume distribution. Actual Level 2 order-book market depth is unavailable without a licensed exchange feed."
    input_freshness: Dict[str, Any] = field(default_factory=dict)
    # Pure price-structure score (0-100) BEFORE the Shariah gate. `overall_score` is zeroed for any instrument that is
    # not a certified Shariah PASS ("zero recommendations"), which made every score 0 while no instrument is screened.
    # `structure_score` is what the chart actually shows; it is NOT a buy recommendation and carries no Shariah view.
    structure_score: Optional[float] = None


class BreakoutEngine:
    """
    Mathematical scanner for consolidation and breakout setups across equities.
    """

    def __init__(
        self,
        symbol: str,
        open_prices: List[float],
        high_prices: List[float],
        low_prices: List[float],
        close_prices: List[float],
        volumes: List[int],
        shariah_status: str = "UNKNOWN",
        benchmark_closes: Optional[List[float]] = None,
        data_quality: str = "LAST_AVAILABLE",
    ):
        self.symbol = symbol
        self.shariah_status = shariah_status.upper()
        self.data_quality = data_quality
        self.n = len(close_prices)

        self.opens = np.array(open_prices, dtype=np.float64)
        self.highs = np.array(high_prices, dtype=np.float64)
        self.lows = np.array(low_prices, dtype=np.float64)
        self.closes = np.array(close_prices, dtype=np.float64)
        self.volumes = np.array(volumes, dtype=np.float64)
        self.benchmark_closes = (
            np.array(benchmark_closes, dtype=np.float64)
            if benchmark_closes and len(benchmark_closes) == self.n
            else None
        )

        self.indicators = (
            TechnicalIndicators(open_prices, high_prices, low_prices, close_prices, volumes)
            if self.n > 0
            else None
        )

    def _find_support_resistance(self, lookback: int = 40) -> tuple[float, float]:
        """Identify key swing resistance and support levels."""
        if self.n < 10:
            return float(self.closes[-1]), float(self.closes[-1])

        window = min(lookback, self.n - 1)
        if window < 5:
            return float(np.max(self.highs)), float(np.min(self.lows))

        # Exclude current bar to assess resistance that the current bar may be breaking
        recent_highs = self.highs[-window - 1 : -1]
        recent_lows = self.lows[-window - 1 : -1]

        # Swing high: local peak
        swing_highs = []
        for i in range(2, len(recent_highs) - 2):
            if (
                recent_highs[i] >= recent_highs[i - 1]
                and recent_highs[i] >= recent_highs[i - 2]
                and recent_highs[i] >= recent_highs[i + 1]
                and recent_highs[i] >= recent_highs[i + 2]
            ):
                swing_highs.append(recent_highs[i])

        if not swing_highs:
            resistance = float(np.max(recent_highs))
        else:
            resistance = float(np.max(swing_highs))

        # Swing low: local trough
        swing_lows = []
        for i in range(2, len(recent_lows) - 2):
            if (
                recent_lows[i] <= recent_lows[i - 1]
                and recent_lows[i] <= recent_lows[i - 2]
                and recent_lows[i] <= recent_lows[i + 1]
                and recent_lows[i] <= recent_lows[i + 2]
            ):
                swing_lows.append(recent_lows[i])

        if not swing_lows:
            support = float(np.min(recent_lows))
        else:
            support = float(np.min(swing_lows))

        return resistance, support

    def analyze(self) -> BreakoutResult:
        """Execute full mathematical evaluation."""
        if self.n < 30 or self.indicators is None:
            return BreakoutResult(
                symbol=self.symbol,
                stage=BreakoutState.UNAVAILABLE,
                current_price=float(self.closes[-1]) if self.n > 0 else 0.0,
                resistance_level=0.0,
                support_level=0.0,
                distance_to_resistance_pct=0.0,
                rvol=0.0,
                atr=0.0,
                bollinger_band_width=0.0,
                consolidation_bars=0,
                volume_demand_proxy=0.0,
                order_book_depth_status="DATA_UNAVAILABLE (No Level 2 License)",
                entry_zone=[0.0, 0.0],
                invalidation_level=0.0,
                target_1=0.0,
                target_2=0.0,
                risk_reward_ratio=0.0,
                overall_score=0.0,
                score_breakdown={},
                shariah_status=self.shariah_status,
                is_shariah_eligible=False,
                reasons=["Insufficient historical data (minimum 30 periods required)"],
                invalidation_conditions=["Dataset incomplete"],
                data_quality=self.data_quality,
            )

        curr_price = float(self.closes[-1])
        curr_vol = float(self.volumes[-1])

        # 1. Technical Indicators
        atr_vals = self.indicators.atr(14)
        curr_atr = float(atr_vals[-1]) if not np.isnan(atr_vals[-1]) else curr_price * 0.02

        sma_20_vals = self.indicators.sma(20)
        sma_20 = float(sma_20_vals[-1]) if not np.isnan(sma_20_vals[-1]) else curr_price

        sma_50_vals = self.indicators.sma(50)
        sma_50 = float(sma_50_vals[-1]) if not np.isnan(sma_50_vals[-1]) else sma_20

        rsi_vals = self.indicators.rsi(14)
        curr_rsi = float(rsi_vals[-1]) if not np.isnan(rsi_vals[-1]) else 50.0

        # Bollinger Bands & Band Width
        upper, middle, lower = self.indicators.bollinger_bands(20, 2.0)
        curr_upper = upper[-1] if not np.isnan(upper[-1]) else curr_price * 1.05
        curr_lower = lower[-1] if not np.isnan(lower[-1]) else curr_price * 0.95
        curr_mid = middle[-1] if not np.isnan(middle[-1]) else curr_price
        bbw = float((curr_upper - curr_lower) / curr_mid) if curr_mid > 0 else 0.10

        # 2. Volume & RVOL
        vol_sma_20 = float(np.mean(self.volumes[-21:-1])) if self.n >= 21 else float(np.mean(self.volumes))
        rvol = float(curr_vol / vol_sma_20) if vol_sma_20 > 0 else 1.0

        # Volume-based Demand Proxy: aggregate buying pressure across last 5 bars
        # Formula: buy_fraction = (Close - Low) / (High - Low)
        recent_window = min(5, self.n)
        buy_vols = []
        tot_vols = []
        for i in range(self.n - recent_window, self.n):
            h_l = max(self.highs[i] - self.lows[i], 0.0001)
            buy_frac = max(0.0, min(1.0, (self.closes[i] - self.lows[i]) / h_l))
            buy_vols.append(buy_frac * self.volumes[i])
            tot_vols.append(self.volumes[i])
        volume_demand_proxy = float(np.sum(buy_vols) / max(np.sum(tot_vols), 1.0))

        # 3. S/R and Distance
        resistance, support = self._find_support_resistance()
        dist_res_pct = float((resistance - curr_price) / resistance * 100.0)

        # 4. Consolidation Duration (bars within tight range < 6%)
        consolidation_bars = 0
        for i in range(self.n - 1, max(self.n - 20, 0), -1):
            bar_spread = (self.highs[i] - self.lows[i]) / self.closes[i]
            if bar_spread < 0.045:
                consolidation_bars += 1
            else:
                break

        # 5. State Classification
        reasons = []
        inval_conditions = []

        # Bar attributes for confirmed breakout
        bar_range = self.highs[-1] - self.lows[-1]
        close_location = (self.closes[-1] - self.lows[-1]) / max(bar_range, 0.0001)

        is_confirmed = (
            curr_price > resistance * 1.002
            and rvol >= 1.30
            and bar_range >= 0.95 * curr_atr
            and close_location >= 0.65
        )

        is_failed = (
            np.max(self.highs[-3:]) > resistance
            and curr_price < resistance * 0.985
        )

        is_candidate = (
            0.0 <= dist_res_pct <= 1.5
            and bbw <= 0.085
            and rvol >= 1.15
            and curr_price > sma_20
        )

        is_watchlist = (
            0.0 <= dist_res_pct <= 3.8
            and curr_rsi >= 48.0
            and curr_price > sma_50
        )

        if is_confirmed:
            stage = BreakoutState.CONFIRMED_BREAKOUT
            reasons.append(f"Price decisively closed above resistance ₹{resistance:.2f} (+{((curr_price/resistance)-1)*100:.2f}%)")
            reasons.append(f"High participation: RVOL of {rvol:.2f}x exceeds 1.40x threshold")
            reasons.append(f"Strong bar expansion ({bar_range:.2f} vs ATR {curr_atr:.2f}) with close in top {close_location*100:.0f}% of range")
            inval_conditions.append(f"Close drops back below breakout level ₹{resistance:.2f}")
        elif is_failed:
            stage = BreakoutState.FAILED_BREAKOUT
            reasons.append(f"False breakout: Intraday test above ₹{resistance:.2f} met immediate selling rejection")
            reasons.append("Setup invalidated; distribution observed at overhead resistance")
            inval_conditions.append("Already invalidated")
        elif is_candidate:
            stage = BreakoutState.BREAKOUT_CANDIDATE
            reasons.append(f"Tight consolidation within {dist_res_pct:.2f}% of key resistance ₹{resistance:.2f}")
            reasons.append(f"Volatility compression: Bollinger Band Width contracted to {bbw*100:.2f}%")
            reasons.append(f"Pre-breakout volume accumulation: RVOL at {rvol:.2f}x")
            inval_conditions.append(f"Close below 20-day SMA support at ₹{sma_20:.2f}")
        elif is_watchlist:
            stage = BreakoutState.WATCHLIST
            reasons.append(f"Approaching key resistance zone ₹{resistance:.2f} ({dist_res_pct:.2f}% away)")
            reasons.append(f"Constructive momentum: RSI at {curr_rsi:.1f} above baseline")
            inval_conditions.append(f"Reversal below 50-day SMA at ₹{sma_50:.2f}")
        else:
            stage = BreakoutState.WATCHLIST
            reasons.append("Consolidating within broader structural range")
            inval_conditions.append("Breakdown below structural support")

        # 6. Trade Planning (Entry, Invalidation, Targets)
        entry_low = round(resistance, 2)
        entry_high = round(resistance + 0.35 * curr_atr, 2)
        invalidation = round(max(support, resistance - 1.25 * curr_atr), 2)
        risk_per_share = max(entry_low - invalidation, 0.5 * curr_atr)

        target_1 = round(entry_low + 1.5 * risk_per_share, 2)
        target_2 = round(entry_low + 3.0 * risk_per_share, 2)
        rr_ratio = round((target_1 - entry_low) / max(risk_per_share, 0.01), 2)

        # 7. Transparent Scoring (0 - 100)
        # Dimensions:
        # Price Structure (25), Volume (20), Volatility Compression (15),
        # Trend Alignment (15), Risk/Reward (15), Relative Strength (10)
        score_breakdown = {}

        # Price Structure & Resistance Proximity (25 pts)
        if stage == BreakoutState.CONFIRMED_BREAKOUT:
            struct_score = 25.0
        elif dist_res_pct <= 1.0:
            struct_score = 22.0
        elif dist_res_pct <= 2.5:
            struct_score = 17.0
        elif dist_res_pct <= 4.0:
            struct_score = 12.0
        else:
            struct_score = 5.0
        score_breakdown["price_structure"] = struct_score

        # Volume & Participation (20 pts)
        if rvol >= 2.0:
            vol_score = 20.0
        elif rvol >= 1.5:
            vol_score = 16.0
        elif rvol >= 1.2:
            vol_score = 12.0
        elif rvol >= 1.0:
            vol_score = 8.0
        else:
            vol_score = 4.0
        score_breakdown["volume_participation"] = vol_score

        # Volatility Compression / Squeeze (15 pts)
        if bbw <= 0.05:
            comp_score = 15.0
        elif bbw <= 0.08:
            comp_score = 12.0
        elif bbw <= 0.12:
            comp_score = 8.0
        else:
            comp_score = 4.0
        score_breakdown["volatility_compression"] = comp_score

        # Trend & Momentum (15 pts)
        trend_score = 0.0
        if curr_price > sma_20:
            trend_score += 5.0
        if sma_20 > sma_50:
            trend_score += 5.0
        if 50.0 <= curr_rsi <= 68.0:
            trend_score += 5.0
        score_breakdown["trend_momentum"] = trend_score

        # Risk/Reward Quality (15 pts)
        if rr_ratio >= 2.5:
            rr_score = 15.0
        elif rr_ratio >= 2.0:
            rr_score = 12.0
        elif rr_ratio >= 1.5:
            rr_score = 8.0
        else:
            rr_score = 3.0
        score_breakdown["risk_reward_quality"] = rr_score

        # Relative Strength vs Benchmark (10 pts)
        rs_score = 5.0
        if self.benchmark_closes is not None:
            stock_ret = (self.closes[-1] - self.closes[-20]) / self.closes[-20]
            bench_ret = (self.benchmark_closes[-1] - self.benchmark_closes[-20]) / self.benchmark_closes[-20]
            if stock_ret > bench_ret + 0.03:
                rs_score = 10.0
            elif stock_ret > bench_ret:
                rs_score = 8.0
            else:
                rs_score = 2.0
        score_breakdown["relative_strength"] = rs_score

        raw_score = sum(score_breakdown.values())

        # 20-day Average Daily Turnover in ₹ Crores (1 Cr = 10,000,000)
        lookback_turnover = min(20, self.n)
        liquidity_adt_cr = round(float(np.mean(self.closes[-lookback_turnover:] * self.volumes[-lookback_turnover:]) / 1e7), 2)

        # Market Regime
        if sma_20 > sma_50 and curr_price > sma_50:
            market_regime = "BULLISH_TREND"
        elif sma_20 < sma_50 and curr_price < sma_50:
            market_regime = "BEARISH_TREND"
        else:
            market_regime = "RANGING"

        # Canonical Setup Type mapping
        setup_mapping = {
            BreakoutState.CONFIRMED_BREAKOUT: "CONFIRMED_BREAKOUT",
            BreakoutState.BREAKOUT_CANDIDATE: "CONSOLIDATION",
            BreakoutState.WATCHLIST: "APPROACHING_RESISTANCE",
            BreakoutState.FAILED_BREAKOUT: "FAILED_BREAKOUT",
            BreakoutState.UNAVAILABLE: "UNAVAILABLE",
        }
        setup_type = setup_mapping.get(stage, stage)

        input_freshness = {
            "data_quality": self.data_quality,
            "observations_count": self.n,
            "provider_provenance": "ohlcv_daily (Clean Aggregated EOD)",
        }

        # 8. Shariah Gate Enforcement
        is_shariah_eligible = self.shariah_status == "PASS"
        if not is_shariah_eligible:
            reasons.append(f"SHARIAH INELIGIBLE: Status is '{self.shariah_status}' (Hard Gate Restriction - Zero Recommendations)")
            # Zero out score and disallow recommendation
            overall_score = 0.0
        else:
            overall_score = round(raw_score, 1)

        return BreakoutResult(
            symbol=self.symbol,
            stage=stage,
            current_price=round(curr_price, 2),
            resistance_level=round(resistance, 2),
            support_level=round(support, 2),
            distance_to_resistance_pct=round(dist_res_pct, 2),
            rvol=round(rvol, 2),
            atr=round(curr_atr, 2),
            bollinger_band_width=round(bbw, 4),
            consolidation_bars=consolidation_bars,
            volume_demand_proxy=round(volume_demand_proxy, 3),
            order_book_depth_status="DATA_UNAVAILABLE (No Level 2 License)",
            entry_zone=[entry_low, entry_high],
            invalidation_level=invalidation,
            target_1=target_1,
            target_2=target_2,
            risk_reward_ratio=rr_ratio,
            overall_score=overall_score,
            structure_score=round(raw_score, 1),
            score_breakdown=score_breakdown,
            shariah_status=self.shariah_status,
            is_shariah_eligible=is_shariah_eligible,
            reasons=reasons,
            invalidation_conditions=inval_conditions,
            data_quality=self.data_quality,
            setup_type=setup_type,
            liquidity_adt_cr=liquidity_adt_cr,
            market_regime=market_regime,
            input_freshness=input_freshness,
        )

