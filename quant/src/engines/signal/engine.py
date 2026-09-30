"""
Signal Engine — Composite Weighted Signal Calculator

Combines 9 scoring dimensions with configurable weights to produce
a composite signal: STRONG_BUY, BUY, WATCH, AVOID, STRONG_AVOID.

CRITICAL RULE: A Shariah FAIL always forces the signal to AVOID.
The signal engine NEVER overrides a failed Shariah screen.

Signal weights (configurable):
  Fundamental:   30%
  Technical:     20%
  Candlestick:   10%
  Momentum:      10%
  Volume:         5%
  Valuation:     10%
  Market Regime:  5%
  Risk:           5%
  Quality:        5%
"""

from dataclasses import dataclass, field
from enum import Enum
from typing import Optional


class Signal(Enum):
    STRONG_BUY = "STRONG_BUY"
    BUY = "BUY"
    WATCH = "WATCH"
    AVOID = "AVOID"
    STRONG_AVOID = "STRONG_AVOID"


@dataclass
class SignalWeights:
    """Configurable weights for signal dimensions. Must sum to 1.0."""
    fundamental: float = 0.30
    technical: float = 0.20
    candlestick: float = 0.10
    momentum: float = 0.10
    volume: float = 0.05
    valuation: float = 0.10
    market_regime: float = 0.05
    risk: float = 0.05
    quality: float = 0.05

    def validate(self) -> bool:
        total = (
            self.fundamental + self.technical + self.candlestick +
            self.momentum + self.volume + self.valuation +
            self.market_regime + self.risk + self.quality
        )
        return abs(total - 1.0) < 0.001


@dataclass
class DimensionScore:
    """Score for a single signal dimension."""
    name: str
    score: float  # 0-100
    weight: float
    contributing_factors: list[str] = field(default_factory=list)
    conflicting_factors: list[str] = field(default_factory=list)


@dataclass
class SignalResult:
    """Complete signal calculation result."""
    signal: Signal
    overall_score: float  # 0-100
    dimensions: list[DimensionScore]
    shariah_status: str
    shariah_override: bool  # True if signal was forced to AVOID due to Shariah
    explanation: str
    supporting_factors: list[str]
    conflicting_factors: list[str]
    confidence: float  # 0-100, based on data completeness
    disclaimer: str = (
        "This signal is a statistical analysis output based on historical data. "
        "It is NOT a guaranteed trading recommendation. Past performance does "
        "not guarantee future results. Always conduct your own due diligence."
    )


class SignalEngine:
    """
    Calculates composite weighted signals from multiple scoring dimensions.

    INVARIANT: Shariah FAIL → signal is ALWAYS AVOID, regardless of other scores.
    """

    def __init__(self, weights: Optional[SignalWeights] = None):
        self.weights = weights or SignalWeights()
        if not self.weights.validate():
            raise ValueError("Signal weights must sum to 1.0")

    def calculate(
        self,
        fundamental_score: Optional[float] = None,
        technical_score: Optional[float] = None,
        candlestick_score: Optional[float] = None,
        momentum_score: Optional[float] = None,
        volume_score: Optional[float] = None,
        valuation_score: Optional[float] = None,
        market_regime_score: Optional[float] = None,
        risk_score: Optional[float] = None,
        quality_score: Optional[float] = None,
        shariah_status: str = "PASS",
        fundamental_factors: Optional[list[str]] = None,
        technical_factors: Optional[list[str]] = None,
        conflicting: Optional[list[str]] = None,
    ) -> SignalResult:
        """
        Calculate the composite signal.

        Scores are 0-100. Missing scores contribute 50 (neutral) and
        reduce confidence proportionally.
        """
        dimensions = []
        total_available = 0
        total_dimensions = 9

        def add_dim(name: str, score: Optional[float], weight: float):
            nonlocal total_available
            actual_score = score if score is not None else 50.0
            if score is not None:
                total_available += 1
            dimensions.append(DimensionScore(
                name=name,
                score=actual_score,
                weight=weight,
            ))

        add_dim("fundamental", fundamental_score, self.weights.fundamental)
        add_dim("technical", technical_score, self.weights.technical)
        add_dim("candlestick", candlestick_score, self.weights.candlestick)
        add_dim("momentum", momentum_score, self.weights.momentum)
        add_dim("volume", volume_score, self.weights.volume)
        add_dim("valuation", valuation_score, self.weights.valuation)
        add_dim("market_regime", market_regime_score, self.weights.market_regime)
        add_dim("risk", risk_score, self.weights.risk)
        add_dim("quality", quality_score, self.weights.quality)

        # Weighted score calculation
        overall = sum(d.score * d.weight for d in dimensions)
        overall = round(max(0, min(100, overall)), 2)

        # Confidence based on data completeness
        confidence = round((total_available / total_dimensions) * 100, 1)

        # Determine signal from score
        signal = self._score_to_signal(overall)

        # CRITICAL: Shariah FAIL forces AVOID
        shariah_override = False
        if shariah_status == "FAIL":
            signal = Signal.AVOID
            shariah_override = True

        # Build explanation
        supporting = fundamental_factors or []
        conflicting_list = conflicting or []
        explanation = self._build_explanation(signal, overall, dimensions, shariah_override)

        return SignalResult(
            signal=signal,
            overall_score=overall,
            dimensions=dimensions,
            shariah_status=shariah_status,
            shariah_override=shariah_override,
            explanation=explanation,
            supporting_factors=supporting,
            conflicting_factors=conflicting_list,
            confidence=confidence,
        )

    def _score_to_signal(self, score: float) -> Signal:
        """Convert numeric score to signal enum."""
        if score >= 80:
            return Signal.STRONG_BUY
        elif score >= 65:
            return Signal.BUY
        elif score >= 50:
            return Signal.WATCH
        elif score >= 35:
            return Signal.AVOID
        else:
            return Signal.STRONG_AVOID

    def _build_explanation(
        self, signal: Signal, score: float,
        dimensions: list[DimensionScore], shariah_override: bool
    ) -> str:
        """Build human-readable explanation of the signal."""
        parts = [f"Composite score: {score:.1f}/100"]

        if shariah_override:
            parts.append("⚠️ SHARIAH FAIL: Signal forced to AVOID regardless of score.")

        # Top contributors
        sorted_dims = sorted(dimensions, key=lambda d: d.score * d.weight, reverse=True)
        top = sorted_dims[:3]
        parts.append("Top contributors: " + ", ".join(
            f"{d.name} ({d.score:.0f} × {d.weight*100:.0f}%)" for d in top
        ))

        # Weak areas
        weak = [d for d in dimensions if d.score < 40]
        if weak:
            parts.append("Weak areas: " + ", ".join(
                f"{d.name} ({d.score:.0f})" for d in weak
            ))

        return " | ".join(parts)
