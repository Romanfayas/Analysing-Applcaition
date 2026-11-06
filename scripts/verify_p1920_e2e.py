#!/usr/bin/env python3
"""P1.9.20 end-to-end verification against the running stack, through the frontend (:3000).

Exercises the real session flow: sign-up/login via /api/auth/*, then API calls through the
authenticated proxy. Creates one throwaway account (e2e-<random>@example.test); nothing else is
written except that account's own alert/settings rows.

Usage: python3 scripts/verify_p1920_e2e.py [base_url]
"""
import http.cookiejar
import json
import secrets
import sys
import urllib.error
import urllib.request

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://localhost:3000").rstrip("/")
results = []


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **k):
        return None


def client():
    jar = http.cookiejar.CookieJar()
    return urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), NoRedirect), jar


def call(op, method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method, headers={"Content-Type": "application/json"})
    try:
        r = op.open(req, timeout=60)
        raw, code, hdrs = r.read(), r.status, r.headers
    except urllib.error.HTTPError as e:
        raw, code, hdrs = e.read(), e.code, e.headers
    try:
        return code, json.loads(raw), hdrs
    except ValueError:
        return code, None, hdrs


def check(name, ok, detail=""):
    results.append(bool(ok))
    print(f"[{'PASS' if ok else 'FAIL'}] {name}" + (f"  ({detail})" if detail and not ok else ""))


def data(body):
    return (body or {}).get("data")


anon, _ = client()
code, _, h = call(anon, "GET", "/")
check("unauthenticated page request redirects to /login", code in (307, 308) and "/login" in (h.get("Location") or ""), f"{code} {h.get('Location')}")
code, _, _ = call(anon, "GET", "/api/proxy/stocks")
check("unauthenticated API call is rejected", code == 401, code)
code, _, _ = call(anon, "GET", "/api/proxy/stocks", None)
check("no backend rewrite bypasses the proxy (/api/v1 is gone)", call(anon, "GET", "/api/v1/stocks")[0] == 404)

email, pw = f"e2e-{secrets.token_hex(4)}@example.test", "correct horse battery"
user, _ = client()
code, body, h = call(user, "POST", "/api/auth/register", {"email": email, "password": pw, "name": "E2E"})
check("register creates an account", code == 201, f"{code} {body}")
cookie = h.get("Set-Cookie") or ""
check("session cookie is httpOnly and the token is not in the body", "HttpOnly" in cookie and "token" not in json.dumps(body), cookie[:80])
code, body, _ = call(user, "POST", "/api/auth/register", {"email": email, "password": pw, "name": "E2E"})
check("duplicate registration is a 409", code == 409, code)
code, _, _ = call(anon, "POST", "/api/auth/login", {"email": email, "password": "wrong password!!"})
check("wrong password is a 401", code == 401, code)
code, body, _ = call(user, "GET", "/api/auth/me")
check("/api/auth/me returns the signed-in user", code == 200 and body["user"]["email"] == email, body)

P = "/api/proxy"
code, body, _ = call(user, "GET", f"{P}/stocks")
stocks = data(body) or []
check("stocks list loads (at least the 51 canonical instruments)", code == 200 and len(stocks) >= 51, f"{code} {len(stocks)}")
check("every instrument has exactly one Shariah verdict", all(s["shariah"] in ("PASS", "FAIL", "DATA_INSUFFICIENT", "UNKNOWN") for s in stocks))
check("banks and insurers are never PASS", all(s["shariah"] != "PASS" for s in stocks if s["symbol"] in ("HDFCBANK", "ICICIBANK", "SBIN", "SBILIFE", "HDFCLIFE")))
fresh = {s["freshness"] for s in stocks}
print(f"      freshness across universe: {sorted(fresh)}")

