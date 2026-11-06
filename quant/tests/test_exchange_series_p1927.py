"""P1.9.27: research series from NSE records. Pure-function tests."""
from datetime import date, timedelta

import pandas as pd
import pytest

from src.engines.research.exchange_series import adjust_symbol


def bars(closes, start=date(2024, 1, 1), opens=None):
    ds = [start + timedelta(days=i) for i in range(len(closes))]
    return pd.DataFrame({"symbol": "X", "date": ds, "open": opens or closes, "high": closes, "low": closes, "close": closes,
                         "volume": [1000] * len(closes)})


def act(ex, factor, cls="BONUS", vendor=None):
    return pd.DataFrame([{"symbol": "X", "ex_date": ex, "action_class": cls, "subject": "s", "factor": factor, "vendor_ratio": vendor}])


def test_a_bonus_leaves_no_false_return_and_keeps_recent_prices_as_traded():
    b = bars([100, 102, 51, 52], opens=[100, 101, 50.5, 51])
    adj, audit, breaks = adjust_symbol("X", b, act(date(2024, 1, 3), 0.5))
    assert audit[0].status == "CONFIRMED" and breaks == []
    assert adj["close"].tolist() == [50.0, 51.0, 51.0, 52.0]
    assert adj["volume"].tolist()[:2] == [2000, 2000], "shares before a 1:1 bonus double"
    assert adj["volume"].tolist()[2:] == [1000, 1000]


def test_two_actions_on_one_day_multiply():
    b = bars([1000, 1000, 100, 100], opens=[1000, 1000, 100, 100])
    a = pd.concat([act(date(2024, 1, 3), 0.5), act(date(2024, 1, 3), 0.2, "SPLIT")])
    adj, audit, _ = adjust_symbol("X", b, a)
    assert audit[0].factor == pytest.approx(0.1)
    assert adj["close"].tolist() == [100.0, 100.0, 100.0, 100.0]


def test_a_factor_the_market_contradicts_is_flagged_and_not_applied():
    b = bars([100, 100, 99, 100], opens=[100, 100, 99, 100])
    adj, audit, _ = adjust_symbol("X", b, act(date(2024, 1, 3), 0.5))
    assert audit[0].status == "FACTOR_MISMATCH"
    assert adj["close"].iloc[0] == 100, "no adjustment on a factor the price step does not support"


def test_an_unexplained_break_drops_earlier_history_instead_of_faking_a_loss():
    b = bars([100, 100, 40, 41], opens=[100, 100, 40, 41])
    adj, audit, breaks = adjust_symbol("X", b, act(date(2030, 1, 1), 0.5).iloc[0:0])
    assert len(breaks) == 1 and breaks[0]["step"] == pytest.approx(0.4)
    assert len(adj) == 2 and adj["close"].iloc[0] == 40


def test_a_demerger_uses_the_vendor_ratio_when_it_exists_else_the_observed_step():
    b = bars([100, 100, 96, 97], opens=[100, 100, 96, 97])
    adj, audit, _ = adjust_symbol("X", b, act(date(2024, 1, 3), None, "DEMERGER", 0.96))
    assert audit[0].basis == "DEMERGER_OBSERVED_STEP" and audit[0].factor == pytest.approx(0.96)
    adj2, audit2, _ = adjust_symbol("X", b, act(date(2024, 1, 3), None, "DEMERGER", None))
    assert audit2[0].factor == pytest.approx(0.96)
