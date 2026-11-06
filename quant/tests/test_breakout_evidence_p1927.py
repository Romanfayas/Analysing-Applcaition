"""P1.9.27: the breakout study's trade simulation and its holdout discipline. Pure-function tests."""
import numpy as np
import pandas as pd

from src.engines.research.breakout_evidence import MIN_GROUP_TRADES, _simulate, evaluate_rules, summarize


def data(o, h, l, c, signals):
    n = len(o)
    return {"o": np.array(o, float), "h": np.array(h, float), "l": np.array(l, float), "c": np.array(c, float),
            "dates": np.array([str(pd.Timestamp("2024-01-01") + pd.Timedelta(days=i))[:10] for i in range(n)]), "signals": signals}


SIG = {"t": 0, "date": "2024-01-01", "stop": 95.0, "t1": 110.0, "t2": 120.0, "entry_high": 101.0, "atr": 2.0}
PLAN = {"id": "T1", "target": "T1", "limit": 10}
gross = lambda entry, px, days: (px / entry - 1) * 100
always = lambda d: True


def test_entry_is_the_next_open_and_never_the_signal_bar():
    d = data([100, 102, 103, 104], [100, 103, 111, 105], [100, 101, 102, 103], [100, 102, 110, 104], [SIG])
    r = _simulate(d, PLAN, None, always, gross)
    assert r[0]["entry"] == 102
    assert r[0]["why"] == "TARGET" and r[0]["exit"] == 110


def test_a_bar_touching_both_assumes_the_stop():
    d = data([100, 102, 102], [100, 103, 112], [100, 101, 94], [100, 102, 100], [SIG])
    assert _simulate(d, PLAN, None, always, gross)[0]["why"] == "STOP_LOSS"


def test_extended_entries_are_skipped_by_the_filter():
    d = data([100, 104, 104, 104], [100, 105, 105, 105], [100, 103, 103, 103], [100, 104, 104, 104], [SIG])
    assert _simulate(d, PLAN, 1.0, always, gross) == [], "the open is 3% above the zone"
    assert len(_simulate(d, PLAN, None, always, gross)) == 1


def test_a_trailing_stop_uses_only_earlier_highs():
    plan = {"id": "T", "trail_atr": 2.0, "limit": 20}
    d = data([100, 100, 100, 110], [100, 100, 110, 111], [100, 100, 100, 105], [100, 100, 109, 106], [SIG])
    # The bar making the 110 high has a low of 100: it must not be stopped by a trail that high itself created.
    r = _simulate(d, plan, None, always, gross)
    assert r[0]["why"] == "TIME_LIMIT" or r[0]["exit_date"] != d["dates"][2]


def rows(n, net, period, flag):
    return [{"net_return_pct": net, "gross_return_pct": net, "exit_reason": "TARGET", "mfe_pct": 1, "mae_pct": -1, "bars_held": 3, "period": period,
             "extension_pct": 0 if flag else 5, "rvol": 3 if flag else 1, "structure_score": 80 if flag else 50, "atr_pct": 1 if flag else 5, "market_uptrend": flag} for _ in range(n)]


def test_a_rule_that_fails_on_unseen_data_is_rejected():
    r = rows(MIN_GROUP_TRADES, 2.0, "DISCOVERY", True) + rows(MIN_GROUP_TRADES, -1.0, "DISCOVERY", False) + \
        rows(MIN_GROUP_TRADES, -3.0, "HOLDOUT", True) + rows(MIN_GROUP_TRADES, 1.0, "HOLDOUT", False)
    assert all(x["verdict"] == "REJECTED" for x in evaluate_rules(r))


def test_a_rule_that_holds_on_unseen_data_is_adopted():
    r = rows(MIN_GROUP_TRADES, 2.0, "DISCOVERY", True) + rows(MIN_GROUP_TRADES, -1.0, "DISCOVERY", False) + \
        rows(MIN_GROUP_TRADES, 1.5, "HOLDOUT", True) + rows(MIN_GROUP_TRADES, -1.0, "HOLDOUT", False)
    assert any(x["verdict"] == "ADOPTED" for x in evaluate_rules(r))


def test_summarize_handles_no_trades():
    assert summarize([]) == {"trades": 0}
