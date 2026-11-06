"""Load 1-minute intraday bars and aggregate them into 5-minute session bars."""
from datetime import date, datetime, timedelta, timezone
from typing import Optional

import numpy as np
import pandas as pd

from .config import BASE_BAR_MINUTES, MIN_TICKS_PER_BAR, SESSION_CLOSE, SESSION_OPEN

IST = timezone(timedelta(hours=5, minutes=30))


def load_minute_bars(conn, since: Optional[date] = None, until: Optional[date] = None) -> pd.DataFrame:
    """VALID 1-minute bars for active instruments. SUSPECT bars are excluded, never repaired."""
    sql = """
        SELECT i.symbol_id, s.symbol, i.timestamp AS ts, i.open, i.high, i.low, i.close, i.volume
        FROM ohlcv_intraday i
        JOIN symbols s ON s.id = i.symbol_id
        WHERE s.is_active
          AND i.quality_status = 'VALID'
          AND i.interval_minutes = 1
          AND (%(since)s::date IS NULL OR i.timestamp >= (%(since)s::date::timestamp AT TIME ZONE 'Asia/Kolkata'))
          AND (%(until)s::date IS NULL OR i.timestamp <  ((%(until)s::date + 1)::timestamp AT TIME ZONE 'Asia/Kolkata'))
        ORDER BY i.symbol_id, i.timestamp
    """
    with conn.cursor() as cur:
        cur.execute(sql, {"since": since, "until": until})
        rows = cur.fetchall()
    df = pd.DataFrame(rows, columns=["symbol_id", "symbol", "ts", "open", "high", "low", "close", "volume"])
    if df.empty:
        return df
    for c in ("open", "high", "low", "close"):
        df[c] = df[c].astype(float)
    df["volume"] = df["volume"].astype(float)
    df["ts"] = pd.to_datetime(df["ts"], utc=True)
    return df


def to_session_bars(minute: pd.DataFrame) -> pd.DataFrame:
    """Aggregate to 5-minute bars inside the NSE session.

    A bar is labelled by its start time; its information is available at `bar_end` (start + 5 min).
    Bars assembled from fewer than MIN_TICKS_PER_BAR minutes are dropped rather than filled.
    """
    if minute.empty:
        return minute.assign(date=[], bar_start=[], bar_end=[], n_ticks=[])
    m = minute.sort_values(["symbol_id", "ts"]).copy()
    ist = m["ts"].dt.tz_convert(IST)
    m["bar_start"] = ist.dt.floor(f"{BASE_BAR_MINUTES}min")
    t = m["bar_start"].dt.time
    m = m[(t >= SESSION_OPEN) & (t < SESSION_CLOSE)]

    bars = (
        m.groupby(["symbol_id", "symbol", "bar_start"], sort=True)
        .agg(open=("open", "first"), high=("high", "max"), low=("low", "min"),
             close=("close", "last"), volume=("volume", "sum"), n_ticks=("close", "size"))
        .reset_index()
    )
    bars = bars[bars["n_ticks"] >= MIN_TICKS_PER_BAR].copy()
    bars["date"] = bars["bar_start"].dt.date
    bars["bar_end"] = bars["bar_start"] + pd.Timedelta(minutes=BASE_BAR_MINUTES)
    return bars.sort_values(["symbol_id", "date", "bar_start"]).reset_index(drop=True)


def session_close_ist(d: date) -> datetime:
    return datetime.combine(d, SESSION_CLOSE, tzinfo=IST)


def safe_log_ratio(a, b):
    with np.errstate(divide="ignore", invalid="ignore"):
        return np.log(np.asarray(a, dtype=float) / np.asarray(b, dtype=float))


def load_five_minute_bars(conn, since: Optional[date] = None, until: Optional[date] = None) -> pd.DataFrame:
    """VALID 5-minute history rows (backfilled from the provider's 60-day window), as session bars."""
    sql = """
        SELECT i.symbol_id, s.symbol, i.timestamp AS ts, i.open, i.high, i.low, i.close, i.volume
        FROM ohlcv_intraday i
        JOIN symbols s ON s.id = i.symbol_id
        WHERE s.is_active AND i.quality_status = 'VALID' AND i.interval_minutes = 5
          AND (%(since)s::date IS NULL OR i.timestamp >= (%(since)s::date::timestamp AT TIME ZONE 'Asia/Kolkata'))
          AND (%(until)s::date IS NULL OR i.timestamp <  ((%(until)s::date + 1)::timestamp AT TIME ZONE 'Asia/Kolkata'))
        ORDER BY i.symbol_id, i.timestamp
    """
    with conn.cursor() as cur:
        cur.execute(sql, {"since": since, "until": until})
        rows = cur.fetchall()
    df = pd.DataFrame(rows, columns=["symbol_id", "symbol", "ts", "open", "high", "low", "close", "volume"])
    return five_minute_rows_to_bars(df)


def five_minute_rows_to_bars(df: pd.DataFrame) -> pd.DataFrame:
    """Shape stored 5-minute rows like `to_session_bars` output. Rows are already bars; only session filtering applies."""
    cols = ["symbol_id", "symbol", "bar_start", "open", "high", "low", "close", "volume", "n_ticks", "date", "bar_end"]
    if df.empty:
        return pd.DataFrame(columns=cols)
    b = df.copy()
    for c in ("open", "high", "low", "close", "volume"):
        b[c] = b[c].astype(float)
    b["bar_start"] = pd.to_datetime(b["ts"], utc=True).dt.tz_convert(IST)
    t = b["bar_start"].dt.time
    b = b[(t >= SESSION_OPEN) & (t < SESSION_CLOSE)].copy()
    b["n_ticks"] = BASE_BAR_MINUTES  # a stored 5-minute bar is complete by construction
    b["date"] = b["bar_start"].dt.date
    b["bar_end"] = b["bar_start"] + pd.Timedelta(minutes=BASE_BAR_MINUTES)
    return b[cols].sort_values(["symbol_id", "date", "bar_start"]).reset_index(drop=True)


def merge_bar_sources(bars_1m: pd.DataFrame, bars_5m: pd.DataFrame) -> pd.DataFrame:
    """Union of 1-minute-derived and stored 5-minute bars. Where both exist for the same (symbol, bar) the
    1-minute-derived bar wins: it is the live feed the forecasts will actually see."""
    if bars_5m.empty:
        return bars_1m
    if bars_1m.empty:
        return bars_5m
    out = pd.concat([bars_1m, bars_5m], ignore_index=True)
    out = out.drop_duplicates(subset=["symbol_id", "bar_start"], keep="first")
    return out.sort_values(["symbol_id", "date", "bar_start"]).reset_index(drop=True)


def load_session_bars(conn, since: Optional[date] = None, until: Optional[date] = None) -> pd.DataFrame:
    """All available 5-minute session bars: live 1-minute data plus the 60-day 5-minute history."""
    return merge_bar_sources(to_session_bars(load_minute_bars(conn, since, until)), load_five_minute_bars(conn, since, until))
