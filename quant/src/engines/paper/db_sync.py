import json
import requests
from src.engines.paper.order_manager import PaperOrder

class DBSyncClient:
    def __init__(self, api_url: str = "http://localhost:8080/internal/paper"):
        self.api_url = api_url

    def sync_pending_order(self, order: PaperOrder) -> int:
        payload = {
            "portfolio_id": 1,
            "symbol_id": hash(order.symbol) % 10000, # Simulated mock ID logic for constraints
            "side": order.side,
            "quantity": order.quantity,
            "order_price": order.requested_price
        }
        try:
            # Simulate posting to the Go backend
            print(f"Syncing PENDING Order: {payload}")
            # response = requests.post(f"{self.api_url}/order", json=payload)
            # return response.json().get('id')
            return 999 # Simulated ID
        except Exception as e:
            print(f"DB Sync failed: {e}")
            return -1

    def sync_filled_order(self, order_id: int, fill_price: float, slippage: float):
        payload = {
            "order_id": order_id,
            "fill_price": fill_price,
            "simulated_slippage": slippage
        }
        try:
            print(f"Syncing FILLED Order: {payload}")
            # requests.post(f"{self.api_url}/fill", json=payload)
        except Exception as e:
            print(f"DB Sync failed: {e}")