code, body, _ = call(user, "GET", f"{P}/stocks/search?q=reliance")
check("search finds RELIANCE", code == 200 and any(r["symbol"] == "RELIANCE" for r in data(body)), body)
code, body, _ = call(user, "GET", f"{P}/stocks/RELIANCE/candles?limit=300")
d = data(body) or {}
check("candles carry eligibility flags", code == 200 and d.get("count") == 300 and "research_eligible_count" in d, d.get("count"))
print(f"      RELIANCE candles: eligible={d.get('research_eligible_count')} quarantined={d.get('quarantined_count')}")
code, body, _ = call(user, "GET", f"{P}/stocks/RELIANCE/technicals")
d = data(body) or {}
check("technicals computed from eligible bars only", code == 200 and d.get("basis", {}).get("research_eligible_only") is True, d.get("status"))
print(f"      technicals: status={d.get('status')} bars={d.get('basis', {}).get('eligible_bars')} indicators={[i['indicator'] for i in d.get('indicators', [])]}")
code, body, _ = call(user, "GET", f"{P}/stocks/RELIANCE/risk")
check("risk reports insufficient history rather than inventing metrics", code == 200 and data(body)["status"] in ("AVAILABLE", "DATA_INSUFFICIENT"), body)
code, body, _ = call(user, "GET", f"{P}/stocks/RELIANCE/analytics")
check("analytics reports readiness", code == 200 and "readiness" in data(body), body)
code, body, _ = call(user, "GET", f"{P}/stocks/NOPE/technicals")
check("unknown symbol is a 404", code == 404, code)

code, body, _ = call(user, "GET", f"{P}/stocks/market/overview")
idx = (data(body) or {}).get("indices", [])
check("market overview shows an index value only as an official end-of-day close with its date", code == 200 and all(i["value"] is None or (i.get("status") == "END_OF_DAY" and i.get("as_of")) for i in idx), idx)

code, body, _ = call(user, "GET", f"{P}/market-data/health")
mh = data(body) or {}
print(f"      market-data health: freshness={mh.get('freshness_status')} latest={mh.get('latest_market_timestamp')}")

code, body, _ = call(user, "GET", f"{P}/ipos?status=all")
ipos = data(body) or []
check("IPO rows all have real names", code == 200 and ipos and all(i["name"].strip() for i in ipos), [i["name"] for i in ipos])

code, body, _ = call(user, "GET", f"{P}/backtesting/gate")
g = data(body) or {}
check("research gate is measured and closed", code == 200 and g.get("status") == "RESEARCH_BLOCKED" and g.get("quarantined_bars_count", 0) > 0, g.get("reason"))
check("quarantined history is preserved (23,132 bars)", g.get("quarantined_bars_count") == 23132, g.get("quarantined_bars_count"))
check("strategy attestation is live", g.get("is_strategy_hash_valid") is True, g.get("is_strategy_hash_valid"))
code, body, _ = call(user, "POST", f"{P}/backtesting/run", {})
check("backtest run is blocked by the gate", code == 200 and data(body).get("status") == "RESEARCH_BLOCKED", body)
code, _, _ = call(user, "PUT", f"{P}/signals/weights", {})
check("frozen signal weights cannot be edited", code == 409, code)
code, body, _ = call(user, "GET", f"{P}/signals/weights")
check("weights come from the database", code == 200 and abs(sum(data(body).values()) - 1.0) < 1e-9, body)

# portfolio isolation and the server-priced ledger
code, body, _ = call(user, "GET", f"{P}/portfolio")
pf = data(body)["portfolio"]
check("new account gets its own 10,00,000 paper portfolio", code == 200 and pf["cash_balance"] == 1000000 and pf["id"] != 1, pf)
other, _ = client()
call(other, "POST", "/api/auth/register", {"email": f"e2e-{secrets.token_hex(4)}@example.test", "password": pw, "name": "Other"})
code, body, _ = call(other, "GET", f"{P}/portfolio")
check("a second account has a different portfolio", data(body)["portfolio"]["id"] != pf["id"])
code, _, _ = call(other, "GET", f"{P}/paper-trading/portfolios/{pf['id']}")
check("another account's portfolio id is a 404", code == 404, code)
code, body, _ = call(user, "POST", f"{P}/portfolio/trade", {"symbol": "TCS", "trade_type": "BUY", "shares": 1, "price": 1})
check("client-supplied fill price is refused", code == 400 and "price is set by the server" in json.dumps(body), body)
code, body, _ = call(user, "POST", f"{P}/portfolio/trade", {"symbol": "TCS", "trade_type": "BUY", "shares": 1})
if code == 201:
    st = (data(body) or {}).get("trade", {}).get("shariah_status_at_trade")
    check("paper BUY executes and records the instrument's Shariah status (P1.9.24)", st in ("PASS", "FAIL", "DATA_INSUFFICIENT"), st)
