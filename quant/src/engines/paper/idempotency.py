import hashlib

class IdempotencyGuard:
    def __init__(self):
        self.processed_signatures = set()
        
    def generate_key(self, strategy_version: str, symbol: str, signal_timestamp: str, signal_type: str) -> str:
        payload = f"{strategy_version}_{symbol}_{signal_timestamp}_{signal_type}"
        return hashlib.sha256(payload.encode('utf-8')).hexdigest()
        
    def check_and_mark(self, key: str) -> bool:
        if key in self.processed_signatures:
            return False
        self.processed_signatures.add(key)
        return True
