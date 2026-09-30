"""
IPO Listing Prediction and Decision Engine.

Deterministically predicts IPO listing gains based on GMP, subscription data, and market sentiment,
and issues APPLY/WATCH/AVOID recommendations subject to strict Shariah screening overrides.
"""

from enum import Enum
from pydantic import BaseModel
from typing import Optional, Dict


class IPODecision(Enum):
    APPLY = "APPLY"
    WATCH = "WATCH"
    AVOID = "AVOID"


class IPOData(BaseModel):
    issue_price: float
    gmp: Optional[float] = None
    qib_subscription: Optional[float] = None
    nii_subscription: Optional[float] = None
    retail_subscription: Optional[float] = None
    issue_size_crores: Optional[float] = None
    shariah_status: str = "PASS"  # Must be PASS, FAIL, or REVIEW_REQUIRED


class PredictionResult(BaseModel):
    base_listing_price: float
    bull_listing_price: float
    bear_listing_price: float
    expected_gain_percent: float
    decision: IPODecision
    shariah_override: bool
    explanation: str


class IPOPredictor:
    def __init__(self):
        # Weights for subscription impact on listing gains
        self.qib_weight = 0.50
        self.nii_weight = 0.30
        self.retail_weight = 0.20
        
        # Max premium caps to prevent unrealistic predictions
        self.max_premium_multiplier = 2.0  # Max 200% listing gain prediction

    def predict(self, data: IPOData) -> PredictionResult:
        if data.issue_price <= 0:
            raise ValueError("Issue price must be greater than 0")

        # 1. Base prediction driven by GMP (Grey Market Premium)
        gmp_value = data.gmp if data.gmp is not None else 0.0
        gmp_premium_pct = gmp_value / data.issue_price
        
        # 2. Subscription Multiplier
        # Extremely high subscriptions push prices to the bull case
        qib_sub = data.qib_subscription or 1.0
        nii_sub = data.nii_subscription or 1.0
        retail_sub = data.retail_subscription or 1.0
        
        # Subscription score formula: log2(sub) to dampen astronomical subscriptions
        import math
        def sub_score(sub):
            return math.log2(sub) if sub > 1 else 0

        composite_sub_score = (
            sub_score(qib_sub) * self.qib_weight +
            sub_score(nii_sub) * self.nii_weight +
            sub_score(retail_sub) * self.retail_weight
        )
        
        # Subscriptions boost the GMP expectations (or create them if GMP is missing)
        # E.g., a composite score of 5 adds 5% to the premium
        sub_boost_pct = composite_sub_score * 0.01

        # 3. Calculate scenarios
        expected_premium_pct = gmp_premium_pct + sub_boost_pct
        
        # Cap expected premium
        expected_premium_pct = min(expected_premium_pct, self.max_premium_multiplier)
        
        base_price = data.issue_price * (1 + expected_premium_pct)
        bull_price = data.issue_price * (1 + expected_premium_pct + 0.10) # +10% in bull market
        bear_price = data.issue_price * (1 + expected_premium_pct - 0.15) # -15% in bear market (IPOs punish heavily)

        # Ensure bear price doesn't go below an unrealistic floor (e.g. 50% loss max)
        bear_price = max(bear_price, data.issue_price * 0.5)

        # 4. Decision Logic
        decision = IPODecision.WATCH
        explanation_parts = []
        
        if expected_premium_pct > 0.15 and composite_sub_score > 3:
            decision = IPODecision.APPLY
            explanation_parts.append(f"Strong listing gain expected ({expected_premium_pct*100:.1f}%) with solid subscription.")
        elif expected_premium_pct < 0.05:
            decision = IPODecision.AVOID
            explanation_parts.append(f"Low or negative expected premium ({expected_premium_pct*100:.1f}%).")
        else:
            decision = IPODecision.WATCH
            explanation_parts.append(f"Moderate expectations. Wait for final subscription data.")

        shariah_override = False
        if data.shariah_status == "FAIL":
            decision = IPODecision.AVOID
            shariah_override = True
            explanation_parts = ["SHARIAH FAIL: This IPO does not meet Shariah compliance criteria. Hard override to AVOID."] + explanation_parts

        return PredictionResult(
            base_listing_price=round(base_price, 2),
            bull_listing_price=round(bull_price, 2),
            bear_listing_price=round(bear_price, 2),
            expected_gain_percent=round(expected_premium_pct * 100, 2),
            decision=decision,
            shariah_override=shariah_override,
            explanation=" ".join(explanation_parts)
        )
