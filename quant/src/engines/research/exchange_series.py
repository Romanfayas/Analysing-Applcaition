"""Research price series built from NSE's own records (P1.9.27).

The vendor's daily history stays quarantined because its "adjusted" closes cannot be tied to evidence. This module does
not touch it. It builds a separate series from two NSE sources:

  * bhavcopy OHLCV (`exchange_daily_bars`): the prices that actually traded;
  * NSE's corporate-actions feed, through the stored reconciliation (`ca_reconciliation_findings`): split/bonus factors.

Every adjustment is checked against the exchange's own price step on the ex-date, so a factor that does not match what
the market did is flagged rather than applied blindly. Any overnight discontinuity with no matching action is reported
as UNEXPLAINED and history before it is dropped for that symbol: an unexplained break must never look like a return.

What this series is not: it is not certified. The feed artifacts are AUTHENTICITY_PENDING until a person reviews them, cash
dividends are not added back (price return, which understates a holder's return), and the universe is today's NIFTY 50, so
results carry survivorship bias. `SERIES_LIMITATIONS` travels with every result built on it.
"""
from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import date
from typing import Any, Dict, List, Optional, Tuple

import pandas as pd

SERIES_VERSION = "NSE_EXCHANGE_ADJUSTED_V1"
# A split/bonus step may differ from the factor by a normal overnight move.
MAX_FACTOR_MISMATCH = 0.10
# An overnight step this large with no corporate action is treated as a break in the series.
UNEXPLAINED_GAP = 0.20

SERIES_LIMITATIONS = [
    "Built from NSE bhavcopy prices and NSE's corporate-action feed; the feed artifacts await human review, so this is research-grade, not certified.",
    "Price return only: cash dividends are not added back, which understates a holder's return.",
    "The universe is today's index members, so companies that dropped out are missing (survivorship bias flatters results).",
    "Costs are modelled and fills are simulated at the next session's open or at the stop/target level; real fills will differ.",
]


@dataclass
class Adjustment:
    symbol: str
    ex_date: str
    factor: float
    observed_step: Optional[float]
    subjects: str
    basis: str   # NSE_FEED_FACTOR | DEMERGER_OBSERVED_STEP
    status: str  # CONFIRMED | FACTOR_MISMATCH | NO_PRICE_ON_EX_DATE


def load_bars(conn, include_user_added: bool = False) -> pd.DataFrame:
    extra = "" if include_user_added else "AND COALESCE(s.metadata->>'user_added', 'false') <> 'true'"
    with conn.cursor() as cur:
        cur.execute(f"""
            SELECT s.id, s.symbol, e.trade_date, e.open::float8, e.high::float8, e.low::float8, e.close::float8, e.volume
            FROM exchange_daily_bars e JOIN symbols s ON s.id = e.symbol_id
            WHERE s.is_active {extra}
            ORDER BY s.symbol, e.trade_date""")
        rows = cur.fetchall()
    return pd.DataFrame(rows, columns=["symbol_id", "symbol", "date", "open", "high", "low", "close", "volume"])


def load_actions(conn) -> pd.DataFrame:
    """Price-changing actions from the newest reconciliation run (one row per action; several can share an ex-date)."""
    with conn.cursor() as cur:
        cur.execute("""
            SELECT DISTINCT s.symbol, f.ex_date, f.action_class, f.feed_subject,
                   f.implied_price_factor::float8, f.observed_vendor_ratio::float8
            FROM ca_reconciliation_findings f JOIN symbols s ON s.id = f.symbol_id
            WHERE f.run_id = (SELECT max(run_id) FROM ca_reconciliation_findings)
              AND f.ex_date IS NOT NULL
              AND (f.action_class = 'DEMERGER' OR (f.implied_price_factor IS NOT NULL AND f.implied_price_factor <> 1))""")
        rows = cur.fetchall()
    return pd.DataFrame(rows, columns=["symbol", "ex_date", "action_class", "subject", "factor", "vendor_ratio"])


