from typing import List
from pydantic import BaseModel

class RegimeConfig(BaseModel):
    sma_fast_period: int = 50
    sma_slow_period: int = 200
    atr_period: int = 14

class RegimeClassifier:
    def __init__(self, config: RegimeConfig = RegimeConfig()):
        self.config = config

    def classify_period(self, prices: List[float], atrs: List[float]) -> str:
        """
        Classifies the market regime for a specific window based on SMA and ATR logic.
        (Implementation would compute the indicators if not passed, but assuming they are passed).
        """
        if not prices or len(prices) < self.config.sma_slow_period:
            return "INSUFFICIENT_DATA"

        fast_sma = sum(prices[-self.config.sma_fast_period:]) / self.config.sma_fast_period
        slow_sma = sum(prices[-self.config.sma_slow_period:]) / self.config.sma_slow_period
        
        current_atr = atrs[-1] if atrs else 0.0
        
        if fast_sma > slow_sma:
            return "BULL"
        elif fast_sma < slow_sma:
            return "BEAR"
        
        return "SIDEWAYS"
