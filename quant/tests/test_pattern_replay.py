"""P1.9.31: paper-test of closest / upcoming / confirmed readings on a stock's own past, on hand-built bars."""
from datetime import date, timedelta

import numpy as np
import pytest

from src.engines.research import pattern_replay as pr
from src.engines.research.breakout_watch import CANDIDATE, CONFIRMED, SymbolSeries
from src.engines.technical.breakout import BreakoutState

WATCH = BreakoutState.WATCHLIST


def cost(entry, px, days):          # flat 0.5% round trip keeps the arithmetic checkable
    return (px / entry - 1) * 100 - 0.5


def series(n=80, stages=None, o=None, h=None, l=None, c=None, stop=95.0, t1=107.5, res=100.0, dist=1.0):
    dates = np.array([str(date(2026, 1, 1) + timedelta(days=i)) for i in range(n)])
    cl = np.array(c if c is not None else [98.0] * n, float)
    op = np.array(o if o is not None else cl, float)
    hi = np.array(h if h is not None else cl + 0.5, float)
    lo = np.array(l if l is not None else cl - 0.5, float)
    st, detail = [None] * n, {}
    for t, s in (stages or {}).items():
        st[t] = s
        detail[t] = {"stage": s, "stop": stop, "t1": t1, "t2": 120.0, "entry_low": res, "entry_high": res + 1, "resistance": res, "atr": 2.0, "rvol": 1.5,
                     "score": 60.0, "breakdown": {}, "reasons": [], "invalidation": [], "dist": dist}
    return SymbolSeries("TEST", 7, dates, op, hi, lo, cl, np.full(n, 1000), st, detail)


def arr(n, base=98.0, **over):
    a = [base] * n
    for k, v in over.items():
        a[int(k[1:])] = v
    return a


def test_closest_signal_needs_the_watchlist_stage_within_six_percent():
    s = series(stages={45: WATCH, 46: WATCH, 47: CANDIDATE}, dist=2.0)
    s.detail[46]["dist"] = 7.0
    assert pr.is_signal(s, 45, "CLOSEST") and not pr.is_signal(s, 46, "CLOSEST") and not pr.is_signal(s, 47, "CLOSEST")
    assert pr.is_signal(s, 47, "UPCOMING") and not pr.is_signal(s, 45, "UPCOMING")


def test_buy_stop_fills_at_resistance_and_exits_at_target():
    h = arr(80, base=98.5, t47=100.4, t50=111.0)
    s = series(stages={45: WATCH}, h=h)
    row, end = pr.simulate_signal(s, 45, "CLOSEST", cost)
    assert row["outcome"] == "HIT_TARGET" and row["entry_date"] == s.dates[47] and row["entry_price"] == pytest.approx(100.0)
    assert row["exit_date"] == s.dates[50] and row["exit_price"] == pytest.approx(107.5) and row["net_pct"] == pytest.approx(7.0) and end == 50


def test_gap_above_resistance_fills_at_the_open_not_at_resistance():
    o = arr(80, base=98.0, t46=102.0)
    h = arr(80, base=98.5, t46=103.0)
    s = series(stages={45: WATCH}, o=o, h=h)
    row, _ = pr.simulate_signal(s, 45, "CLOSEST", cost)
    assert row["entry_price"] == pytest.approx(102.0)


def test_never_reaching_resistance_in_five_sessions_is_not_triggered():
    s = series(stages={45: WATCH})
    row, end = pr.simulate_signal(s, 45, "CLOSEST", cost)
    assert row["outcome"] == "NOT_TRIGGERED" and not row["triggered"] and row["net_pct"] is None and end == 50


def test_a_fall_of_one_risk_unit_below_the_flag_price_cancels_the_order():
    c = arr(80, base=98.0, t46=92.0)                     # R is 5, so a close at or under 98 - 5 = 93 cancels
    s = series(stages={45: WATCH}, c=c)
    row, _ = pr.simulate_signal(s, 45, "CLOSEST", cost)
    assert row["outcome"] == "FAILED_BEFORE_ENTRY"


def test_a_stop_above_the_price_does_not_cancel_every_order_at_once():
    s = series(stages={45: WATCH}, stop=99.0, res=104.0, t1=110.0)   # stock 6% under resistance: the engine's stop is ABOVE the price
    s.detail[45]["atr"] = 4.0
    row, _ = pr.simulate_signal(s, 45, "CLOSEST", cost)
    assert row["outcome"] == "NOT_TRIGGERED"


