"""Point-in-time features and cost-aware labels. Nothing crosses a session boundary.

Every feature at bar t uses only bars <= t of the SAME session (plus the cross-section of other
instruments at the same bar t). Labels look forward h bars within the same session and are NaN
when the horizon would run past the close.
"""
import numpy as np
import pandas as pd

from .config import BASE_BAR_MINUTES, FEATURES, SESSION_OPEN
from .data import safe_log_ratio


def _rsi(close: pd.Series, n: int = 14) -> pd.Series:
    d = close.diff()
    up = d.clip(lower=0).rolling(n, min_periods=n).mean()
    dn = (-d.clip(upper=0)).rolling(n, min_periods=n).mean()
    rs = up / dn.replace(0, np.nan)
    rsi = 100 - 100 / (1 + rs)
    return rsi.where(dn != 0, 100.0).where(up.notna())


def build_features(bars: pd.DataFrame) -> pd.DataFrame:
    df = bars.copy()
    g = df.groupby(["symbol_id", "date"], sort=False)

    df["lr"] = safe_log_ratio(df["close"], g["close"].shift(1))
    for k in (1, 3, 6, 12):
        df[f"ret_{k}"] = safe_log_ratio(df["close"], g["close"].shift(k))

    g = df.groupby(["symbol_id", "date"], sort=False)
    df["vol_6"] = g["lr"].transform(lambda s: s.rolling(6, min_periods=3).std())
    df["vol_12"] = g["lr"].transform(lambda s: s.rolling(12, min_periods=6).std())
    df["range_pct"] = (df["high"] - df["low"]) / df["close"]
    df["atr_6"] = g["range_pct"].transform(lambda s: s.rolling(6, min_periods=3).mean())

    day_low = g["low"].cummin()
    day_high = g["high"].cummax()
    span = (day_high - day_low).replace(0, np.nan)
    df["day_pos"] = (df["close"] - day_low) / span

    typical = (df["high"] + df["low"] + df["close"]) / 3
    df["_pv"] = typical * df["volume"]
    cum_pv = df.groupby(["symbol_id", "date"], sort=False)["_pv"].cumsum()
    cum_v = g["volume"].cumsum().replace(0, np.nan)
    df["vwap_dist"] = df["close"] / (cum_pv / cum_v) - 1

    df["ret_from_open"] = safe_log_ratio(df["close"], g["open"].transform("first"))
    vol_mean = g["volume"].transform(lambda s: s.rolling(12, min_periods=3).mean()).replace(0, np.nan)
    df["vol_ratio"] = df["volume"] / vol_mean
    df["rsi_14"] = g["close"].transform(_rsi)

    open_minutes = SESSION_OPEN.hour * 60 + SESSION_OPEN.minute
    start_minutes = df["bar_start"].dt.hour * 60 + df["bar_start"].dt.minute
    df["minutes_since_open"] = (start_minutes - open_minutes + BASE_BAR_MINUTES).astype(float)

    # Cross-section at the same bar: known at bar_end for every instrument.
    xs = df.groupby("bar_start")
    df["mkt_ret_1"] = xs["ret_1"].transform("mean")
    df["mkt_ret_3"] = xs["ret_3"].transform("mean")
    df["mkt_ret_12"] = xs["ret_12"].transform("mean")
    df["rel_ret_3"] = df["ret_3"] - df["mkt_ret_3"]
    df["mkt_breadth_3"] = xs["ret_3"].transform(lambda s: (s > 0).mean() if s.notna().any() else np.nan)

    return df.drop(columns=["_pv"])


def add_labels(df: pd.DataFrame, h_bars: int, cost: float) -> pd.DataFrame:
    """Forward log return over h bars within the session, plus a 3-class label.

    UP / DOWN require the move to exceed the round-trip cost; anything smaller is NEUTRAL because it
    cannot be traded profitably. The forward bar must be exactly h bars later in time (no gaps).
    """
    out = df.copy()
    g = out.groupby(["symbol_id", "date"], sort=False)
    fwd_close = g["close"].shift(-h_bars)
    fwd_start = g["bar_start"].shift(-h_bars)
    contiguous = (fwd_start - out["bar_start"]) == pd.Timedelta(minutes=BASE_BAR_MINUTES * h_bars)
    out["fwd"] = np.where(contiguous, safe_log_ratio(fwd_close, out["close"]), np.nan)
    out["label"] = np.select([out["fwd"] > cost, out["fwd"] < -cost], [2, 0], default=1)
    out.loc[out["fwd"].isna(), "label"] = -1
    return out


def feature_matrix(df: pd.DataFrame) -> np.ndarray:
    return df[FEATURES].to_numpy(dtype=float)
