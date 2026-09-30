from datetime import datetime
from src.engines.paper.event_bus import MarketEventBus, MarketEvent
from src.engines.paper.order_manager import PaperOrderManager, PaperOrder
from src.engines.paper.idempotency import IdempotencyGuard
import uuid

class PaperTradingScheduler:
    """
    Orchestrates real-time market data flows into Strategy V2 T+1 execution rules.
    """
    def __init__(self, strategy_version: str = "strategy_v2"):
        self.event_bus = MarketEventBus()
        self.order_manager = PaperOrderManager()
        self.idempotency = IdempotencyGuard()
        self.strategy_version = strategy_version

    def on_market_event_received(self, event: MarketEvent):
        """
        Callback fired on tick/bar from WebSocket/REST fallback.
        """
        valid_event = self.event_bus.process_event(event)
        if not valid_event:
            return # Stale or invalid
            
        # If this event constitutes a T Close for a daily candle
        if self._is_market_close(valid_event.timestamp):
            self._generate_signal(valid_event)
            
        # If this event constitutes a T+1 Open for a new daily candle
        if self._is_market_open(valid_event.timestamp):
            self._execute_pending_orders(valid_event)

    def _is_market_close(self, dt: datetime) -> bool:
        # Standard NSE close is 15:30 IST. 
        # For simulation, assume the flag is accurately passed by the event normalizer.
        return dt.hour == 15 and dt.minute >= 30

    def _is_market_open(self, dt: datetime) -> bool:
        # Standard NSE open is 09:15 IST.
        return dt.hour == 9 and dt.minute >= 15 and dt.minute < 30

    def _generate_signal(self, event: MarketEvent):
        # 1. Check idempotency
        key = self.idempotency.generate_key(self.strategy_version, event.symbol, str(event.timestamp), "T_CLOSE")
        if not self.idempotency.check_and_mark(key):
            return
            
        # 2. Invoke production SignalEngine (Mocked here since it's an architecture file)
        # 3. Create Pending Order if Buy
        order = PaperOrder(
            paper_order_id=str(uuid.uuid4()),
            symbol=event.symbol,
            side="BUY",
            quantity=10, # Mocked RiskEngine sizing
            requested_price=event.close,
            created_at=datetime.utcnow(),
            strategy_version=self.strategy_version,
            signal_id=key
        )
        self.order_manager.submit_order(order)
        
    def _execute_pending_orders(self, event: MarketEvent):
        simulator = __import__('src.engines.paper.order_manager', fromlist=['PaperExecutionSimulator']).PaperExecutionSimulator()
        
        # Iterate over pending orders for this symbol
        to_execute = [o for o in self.order_manager.pending_orders if o.symbol == event.symbol and o.status == "PENDING"]
        
        for order in to_execute:
            filled = simulator.execute_market_on_open(order, event.open)
            self.order_manager.filled_orders.append(filled)
            self.order_manager.pending_orders.remove(order)