def test_stop_and_target_are_placed_around_the_price_actually_paid():
    o = arr(80, base=98.0, t46=101.0)
    c = arr(80, base=99.0)
    s = series(stages={45: CONFIRMED}, o=o, c=c, h=[x + 0.5 for x in c], l=[x - 0.5 for x in c])
    row, _ = pr.simulate_signal(s, 45, "CONFIRMED", cost)
    assert row["entry_price"] == pytest.approx(101.0) and row["stop_level"] == pytest.approx(96.0) and row["target_1"] == pytest.approx(108.5)
    pub, _ = pr.simulate_signal(s, 45, "CONFIRMED", cost, rebase=False)
    assert pub["stop_level"] == pytest.approx(95.0) and pub["target_1"] == pytest.approx(107.5)


def test_a_confirmed_breakout_that_opens_above_the_published_target_is_skipped_only_on_the_published_levels():
    o = arr(80, base=98.0, t46=108.0)
    s = series(stages={45: CONFIRMED}, o=o, c=arr(80, base=108.0))
    assert pr.simulate_signal(s, 45, "CONFIRMED", cost, rebase=False)[0]["outcome"] == "SKIPPED"
    assert pr.simulate_signal(s, 45, "CONFIRMED", cost)[0]["outcome"] != "SKIPPED"


def test_filled_bar_that_also_reaches_the_stop_is_a_loss():
    h = arr(80, base=98.5, t46=100.5)
    l = arr(80, base=97.5, t46=94.0)
    s = series(stages={45: WATCH}, h=h, l=l)
    row, _ = pr.simulate_signal(s, 45, "CLOSEST", cost)
    assert row["outcome"] == "STOPPED" and row["exit_price"] == pytest.approx(95.0) and row["bars_held"] == 0


def test_stop_wins_when_one_later_bar_touches_both_levels():
    h = arr(80, base=98.5, t46=100.5, t48=112.0)
    l = arr(80, base=97.5, t48=94.0)
    s = series(stages={45: WATCH}, h=h, l=l)
    row, _ = pr.simulate_signal(s, 45, "CLOSEST", cost)
    assert row["outcome"] == "STOPPED"


def test_confirmed_enters_next_open_and_times_out_after_ten_sessions():
    o = arr(80, base=98.0, t46=101.0)
    c = arr(80, base=99.0)
    c[56] = 103.0
    s = series(stages={45: CONFIRMED}, o=o, c=c, h=[x + 0.5 for x in c], l=[x - 0.5 for x in c])
    row, end = pr.simulate_signal(s, 45, "CONFIRMED", cost)
    assert row["outcome"] == "TIME_EXIT" and row["entry_price"] == pytest.approx(101.0) and row["exit_price"] == pytest.approx(103.0) and end == 56


def test_signal_on_the_last_bar_is_pending_and_an_unfinished_trade_is_open():
    s = series(n=50, stages={49: WATCH, 40: CONFIRMED})
    assert pr.simulate_signal(s, 49, "CLOSEST", cost)[0]["outcome"] == "PENDING"
    row, _ = pr.simulate_signal(s, 40, "CONFIRMED", cost)
    assert row["outcome"] == "OPEN"
    st = pr.stats([row])
    assert st["signals"] == 0 and st["triggered"] == 0          # neither pending nor open trades are statistics


def test_repeat_flags_while_a_trade_is_live_are_not_traded_again():
    h = arr(80, base=98.5, t47=100.4, t50=111.0)
    s = series(stages={45: WATCH, 46: WATCH, 48: WATCH, 60: WATCH}, h=h)
    rows = [r for r in pr.replay_symbol(s, cost) if r["setup"] == "CLOSEST"]
    assert [r["signal_date"] for r in rows] == [s.dates[45], s.dates[60]]


def test_stats_count_what_happened_and_never_claim_a_verdict_on_a_small_sample():
    rows = []
    for i in range(3):
        h = arr(80, base=98.5, t47=100.4, t50=111.0)
        rows += pr.replay_symbol(series(stages={45: WATCH}, h=h), cost)
    rows += pr.replay_symbol(series(stages={45: WATCH}), cost)
    st = pr.stats([r for r in rows if r["setup"] == "CLOSEST"])
    assert st["signals"] == 4 and st["triggered"] == 3 and st["triggered_pct"] == 75.0 and st["hit_target"] == 3 and st["not_triggered"] == 1
    assert st["win_rate_pct"] == 100.0 and st["mean_net_pct"] == pytest.approx(7.0) and st["verdict"] == "INSUFFICIENT_DATA"
    assert st["total_net_inr_on_1L_each"] == pytest.approx(3 * 7000.0)
