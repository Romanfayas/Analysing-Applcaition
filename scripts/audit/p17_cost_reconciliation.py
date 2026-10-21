"""Recompute the P1.6 cost examples from unrounded line items (V1 formulas, reproduced verbatim) and the P1.7 V2 ledger.
Run inside the quant container:  docker exec halal_equity_quant python /app/../scripts/... (or copy) """
from decimal import Decimal as D, ROUND_HALF_UP
import sys, json
sys.path.insert(0, "/app")
from src.engines.research.cost_model import CostEngine

r = lambda x: x.quantize(D("0.01"), rounding=ROUND_HALF_UP)

def v1(qty, pb, ps):
    """P1.6 formulas: STT sell only, NSE 0.00345%, DP 15.93 fixed, slippage on reference turnover, per-leg float rounding."""
    bt, st = D(qty) * D(pb), D(qty) * D(ps)
    lines = {  # name: (buy_unrounded, sell_unrounded)
        "STT": (D(0), st * D("0.001")),
        "NSE": (bt * D("0.0000345"), st * D("0.0000345")),
        "SEBI": (bt * D("0.000001"), st * D("0.000001")),
        "Stamp": (bt * D("0.00015"), D(0)),
        "Brokerage": (D(0), D(0)),
    }
    gst_b = (lines["NSE"][0] + lines["SEBI"][0]) * D("0.18"); gst_s = (lines["NSE"][1] + lines["SEBI"][1]) * D("0.18")
    lines["GST"] = (gst_b, gst_s)
    lines["DP"] = (D(0), D("15.93"))
    lines["Slippage"] = (bt * D("0.0005"), st * D("0.0005"))
    return lines

for name, qty, pb, ps in [("Rs10,000 TCS", 10, 1000, 1080), ("Rs5,00,000 RELIANCE", 200, 2500, 2600)]:
    L = v1(qty, pb, ps)
    print(f"\n=== P1.6 V1 formulas, {name}")
    print(f"{'item':10} {'buy_unr':>12} {'sell_unr':>12} {'buy_rnd':>9} {'sell_rnd':>9} {'tot_rnd(legs)':>13} {'round(sum_unr)':>14}")
    tb = ts = tu = trt = D(0)
    for k, (b, s) in L.items():
        print(f"{k:10} {b:12.6f} {s:12.6f} {r(b):9} {r(s):9} {r(b)+r(s):13} {r(b+s):14}")
        tb += r(b); ts += r(s); tu += b + s; trt += r(b + s)
    print(f"{'TOTAL':10} {'':12} {'':12} {tb:9} {ts:9} {tb+ts:13} {trt:14}   unrounded total={tu:.6f}")
    print(f"  => buy-leg shown {tb} + sell-leg shown {ts} = {tb+ts};  sum of per-line rounded totals = {trt};  exact unrounded = {r(tu)}")

print("\n=== P1.7 V2 ledgers")
eng = CostEngine()
for name, qty, pb, ps in [("Rs10,000 TCS", 10, 1000, 1080), ("Rs5,00,000 RELIANCE", 200, 2500, 2600)]:
    c = eng.calculate_trade(name, pb, ps, quantity=qty)
    print(f"\n--- {name}: qty={qty} ref {pb}->{ps}; fill {c.fill_buy_price} -> {c.fill_sell_price}")
    for l in c.ledger:
        print(f"{l.category:10} {l.leg:5} {l.item:30} base={l.base_inr[:14]:>14} unr={l.unrounded_inr[:12]:>12} rnd={l.rounded_inr:9.2f}")
    print(f"statutory={c.statutory_total} broker={c.broker_total} modeled_slippage={c.modeled_slippage_total} total={c.total_costs} (unrounded {c.total_costs_unrounded[:12]}, delta {c.rounding_delta:+.4f})")
    print(f"gross_ref={c.gross_pnl_reference} net={c.net_pnl} net%={c.net_return_pct:.4f} cost_bps={c.effective_cost_bps:.2f} reconciled={c.is_reconciled} disc={c.reconciliation_discrepancy}")