def adjust_symbol(sym: str, bars: pd.DataFrame, actions: pd.DataFrame) -> Tuple[pd.DataFrame, List[Adjustment], List[Dict[str, Any]]]:
    """Back-adjust one symbol. Pure: takes frames, returns the adjusted frame, the audit trail and any unexplained breaks."""
    b = bars.sort_values("date").reset_index(drop=True).copy()
    if b.empty:
        return b, [], []
    pos = {d: i for i, d in enumerate(b["date"])}
    audit: List[Adjustment] = []
    factors: Dict[int, float] = {}  # bar index of the ex-date -> factor applied to everything before it

    grouped = actions.groupby("ex_date") if not actions.empty else []
    for ex, grp in grouped:
        i = pos.get(ex)
        if i is None:  # ex-date without a bar: use the next session that traded
            later = [d for d in b["date"] if d >= ex]
            i = pos[later[0]] if later else None
        priced = grp[grp["factor"].notna() & (grp["factor"] != 1)]
        demerger = grp[grp["action_class"] == "DEMERGER"]
        step = float(b["open"].iloc[i] / b["close"].iloc[i - 1]) if i not in (None, 0) else None
        if not priced.empty:
            f = float(priced["factor"].prod())
            basis = "NSE_FEED_FACTOR"
        elif not demerger.empty and step is not None:
            vendor = demerger["vendor_ratio"].dropna()
            usable = vendor[(vendor > 0) & (vendor != 1)]
            f = float(usable.iloc[0]) if len(usable) else step
            basis = "DEMERGER_OBSERVED_STEP"
        else:
            continue
        status = "NO_PRICE_ON_EX_DATE" if step is None else ("CONFIRMED" if abs(step / f - 1) <= MAX_FACTOR_MISMATCH else "FACTOR_MISMATCH")
        audit.append(Adjustment(sym, str(ex), round(f, 6), None if step is None else round(step, 6), "; ".join(sorted(set(grp["subject"].dropna())))[:160], basis, status))
        if i not in (None, 0) and status == "CONFIRMED":
            factors[i] = f

    # Unexplained breaks: a large step on a day with no confirmed action.
    breaks: List[Dict[str, Any]] = []
    closes, opens = b["close"].to_numpy(float), b["open"].to_numpy(float)
    for i in range(1, len(b)):
        if i in factors:
            continue
        step = opens[i] / closes[i - 1]
        if abs(step - 1) > UNEXPLAINED_GAP:
            breaks.append({"symbol": sym, "date": str(b["date"].iloc[i]), "step": round(float(step), 4)})

    cum = [1.0] * len(b)
    running = 1.0
    for i in range(len(b) - 1, -1, -1):
        cum[i] = running
        if i in factors:
            running *= factors[i]
    for col in ("open", "high", "low", "close"):
        b[col] = b[col] * cum
    b["volume"] = [int(round(v / c)) for v, c in zip(b["volume"], cum)]
    if breaks:
        last = max(pd.to_datetime(x["date"]).date() for x in breaks)
        b = b[b["date"] >= last].reset_index(drop=True)
    return b, audit, breaks


def build_series(conn, include_user_added: bool = False) -> Tuple[pd.DataFrame, Dict[str, Any]]:
    bars = load_bars(conn, include_user_added)
    actions = load_actions(conn)
    frames, audit, breaks = [], [], []
    for sym, grp in bars.groupby("symbol"):
        adj, a, br = adjust_symbol(sym, grp, actions[actions["symbol"] == sym] if not actions.empty else actions)
        frames.append(adj)
        audit.extend(a)
        breaks.extend(br)
    df = pd.concat(frames, ignore_index=True) if frames else bars
    report = {
        "series_version": SERIES_VERSION,
        "symbols": int(df["symbol"].nunique()) if len(df) else 0,
        "bars": int(len(df)),
        "from": str(df["date"].min()) if len(df) else None,
        "to": str(df["date"].max()) if len(df) else None,
        "adjustments_total": len(audit),
        "adjustments_confirmed": sum(1 for a in audit if a.status == "CONFIRMED"),
        "adjustments_flagged": [asdict(a) for a in audit if a.status != "CONFIRMED"],
        "adjustments": [asdict(a) for a in audit],
        "unexplained_breaks": breaks,
        "limitations": SERIES_LIMITATIONS,
    }
    return df, report
