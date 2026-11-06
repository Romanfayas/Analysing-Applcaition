"""
P1.7 daily-data audit against a versioned exchange calendar.

Everything is computed from live queries; nothing is hardcoded or faked on failure.
If the database is unreachable the audit raises, it never returns "clean".
"""
import json
import os
from datetime import date, timedelta
from pathlib import Path
from typing import Any, Dict, List, Optional, Set

import psycopg2

CALENDAR_PATH = Path(os.getenv("NSE_CALENDAR_PATH", Path(__file__).resolve().parents[3] / "data" / "nse_calendar_v1.json"))
SESSION_STAMP_UTC = "03:45:00"  # 09:15 IST


def load_calendar(path: Path = CALENDAR_PATH) -> Dict[str, Any]:
    with open(path) as f:
        return json.load(f)


def expected_sessions(start: date, end: date, cal: Optional[Dict[str, Any]] = None) -> List[date]:
    """Weekdays minus full-day holidays, plus special (weekend/evening) sessions, inclusive."""
    cal = cal or load_calendar()
    holidays = {date.fromisoformat(d) for d in cal["full_day_holidays"]}
    special = {date.fromisoformat(d) for d in cal["special_sessions"]}
    out, d = [], start
    while d <= end:
        if (d.weekday() < 5 and d not in holidays) or d in special:
            out.append(d)
        d += timedelta(days=1)
    return out


def connect(db_url: Optional[str] = None):
    return psycopg2.connect(db_url or os.environ["DATABASE_URL"])


def audit_daily(conn, symbol_filter: Optional[List[str]] = None) -> Dict[str, Any]:
    """Returns a full audit dict. Raises on DB errors."""
    cal = load_calendar()
    holidays = {date.fromisoformat(d) for d in cal["full_day_holidays"]}
    cur = conn.cursor()
    where = "" if not symbol_filter else "AND s.symbol = ANY(%(syms)s)"
    params = {"syms": symbol_filter}
    cur.execute(f"""
        SELECT s.symbol, (o.timestamp AT TIME ZONE 'Asia/Kolkata')::date, o.volume, o.open, o.high, o.low, o.close,
               o.quality_status::text, (o.timestamp AT TIME ZONE 'UTC')::time::text, o.adjusted_close
        FROM ohlcv_daily o JOIN symbols s ON s.id = o.symbol_id WHERE COALESCE(s.metadata->>'user_added', 'false') <> 'true' {where}""", params)
    rows = cur.fetchall()
    if not rows:
        raise RuntimeError("ohlcv_daily returned no rows; refusing to report coverage")

    by_symbol: Dict[str, Dict[date, tuple]] = {}
    dup = 0
    invalid_ohlc = zero_vol_flat = off_stamp = suspect = adj_zero = adj_null = 0
    for sym, d, vol, o, h, l, c, q, tod, adj in rows:
        m = by_symbol.setdefault(sym, {})
        if d in m:
            dup += 1
        m[d] = (vol, q)
        if h < l or o <= 0 or c <= 0 or h < max(o, c) or l > min(o, c) or vol < 0:
            invalid_ohlc += 1
        if vol == 0 and o == c and h == l:
            zero_vol_flat += 1
        if tod != SESSION_STAMP_UTC:
            off_stamp += 1
        if q != "VALID":
            suspect += 1
        if adj is None:
            adj_null += 1
        elif adj == 0:
            adj_zero += 1

    all_dates = [d for m in by_symbol.values() for d in m]
    lo, hi = min(all_dates), max(all_dates)
    expected = expected_sessions(lo, hi, cal)
    exp_set: Set[date] = set(expected)

    per_symbol = {}
    unexpected_total = missing_total = 0
    holiday_bars = 0
    for sym, m in sorted(by_symbol.items()):
        have = set(m)
        missing = sorted(exp_set - have)
        unexpected = sorted(have - exp_set)
        holiday_bars += sum(1 for d in have if d in holidays)
        missing_total += len(missing)
        unexpected_total += len(unexpected)
        per_symbol[sym] = {"bars": len(have), "missing_sessions": [d.isoformat() for d in missing],
                           "unexpected_dates": [d.isoformat() for d in unexpected]}

    cur.execute("SELECT interval_minutes, count(*) FROM ohlcv_intraday GROUP BY 1 ORDER BY 1")
    intraday = {r[0]: r[1] for r in cur.fetchall()}
    quarantined = None
    if _table_exists(cur, "ohlcv_daily_quarantine_p17"):
        cur.execute("SELECT count(*) FROM ohlcv_daily_quarantine_p17")
        quarantined = cur.fetchone()[0]

    return {
        "calendar_version": cal["calendar_version"],
        "date_range": [lo.isoformat(), hi.isoformat()],
        "total_bars": len(rows),
        "symbols": len(by_symbol),
        "expected_sessions_in_range": len(expected),
        "expected_bars": len(expected) * len(by_symbol),
        "missing_bars": missing_total,
        "unexpected_bars": unexpected_total,
        "holiday_bars": holiday_bars,
        "duplicate_symbol_date": dup,
        "invalid_ohlc": invalid_ohlc,
        "zero_volume_flat_bars": zero_vol_flat,
        "off_session_stamp": off_stamp,
        "non_valid_quality": suspect,
        "adjusted_close_zero": adj_zero,
        "adjusted_close_null": adj_null,
        "intraday_interval_counts": intraday,
        "quarantined_p17": quarantined,
        "last_bar_date": hi.isoformat(),
        "per_symbol": per_symbol,
    }


def _table_exists(cur, name: str) -> bool:
    cur.execute("SELECT to_regclass(%s) IS NOT NULL", (name,))
    return cur.fetchone()[0]
