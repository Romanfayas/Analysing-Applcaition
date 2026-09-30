from datetime import datetime
from typing import Dict, Any, Optional
from pydantic import BaseModel

class MarketEvent(BaseModel):
    symbol: str
    exchange: str
    timestamp: datetime
    open: float
    high: float
    low: float
    close: float
    volume: int
    source: str
    data_status: str
    received_at: datetime

class MarketEventBus:
    """
    Normalizes incoming tick/bar data and routes it to the scheduler.
    Rejects stale or impossible data (Data Quality Normalizer).
    """
    def __init__(self):
        self.last_event_timestamps: Dict[str, datetime] = {}
        
    def process_event(self, event: MarketEvent) -> Optional[MarketEvent]:
        # 1. Quality Rules
        if event.high < event.low or event.open <= 0:
            # Impossible price
            return None
            
        # 2. Out of order detection
        last_time = self.last_event_timestamps.get(event.symbol)
        if last_time and event.timestamp < last_time:
            # Handle out of order gracefully (drop for paper sim, but log it)
            event.data_status = "STALE"
            return None
            
        self.last_event_timestamps[event.symbol] = event.timestamp
        return event
