"""Generates backend/internal/services/plans/testdata/parity.json from the Python simulator the learner uses. Run in the quant container:
   docker exec -i -e PYTHONPATH=/app halal_equity_quant python - > backend/internal/services/plans/testdata/parity.json < gen_parity.py"""
import json, numpy as np
from src.engines.research import breakout_evidence as be
rng = np.random.default_rng(20261110)
cases = []
dates = lambda n: [str(np.datetime64("2026-01-02") + i) for i in range(n)]
for ci in range(400):
    n = 70
    px = 100.0
    o, h, l, c = [], [], [], []
    gap_bias = rng.choice([0.0, 0.01, -0.01])
    for i in range(n):
        gap = rng.normal(gap_bias if i % 7 == 0 else 0, 0.008)
        op = px * (1 + gap)
        cl = op * (1 + rng.normal(0.001, 0.014))
        hi = max(op, cl) * (1 + abs(rng.normal(0, 0.006)))
        lo = min(op, cl) * (1 - abs(rng.normal(0, 0.006)))
        o.append(round(op, 2)); h.append(round(hi, 2)); l.append(round(lo, 2)); c.append(round(cl, 2)); px = cl
    t = 1  # signal bar; entry bar is index 2
    sc = c[t]
    atr = round(float(np.mean([h[i] - l[i] for i in range(t - 1, t + 1)])) + 0.5, 2)
    stop = round(sc * rng.uniform(0.93, 0.985), 2)
    eh = round(sc * rng.uniform(1.0, 1.01), 2)
    t1 = round(sc * rng.uniform(1.02, 1.06), 2)
    t2 = round(t1 * rng.uniform(1.02, 1.05), 2)
    sig = {"t": t, "date": dates(n)[t], "stop": stop, "t1": t1, "t2": t2, "entry_high": eh, "entry_low": sc, "atr": atr, "close": sc}
    d = {"o": np.array(o), "h": np.array(h), "l": np.array(l), "c": np.array(c), "dates": np.array(dates(n)), "signals": [sig]}
    for pid, plan in [(p["id"], p) for p in be.PLANS]:
        for eid, ext in (("ANY_OPEN", None), ("WITHIN_1PCT", 1.0)):
            rows = be._simulate(d, plan, ext, lambda x: True, lambda e, x, dd: 0.0)
            exp = None if not rows else {"entry": rows[0]["entry"], "exit": rows[0]["exit"], "why": rows[0]["why"], "bars": rows[0]["bars"]}
            cases.append({"plan": pid, "entry_filter": eid, "signal": {"stop": stop, "t1": t1, "t2": t2, "entry_high": eh, "atr": atr},
                          "bars": [{"date": dates(n)[i], "open": o[i], "high": h[i], "low": l[i], "close": c[i]} for i in range(t + 1, n)], "expected": exp})
print(json.dumps(cases))
