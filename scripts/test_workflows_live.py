#!/usr/bin/env python3
import urllib.request
import json

from _auth import sign_in

BASE_PROXY = "http://localhost:3000/api/proxy"
BASE_DIRECT = "http://localhost:8081/api/v1"
SESSION = sign_in()

def req(url, method="GET", payload=None, headers=None):
    if headers is None: 
        headers = {}
    if url.startswith(BASE_PROXY):
        headers.setdefault("Cookie", f"he_session={SESSION}")  # direct-backend calls stay anonymous unless a test adds a header
    data = json.dumps(payload).encode() if payload else None
    if payload: 
        headers["Content-Type"] = "application/json"
    r = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(r, timeout=10) as resp:
            content = resp.read().decode()
            return resp.status, json.loads(content) if content else {}
    except urllib.error.HTTPError as e:
        err_body = e.read().decode()
        try:
            return e.code, json.loads(err_body)
        except Exception:
            return e.code, {"raw": err_body}
    except Exception as e:
        return 0, {"error": str(e)}

print("=== 1. Stock Universe & Market Overview ===")
s, d = req(f"{BASE_PROXY}/stocks")
stocks = d.get("data", [])
print(f"Stocks List: Code {s}, Active Count={len(stocks)}")
s, d = req(f"{BASE_PROXY}/stocks/market/overview")
mkt = d.get("data", {})
print(f"Market Overview: Code {s}, ActiveEquities={mkt.get('total_active_equities')}, Sectors={len(mkt.get('sectors', []))}")

print("\n=== 2. Multiple Stock Detail Pages ===")
for sym in ["TCS", "INFY", "TATAMOTORS", "TMPV", "TMCV", "DIVISLAB"]:
    s, d = req(f"{BASE_PROXY}/stocks/{sym}")
    stk = d.get("data", {})
    shariah = stk.get("shariah", {}).get("status") if isinstance(stk.get("shariah"), dict) else "N/A"
    print(f"Stock {sym:10}: Code {s}, Price=₹{stk.get('current_price')}, Freshness={stk.get('freshness')}, Shariah={shariah}, Active={stk.get('is_active')}")

print("\n=== 3. Shariah Screenings Evidence ===")
s, d = req(f"{BASE_PROXY}/shariah/screenings")
screenings = d.get("data", [])
print(f"Shariah Screenings: Code {s}, Total Records={len(screenings)}")
if screenings:
    sample = screenings[0]
    print(f"Sample Screening ({sample.get('symbol')}): DebtRatio={sample.get('debt_ratio')}, Status={sample.get('status')}, Notes={sample.get('business_activity_notes')}")

print("\n=== 4. IPO Dashboard ===")
s, d = req(f"{BASE_PROXY}/ipos?status=all")
ipos = d.get("data", [])
print(f"IPOs: Code {s}, Total Records={len(ipos)}")
if ipos:
    sample_ipo = ipos[0]
    print(f"Sample IPO ({sample_ipo.get('symbol')}): Status={sample_ipo.get('status')}, PriceBand={sample_ipo.get('price_band')}, Shariah={sample_ipo.get('shariah_status')}")

print("\n=== 5. Position Sizing & Risk Calculation ===")
s, d = req(f"{BASE_PROXY}/risk/calculate", method="POST", payload={
    "capital": 1000000,
    "risk_percentage": 1.5,
    "entry_price": 2500,
    "stop_loss": 2400,
    "target_price": 2800
})
risk = d.get("data", {})
print(f"Risk Calculate: Code {s}, SuggestedShares={risk.get('suggested_shares')}, RiskPerShare=₹{risk.get('risk_per_share')}, R:R={risk.get('risk_reward_ratio')}, CapitalExposure={risk.get('capital_exposure_pct')}%")

print("\n=== 6. Paper BUY Gating (Fail-Closed Rejections) ===")
s, d = req(f"{BASE_PROXY}/portfolio/trade", method="POST", payload={"symbol": "TCS", "trade_type": "BUY", "shares": 5})
print(f"BUY TCS (DATA_INSUFFICIENT): Code {s}, Error={d.get('error')}")
s, d = req(f"{BASE_PROXY}/portfolio/trade", method="POST", payload={"symbol": "TATASTEEL", "trade_type": "BUY", "shares": 5})
print(f"BUY TATASTEEL (FAIL): Code {s}, Error={d.get('error')}")

print("\n=== 7. Paper SELL Gating (Short-Selling Restriction) ===")
s, d = req(f"{BASE_PROXY}/portfolio/trade", method="POST", payload={"symbol": "TCS", "trade_type": "SELL", "shares": 5})
print(f"SELL TCS (0 Owned Shares): Code {s}, Error={d.get('error')}")

print("\n=== 8. Portfolio Balances & Ledger State ===")
s, d = req(f"{BASE_PROXY}/portfolio")
port = d.get("data", {}).get("portfolio", {})
print(f"Portfolio: Code {s}, Cash=₹{port.get('cash_balance')}, Equity=₹{port.get('total_equity')}")

print("\n=== 9. Backtesting Research Gate ===")
s, d = req(f"{BASE_PROXY}/backtesting/run", method="POST", payload={"strategy": "breakout_v2", "symbol": "TCS", "start_date": "2024-01-01", "end_date": "2026-10-15"})
bt = d.get("data", {})
print(f"Backtesting Gate: Code {s}, Status={bt.get('status')}")
print(f"Reason: {bt.get('reason')}")
print(f"Quarantined Bars Invariant: {bt.get('quarantined_bars')}")
print(f"Frozen Strategy SHA256: {bt.get('gate', {}).get('canonical_strategy_hash')}")

print("\n=== 10. Data Quality Dashboard & Quarantine Counts ===")
s, d = req(f"{BASE_PROXY}/data-quality")
dq = d.get("data", [])
print(f"Data Quality: Code {s}, Datasets Count={len(dq)}")
for ds in dq:
    print(f" - {ds.get('dataset')}: Status={ds.get('status')}, Valid={ds.get('valid_records')}, Invalid={ds.get('invalid_records')}")
s, d = req(f"{BASE_PROXY}/data-quality/issues")
issues = d.get("data", [])
print(f"Quarantine Issues API: Code {s}, Issues Count={len(issues)}")

print("\n=== 11. Security & Negative Input Tests ===")
s, d = req(f"{BASE_DIRECT}/stocks")
print(f"Direct Backend Without JWT: Code {s}, Error={d.get('error') or d.get('raw')}")
s, d = req(f"{BASE_PROXY}/stocks/NONEXISTENT")
print(f"Nonexistent Stock: Code {s}, Error={d.get('error') or d.get('raw')}")
s, d = req(f"{BASE_PROXY}/risk/calculate", method="POST", payload={"capital": 1000000, "risk_percentage": 1.5, "entry_price": 2500, "stop_loss": 2600})
print(f"Stop Loss >= Entry: Code {s}, Error={d.get('error') or d.get('raw')}")
s, d = req(f"{BASE_PROXY}/portfolio/trade", method="POST", payload={"symbol": "TCS", "trade_type": "SHORT_SELL", "shares": 10})
print(f"Invalid Trade Type: Code {s}, Error={d.get('error') or d.get('raw')}")
s, d = req(f"{BASE_PROXY}/portfolio/trade", method="POST", payload={"symbol": "TCS", "trade_type": "BUY", "shares": -10})
print(f"Negative Shares: Code {s}, Error={d.get('error') or d.get('raw')}")
