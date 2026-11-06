#!/usr/bin/env python3
"""
End-to-End Delivery Verification Script for HalalEquity Sprint.
Tests all operational endpoints, Shariah compliance gates, paper trading ledger,
risk management cockpit, data quality metrics, and Strategy V2 research gate.
"""

import sys
import json
import urllib.request
import urllib.error

from _auth import sign_in

BASE_BACKEND = "http://localhost:8081"
BASE_QUANT = "http://localhost:8000"
BASE_PROXY = "http://localhost:3000/api/proxy"

SESSION = sign_in()
results = []

def record(test_name: str, passed: bool, details: str):
    results.append({
        "test": test_name,
        "passed": passed,
        "details": details
    })
    status = "PASS" if passed else "FAIL"
    print(f"[{status}] {test_name}: {details}")

def make_request(url: str, method: str = "GET", payload: dict = None, headers: dict = None):
    if headers is None:
        headers = {}
    if url.startswith(BASE_PROXY):
        headers.setdefault("Cookie", f"he_session={SESSION}")  # the proxy authenticates from the session cookie
    elif "Authorization" not in headers:
        headers["Authorization"] = f"Bearer {SESSION}"  # the session cookie value is the backend JWT
    
    data = None
    if payload is not None:
        data = json.dumps(payload).encode('utf-8')
        headers["Content-Type"] = "application/json"
    
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            content = resp.read().decode('utf-8')
            return resp.status, json.loads(content) if content else {}
    except urllib.error.HTTPError as e:
        err_content = e.read().decode('utf-8')
        try:
            parsed = json.loads(err_content)
        except Exception:
            parsed = {"raw": err_content}
        return e.code, parsed
    except Exception as e:
        return 0, {"error": str(e)}

def unwrap(res: dict):
    if isinstance(res, dict) and "data" in res and res.get("success") is True:
        return res["data"]
    return res

