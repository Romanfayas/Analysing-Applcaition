from datetime import datetime
from typing import Dict, Any, List
from pydantic import BaseModel
from src.engines.paper.db_sync import DBSyncClient

class PaperOrder(BaseModel):
    paper_order_id: str
    symbol: str
    side: str # "BUY" or "SELL"
    quantity: int
    requested_price: float
    simulated_fill_price: float = 0.0
    order_type: str = "MARKET_ON_OPEN"
    created_at: datetime
    filled_at: datetime = None
    status: str = "PENDING"
    slippage: float = 0.0
    transaction_cost: float = 0.0
    strategy_version: str
    signal_id: str

class PaperExecutionSimulator:
    def __init__(self, slippage_model: str = "baseline", cost_model: str = "india_equity_delivery_v1"):
        self.slippage_bps = 10 if slippage_model == "baseline" else 0
        self.cost_model = cost_model
        
    def execute_market_on_open(self, order: PaperOrder, open_price: float) -> PaperOrder:
        """
        Simulates T+1 Open execution with deterministic slippage.
        """
        # Calculate slippage
        slippage_amount = open_price * (self.slippage_bps / 10000.0)
        
        if order.side == "BUY":
            fill_price = open_price + slippage_amount
        else:
            fill_price = open_price - slippage_amount
            
        order.simulated_fill_price = fill_price
        order.slippage = slippage_amount
        
        # Calculate Transaction Costs (simplified india_equity_delivery_v1 for simulation)
        gross_value = order.quantity * fill_price
        stt = gross_value * 0.001
        
        order.transaction_cost = stt
        order.status = "FILLED"
        order.filled_at = datetime.utcnow()
        return order
        
class PaperOrderManager:
    def __init__(self):
        self.pending_orders: List[PaperOrder] = []
        self.filled_orders: List[PaperOrder] = []
        self.db_sync = DBSyncClient()
        
    def submit_order(self, order: PaperOrder):
        # 1. Store in Memory
        self.pending_orders.append(order)
        # 2. Persist to Postgres via API
        db_id = self.db_sync.sync_pending_order(order)
        # Assume we attach db_id for future reference
        
    def mark_filled(self, order: PaperOrder):
        self.db_sync.sync_filled_order(
            order_id=999, # Hardcoded simulated ID for constraint handling
            fill_price=order.simulated_fill_price,
            slippage=order.slippage
        )
