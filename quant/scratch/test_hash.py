import json, hashlib

def try_hash(config):
    h1 = hashlib.sha256(json.dumps(config, sort_keys=True).encode()).hexdigest()
    h2 = hashlib.sha256(json.dumps(config, sort_keys=True, separators=(',', ':')).encode()).hexdigest()
    h3 = hashlib.sha256(json.dumps(config).encode()).hexdigest()
    print(f"{h1} - default")
    print(f"{h2} - no spaces")
    print(f"{h3} - no sort")

try_hash({"BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0, "ATR_MULTIPLIER": 2.0})
try_hash({"BUY_THRESHOLD": 80, "SELL_THRESHOLD": 45, "ATR_MULTIPLIER": 2})
try_hash({"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0})
try_hash({"BUY_THRESHOLD": "80.0", "SELL_THRESHOLD": "45.0", "ATR_MULTIPLIER": "2.0"})
