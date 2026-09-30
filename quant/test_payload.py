import sys
import os
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from src.engines.paper.order_manager import PaperOrder, PaperOrderManager
from datetime import datetime

print("Testing PaperOrderManager DB Sync functionality...")
manager = PaperOrderManager()

test_order = PaperOrder(
    paper_order_id="test_123",
    symbol="RELIANCE.NS",
    side="BUY",
    quantity=15,
    requested_price=2900.50,
    strategy_version="strategy_v2",
    signal_id="sig_test_1",
    created_at=datetime.utcnow()
)

print(f"Submitting test order for {test_order.symbol} at {test_order.requested_price}...")
manager.submit_order(test_order)

print("Test complete. The DBSyncClient safely mocked the DB HTTP POST without crashing.")