def run_tests():
    print("======================================================================")
    print("HALALEQUITY END-TO-END DELIVERY VERIFICATION")
    print("======================================================================")

    # 1. System Health
    code, data = make_request(f"{BASE_BACKEND}/health")
    health_data = unwrap(data)
    record("Backend Health", code == 200 and health_data.get("status") == "healthy", f"Code {code}, status={health_data.get('status')}")

    code, data = make_request(f"{BASE_QUANT}/health")
    record("Quant Engine Health", code == 200 and data.get("status") == "healthy", f"Code {code}, status={data.get('status')}")

    code, data = make_request(f"{BASE_BACKEND}/api/v1/market-data/health")
    record("Market Data Health", code == 200, f"Code {code}")

    # 2. Stock Universe & Detail (via Direct API & Proxy)
    code, data = make_request(f"{BASE_PROXY}/stocks")
    stocks = unwrap(data)
    record("Stock Universe List (via Proxy)", code == 200 and len(stocks) >= 40, f"Found {len(stocks)} active securities")

    code, data = make_request(f"{BASE_PROXY}/stocks/market/overview")
    movers = unwrap(data)
    has_movers = "sectors" in movers and "total_active_equities" in movers
    record("Market Overview Route", code == 200 and has_movers, f"TotalEquities={movers.get('total_active_equities')}, Sectors={len(movers.get('sectors', []))}")

    code, tcs_resp = make_request(f"{BASE_PROXY}/stocks/TCS")
    tcs_data = unwrap(tcs_resp)
    tcs_ok = code == 200 and tcs_data.get("symbol") == "TCS" and tcs_data.get("current_price", 0) > 0
    tcs_shariah = tcs_data.get("shariah", {}).get("status")
    record("TCS Stock Detail", tcs_ok and tcs_shariah in ("PASS", "FAIL", "DATA_INSUFFICIENT"), f"Price=₹{tcs_data.get('current_price')}, Shariah={tcs_shariah}")

    # 3. Shariah Screening
    code, s_resp = make_request(f"{BASE_PROXY}/shariah/screenings")
    screenings = unwrap(s_resp)
    record("Shariah Screening Universe", code == 200 and len(screenings) > 0, f"Retrieved {len(screenings)} screenings")

    # 4. IPO Pipeline
    code, ipo_resp = make_request(f"{BASE_PROXY}/ipos?status=all")
    ipos = unwrap(ipo_resp)
    record("IPO Pipeline Ingestion", code == 200 and len(ipos) > 0, f"Retrieved {len(ipos)} IPO records")

    # 5. Quant Breakout Signals
    code, sig_resp = make_request(f"{BASE_PROXY}/signals/breakouts")
    signals = unwrap(sig_resp)
    record("Quant Breakout Signals", code == 200, f"Retrieved {len(signals) if isinstance(signals, list) else 0} signals")

    # 6. Portfolio & Paper Trading Ledger
    code, p_resp = make_request(f"{BASE_PROXY}/portfolio")
    p_data = unwrap(p_resp)
    portfolio_record = p_data.get("portfolio", {})
    has_cash = "cash_balance" in portfolio_record and "total_equity" in portfolio_record
    record("Portfolio Mark-to-Market", code == 200 and has_cash, f"Cash=₹{portfolio_record.get('cash_balance')}, Equity=₹{portfolio_record.get('total_equity')}")

    # Test Shariah Gate: Block BUY on non-certified stock (TCS is DATA_INSUFFICIENT)
    buy_payload = {
        "symbol": "TCS",
        "trade_type": "BUY",
        "shares": 5,
    }
    code, buy_res = make_request(f"{BASE_PROXY}/portfolio/trade", method="POST", payload=buy_payload)
    # P1.9.24: paper BUYs are no longer blocked by Shariah status (owner decision); the status is recorded instead.
    trade = unwrap(buy_res).get("trade", {}) if code == 201 else {}
    ok = (code == 201 and trade.get("shariah_status_at_trade") in ("PASS", "FAIL", "DATA_INSUFFICIENT")) or \
         (code == 400 and "Shariah" not in str(buy_res) and "price" in str(buy_res))
    record("Paper BUY not Shariah-blocked; status recorded on the trade", ok,
           f"Code={code}, ShariahAtTrade={trade.get('shariah_status_at_trade')}, Error={buy_res.get('error')}")

    # Test Short Selling Prohibition: Block SELL on unheld shares
    sell_payload = {  # more than the account can hold after the BUY above: a short sale
        "symbol": "TCS",
        "trade_type": "SELL",
        "shares": 1000,
    }
    code, sell_res = make_request(f"{BASE_PROXY}/portfolio/trade", method="POST", payload=sell_payload)
    is_blocked_short = (code == 400 or (code == 200 and sell_res.get("success") is False)) and "insufficient shares" in str(sell_res)
    record("Short Selling Restriction Enforcement", is_blocked_short, f"Result={sell_res.get('error')}")

    # Paper trading session
    code, sess_resp = make_request(f"{BASE_PROXY}/paper-trading/session")
    sess_data = unwrap(sess_resp)
    record("Paper Trading Session Safety State", code == 200 and sess_data.get("paper_trading_enabled") is True and sess_data.get("live_order_execution") is False,
           f"PaperTrading={sess_data.get('paper_trading_enabled')}, LiveExecution={sess_data.get('live_order_execution')}")

    # 7. Risk Management Cockpit
    calc_payload = {
        "capital": 1000000,
        "risk_percentage": 1.5,
        "entry_price": 2500.0,
        "stop_loss": 2400.0,
        "target_price": 2800.0
    }
    code, risk_resp = make_request(f"{BASE_PROXY}/risk/calculate", method="POST", payload=calc_payload)
    risk_res = unwrap(risk_resp)
    calc_ok = code == 200 and risk_res.get("suggested_shares", 0) > 0 and risk_res.get("risk_reward_ratio", 0) > 0
    record("Risk Position Sizing Calculator", calc_ok, f"SuggestedShares={risk_res.get('suggested_shares')}, R:R={risk_res.get('risk_reward_ratio')}, RiskPerShare=₹{risk_res.get('risk_per_share')}")

    code, metrics_resp = make_request(f"{BASE_PROXY}/risk/metrics")
    risk_metrics = unwrap(metrics_resp)
    record("Portfolio Risk Metrics", code == 200 and risk_metrics.get("execution_mode") == "PAPER_TRADING",
           f"ExecutionMode={risk_metrics.get('execution_mode')}, MaxHeat={risk_metrics.get('max_portfolio_heat_pct')}%, MaxRiskTrade={risk_metrics.get('max_risk_per_trade_pct')}%")

    # 8. Data Quality & Quarantine
    code, dq_resp = make_request(f"{BASE_PROXY}/data-quality")
    dq_data = unwrap(dq_resp)
    record("Data Quality Dashboard", code == 200 and len(dq_data) >= 4, f"Datasets={len(dq_data)}")

    code, dq_issues_resp = make_request(f"{BASE_PROXY}/data-quality/issues")
    issues_list = unwrap(dq_issues_resp)
    record("Data Quality Quarantine Issues List", code == 200 and len(issues_list) > 0, f"Retrieved {len(issues_list)} quarantine records")

    # 9. Strategy V2 Research Gate Check
    bt_payload = {
        "strategy": "breakout_v2",
        "symbol": "TCS",
        "start_date": "2024-01-01",
        "end_date": "2026-10-15"
    }
    code, bt_resp = make_request(f"{BASE_PROXY}/backtesting/run", method="POST", payload=bt_payload)
    bt_res = unwrap(bt_resp)
    is_blocked = code == 200 and bt_res.get("status") == "RESEARCH_BLOCKED"
    # Strategy parameters and the hash are attested by the quant engine at runtime, not hardcoded in the API.
    _, gov = make_request(f"{BASE_QUANT}/api/v1/governance/strategy")
    resolved = gov.get("resolved", {})
    gate = bt_res.get("gate", {})
    params_match = (
        resolved.get("BUY_THRESHOLD") == 80.0 and
        resolved.get("SELL_THRESHOLD") == 45.0 and
        resolved.get("ATR_MULTIPLIER") == 2.0 and
        gate.get("is_strategy_hash_valid") is True and
        gate.get("canonical_strategy_hash") == "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
    )
    record("Strategy V2 Research Gate & Parameter Verification", is_blocked and params_match,
           f"Status={bt_res.get('status')}, QuarantinedBars={bt_res.get('quarantined_bars')}, SHA256={str(gate.get('canonical_strategy_hash'))[:16]}...")

    print("======================================================================")
    total = len(results)
    passed = sum(1 for r in results if r["passed"])
    print(f"VERIFICATION SUMMARY: {passed}/{total} checks passed.")
    print("======================================================================")
    return passed == total

if __name__ == "__main__":
    success = run_tests()
    sys.exit(0 if success else 1)