else:  # only acceptable refusal: no current price outside market hours
    check("paper BUY is not refused for Shariah reasons", "Shariah" not in json.dumps(body) and "price" in json.dumps(body), body)
# Selling more than is held is a short sale (the account may hold 1 share from the BUY above).
code, body, _ = call(user, "POST", f"{P}/portfolio/trade", {"symbol": "TCS", "trade_type": "SELL", "shares": 1000})
check("short selling is refused", code == 400 and "Short selling" in json.dumps(body), body)

# alerts + settings
code, body, _ = call(user, "POST", f"{P}/alerts", {"symbol": "TCS", "alert_type": "PRICE_ABOVE", "threshold": 1.0})
alert = data(body) or {}
check("alert is created", code == 201 and alert.get("id"), body)
code, body, _ = call(user, "POST", f"{P}/alerts", {"symbol": "TCS", "alert_type": "RSI_OVERBOUGHT", "threshold": 70})
check("unsupported alert types are rejected, not silently stored", code == 422, code)
code, body, _ = call(other, "DELETE", f"{P}/alerts/{alert.get('id')}")
check("another account cannot delete the alert", code == 404, code)
code, body, _ = call(user, "GET", f"{P}/alerts")
check("alert is listed for its owner", len(data(body)) == 1)
code, body, _ = call(user, "PUT", f"{P}/settings", {"default_benchmark": "SENSEX", "default_risk_pct": 2, "display_timezone": "UTC"})
check("settings save", code == 200, body)
code, body, _ = call(user, "GET", f"{P}/settings")
check("settings persist", data(body)["preferences"]["default_benchmark"] == "SENSEX", body)
code, _, _ = call(user, "PUT", f"{P}/settings", {"default_benchmark": "SENSEX", "default_risk_pct": 99, "display_timezone": "UTC"})
check("out-of-range settings are rejected", code == 422, code)
code, _, _ = call(user, "DELETE", f"{P}/alerts/{alert['id']}")
check("owner deletes the alert", code == 200)

# P1.9.21 prediction engine
code, body, _ = call(user, "GET", f"{P}/predictions/models")
models = (data(body) or {}).get("models", [])
check("prediction models exist for 5/15/60 minutes", code == 200 and sorted(m["horizon_minutes"] for m in models) == [5, 15, 60], code)
check("every model carries a walk-forward verdict", all(m["verdict"] in ("ACTIONABLE", "NO_DEMONSTRATED_EDGE", "DATA_INSUFFICIENT") for m in models))
code, body, _ = call(user, "GET", f"{P}/predictions/latest")
preds = (data(body) or {}).get("predictions", [])
check("latest forecasts load", code == 200, code)
check("BUY candidates only come from ACTIONABLE models",
      all(p["model_verdict"] == "ACTIONABLE" for p in preds if p["action"] == "BUY_CANDIDATE"))
check("FAIL-screened stocks are never BUY candidates", not any(p["action"] == "BUY_CANDIDATE" and p["shariah_status"] == "FAIL" for p in preds))
check("forecasts were issued before their target time", all(p["issued_at"] < p["target_time"] for p in preds))
code, _, _ = call(user, "GET", f"{P}/predictions/tracking")
check("forward tracking loads", code == 200, code)

# pages render for an authenticated session
for page in ["/", "/predictions", "/stocks", "/stocks/RELIANCE", "/signals", "/ipos", "/shariah", "/portfolio", "/paper-trading", "/risk", "/backtesting", "/data-quality", "/alerts", "/settings"]:
    code, _, _ = call(user, "GET", page)
    check(f"page {page} renders", code == 200, code)

code, _, _ = call(user, "POST", "/api/auth/logout")
code, _, _ = call(user, "GET", f"{P}/stocks")
check("logout ends the session", code == 401, code)

print(f"\n{sum(results)}/{len(results)} checks passed")
sys.exit(0 if all(results) else 1)
