#!/usr/bin/env python3
"""P1.9.24 vendor-vs-exchange daily reconciliation report (read-only).

Compares every stored vendor daily bar (ohlcv_daily, Yahoo) with NSE's own end-of-day record (exchange_daily_bars,
loaded from bhavcopies) and writes Markdown + JSON. It modifies nothing.

Usage: python3 scripts/p1924_reconciliation_report.py [output_basename]
"""
import csv
import io
import json
import subprocess
import sys
from datetime import datetime, timezone

OUT = sys.argv[1] if len(sys.argv) > 1 else "reports/audits/HALALEQUITY_P1_9_24_RECONCILIATION_REPORT"


def q(sql):
    """Run a read-only query through the database container and return rows as dicts."""
    cmd = ["docker", "exec", "-i", "halal_equity_postgres", "sh", "-c",
           'psql -X -q --csv -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d halal_equity']
    res = subprocess.run(cmd, input="BEGIN READ ONLY;\n" + sql.rstrip().rstrip(";") + ";\nCOMMIT;\n",
                         capture_output=True, text=True, check=True)
    lines = [l for l in res.stdout.splitlines() if l not in ("BEGIN", "COMMIT")]
    return list(csv.DictReader(io.StringIO("\n".join(lines))))


R = {}
V = "evidence.v_daily_bar_reconciliation"
VENDOR_FIRST = q("SELECT min((timestamp AT TIME ZONE 'Asia/Kolkata')::date) d FROM ohlcv_daily")[0]["d"]

R["coverage"] = {
    "exchange_files": q("SELECT count(*) n, min(trade_date) first, max(trade_date) last FROM exchange_source_files")[0],
    "exchange_files_by_year": q("SELECT extract(year FROM trade_date)::int y, count(*) files FROM exchange_source_files GROUP BY 1 ORDER BY 1"),
    "exchange_bars": q("SELECT count(*) n, count(DISTINCT symbol_id) symbols FROM exchange_daily_bars")[0],
    "vendor_bars": q("""SELECT count(*) n, count(DISTINCT symbol_id) symbols, min((timestamp AT TIME ZONE 'Asia/Kolkata')::date) first,
                         max((timestamp AT TIME ZONE 'Asia/Kolkata')::date) last FROM ohlcv_daily""")[0],
    "missing_weekday_files": q(f"""
        SELECT d::date AS weekday FROM generate_series('2022-01-03'::date, (SELECT max(trade_date) FROM exchange_source_files), '1 day') d
        WHERE extract(isodow FROM d) < 6 AND NOT EXISTS (SELECT 1 FROM exchange_source_files f WHERE f.trade_date = d::date)
          AND EXISTS (SELECT 1 FROM ohlcv_daily o WHERE (o.timestamp AT TIME ZONE 'Asia/Kolkata')::date = d::date)
        ORDER BY 1"""),
}

R["classification_overlap"] = q(f"""
    SELECT classification, count(*) n FROM {V}
    WHERE trade_date >= '{VENDOR_FIRST}' GROUP BY 1 ORDER BY 2 DESC""")
R["classification_pre_vendor"] = q(f"""
    SELECT classification, count(*) n FROM {V} WHERE trade_date < '{VENDOR_FIRST}' GROUP BY 1 ORDER BY 2 DESC""")

R["matched"] = q(f"""
    SELECT count(*) FILTER (WHERE classification = 'NOMINAL_MATCH') nominal_match_within_5bps,
           count(*) FILTER (WHERE classification = 'NOMINAL_MATCH' AND vendor_close = exchange_close) exact_close
    FROM {V} WHERE trade_date >= '{VENDOR_FIRST}'""")[0]

R["duplicates"] = {
    "vendor_same_symbol_same_ist_date": q("""
        SELECT s.symbol, (d.timestamp AT TIME ZONE 'Asia/Kolkata')::date AS trade_date, count(*) n
        FROM ohlcv_daily d JOIN symbols s ON s.id = d.symbol_id GROUP BY 1, 2 HAVING count(*) > 1 ORDER BY 1, 2"""),
    "exchange": "0 by construction: (symbol_id, trade_date) is the primary key of exchange_daily_bars",
}

R["timestamps"] = {
    "vendor_not_session_open_label": q("""
        SELECT to_char(timestamp AT TIME ZONE 'UTC', 'HH24:MI:SS') utc_time, count(*) n FROM ohlcv_daily
        GROUP BY 1 ORDER BY 2 DESC"""),
    "vendor_bars_on_dates_without_exchange_session": q(f"""
        SELECT symbol, trade_date FROM {V} WHERE classification = 'NO_EXCHANGE_FILE' AND trade_date >= '{VENDOR_FIRST}' ORDER BY 2, 1"""),
    # a vendor bar that matches the exchange bar of the NEXT or PREVIOUS session indicates a date shift
    "vendor_bars_matching_an_adjacent_session": q(f"""
        WITH v AS (SELECT symbol_id, (timestamp AT TIME ZONE 'Asia/Kolkata')::date d, close FROM ohlcv_daily)
        SELECT s.symbol, v.d AS vendor_date, e.trade_date AS exchange_date
        FROM v JOIN exchange_daily_bars e ON e.symbol_id = v.symbol_id AND e.trade_date <> v.d
             AND e.trade_date BETWEEN v.d - 4 AND v.d + 4 AND e.close = v.close
        JOIN symbols s ON s.id = v.symbol_id
        WHERE NOT EXISTS (SELECT 1 FROM exchange_daily_bars x WHERE x.symbol_id = v.symbol_id AND x.trade_date = v.d AND x.close = v.close)
        ORDER BY 1, 2 LIMIT 200"""),
}

R["ohlcv_differences"] = {
    "on_nominal_matches_max_abs_pct": q(f"""
        WITH p AS (SELECT r.trade_date, r.symbol_id, d.open vo, d.high vh, d.low vl, d.close vc, d.volume vv,
                          e.open eo, e.high eh, e.low el, e.close ec, e.volume ev
                   FROM {V} r
                   JOIN ohlcv_daily d ON d.symbol_id = r.symbol_id AND (d.timestamp AT TIME ZONE 'Asia/Kolkata')::date = r.trade_date
                   JOIN exchange_daily_bars e ON e.symbol_id = r.symbol_id AND e.trade_date = r.trade_date
                   WHERE r.classification = 'NOMINAL_MATCH')
        SELECT count(*) pairs,
               round(max(abs(vo / eo - 1)) * 100, 5) open_max_pct, round(max(abs(vh / eh - 1)) * 100, 5) high_max_pct,
               round(max(abs(vl / el - 1)) * 100, 5) low_max_pct, round(max(abs(vc / ec - 1)) * 100, 5) close_max_pct,
               count(*) FILTER (WHERE vo <> eo OR vh <> eh OR vl <> el OR vc <> ec) any_price_not_exact,
               round((percentile_cont(0.05) WITHIN GROUP (ORDER BY vv::numeric / NULLIF(ev, 0)))::numeric, 4) volume_ratio_p05,
               round((percentile_cont(0.50) WITHIN GROUP (ORDER BY vv::numeric / NULLIF(ev, 0)))::numeric, 4) volume_ratio_median,
               round((percentile_cont(0.95) WITHIN GROUP (ORDER BY vv::numeric / NULLIF(ev, 0)))::numeric, 4) volume_ratio_p95,
               count(*) FILTER (WHERE abs(vv::numeric / NULLIF(ev, 0) - 1) > 0.01) volume_diff_over_1pct
        FROM p""")[0],
    "close_difference_buckets": q(f"""
        WITH p AS (SELECT r.trade_date, abs(d.close / e.close - 1) diff FROM {V} r
                   JOIN ohlcv_daily d ON d.symbol_id = r.symbol_id AND (d.timestamp AT TIME ZONE 'Asia/Kolkata')::date = r.trade_date
                   JOIN exchange_daily_bars e ON e.symbol_id = r.symbol_id AND e.trade_date = r.trade_date
                   WHERE r.classification = 'NOMINAL_MATCH')
        SELECT CASE WHEN diff = 0 THEN 'exact' WHEN diff < 0.000001 THEN 'float storage noise (<0.0001%)'
                    WHEN diff < 0.0001 THEN '<0.01%' ELSE '>=0.01%' END AS bucket, count(*) n,
               string_agg(DISTINCT trade_date::text, ', ') FILTER (WHERE diff >= 0.000001) AS dates_with_real_differences
        FROM p GROUP BY 1 ORDER BY 2 DESC"""),
    "vendor_adjusted_series": q(f"""
        SELECT symbol, min(trade_date) first, max(trade_date) last, count(*) bars,
               round(min(close_ratio), 6) min_ratio, round(max(close_ratio), 6) max_ratio
        FROM {V} WHERE classification = 'VENDOR_ADJUSTED' GROUP BY symbol ORDER BY symbol"""),
}

R["unresolved"] = {
    "mismatch": q(f"""SELECT symbol, trade_date, vendor_close, exchange_close, close_ratio FROM {V}
                      WHERE classification = 'MISMATCH' ORDER BY 1, 2"""),
    "missing_vendor_bar_in_overlap": q(f"""SELECT symbol, count(*) n, min(trade_date) first, max(trade_date) last FROM {V}
                      WHERE classification = 'MISSING_VENDOR_BAR' AND trade_date >= '{VENDOR_FIRST}' GROUP BY 1 ORDER BY 2 DESC"""),
    "missing_exchange_bar": q(f"""SELECT symbol, trade_date, vendor_close FROM {V}
                      WHERE classification = 'MISSING_EXCHANGE_BAR' ORDER BY 1, 2"""),
}

R["eligibility"] = {
    "eligible_vs_exchange": q(f"""
        SELECT d.source, count(*) eligible_bars,
               count(*) FILTER (WHERE r.classification = 'NOMINAL_MATCH') exchange_nominal_match,
               count(*) FILTER (WHERE r.classification <> 'NOMINAL_MATCH') exchange_disagree,
               count(*) FILTER (WHERE d.adjusted_close = d.close) adjusted_equals_close,
               min((d.timestamp AT TIME ZONE 'Asia/Kolkata')::date) first_date
        FROM ohlcv_daily d JOIN {V} r ON r.symbol_id = d.symbol_id AND r.trade_date = (d.timestamp AT TIME ZONE 'Asia/Kolkata')::date
        WHERE d.adjusted_close IS NOT NULL GROUP BY 1"""),
    "quarantined": q("SELECT count(*) n FROM ohlcv_daily WHERE adjusted_close IS NULL")[0],
}

R["generated_at"] = datetime.now(timezone.utc).isoformat()
R["vendor_history_starts"] = VENDOR_FIRST
with open(OUT + ".json", "w") as f:
    json.dump(R, f, indent=2, default=str)


def table(rows, cols=None, limit=60):
    if not rows:
        return "_none_\n"
    cols = cols or list(rows[0])
    out = "| " + " | ".join(cols) + " |\n|" + "---|" * len(cols) + "\n"
    for r in rows[:limit]:
        out += "| " + " | ".join(str(r.get(c, "")) for c in cols) + " |\n"
    if len(rows) > limit:
        out += f"\n_{len(rows) - limit} more rows in the JSON file._\n"
    return out


c, m, o = R["coverage"], R["matched"], R["ohlcv_differences"]["on_nominal_matches_max_abs_pct"]
md = f"""# HalalEquity P1.9.24 — Vendor vs Exchange Daily Reconciliation

Generated {R['generated_at']} by `scripts/p1924_reconciliation_report.py` (read-only). Exchange record: NSE CM
bhavcopies; vendor record: `ohlcv_daily` (Yahoo). Data: `{OUT.split('/')[-1]}.json`.

## Coverage
- Exchange files: **{c['exchange_files']['n']}** ({c['exchange_files']['first']} → {c['exchange_files']['last']}); exchange bars: **{c['exchange_bars']['n']}** across {c['exchange_bars']['symbols']} instruments.
- Vendor bars: **{c['vendor_bars']['n']}** ({c['vendor_bars']['first']} → {c['vendor_bars']['last']}), {c['vendor_bars']['symbols']} instruments.
- **Vendor history begins {VENDOR_FIRST}.** Exchange evidence for 2022-01-03 → {VENDOR_FIRST} has no vendor counterpart to reconcile; it is authenticated *nominal* coverage only.
- Weekdays with vendor bars but no exchange file loaded: {len(c['missing_weekday_files'])}.

{table(c['exchange_files_by_year'])}
## Classification in the overlap ({VENDOR_FIRST} → {c['vendor_bars']['last']})
{table(R['classification_overlap'])}
- **Matched:** {m['nominal_match_within_5bps']} bars agree on open/high/low/close within 0.05%; {m['exact_close']} have an identical close.

## Duplicates
- Vendor bars sharing a symbol and IST date: **{len(R['duplicates']['vendor_same_symbol_same_ist_date'])}**.
- Exchange: {R['duplicates']['exchange']}.
{table(R['duplicates']['vendor_same_symbol_same_ist_date'])}
## Timestamps
Vendor timestamp convention (UTC time of day):
{table(R['timestamps']['vendor_not_session_open_label'])}
- Vendor bars on dates with no exchange session file: **{len(R['timestamps']['vendor_bars_on_dates_without_exchange_session'])}**
{table(R['timestamps']['vendor_bars_on_dates_without_exchange_session'])}
- Vendor bars whose close matches an adjacent session (possible date shift): **{len(R['timestamps']['vendor_bars_matching_an_adjacent_session'])}**
{table(R['timestamps']['vendor_bars_matching_an_adjacent_session'])}
## OHLCV differences on matched bars
{table([o])}
Close differences on matched bars:
{table(R['ohlcv_differences']['close_difference_buckets'])}
### Vendor back-adjusted series (constant ratio to the traded price)
{table(R['ohlcv_differences']['vendor_adjusted_series'])}
## Unresolved records
- Mismatches (prices disagree inconsistently): **{len(R['unresolved']['mismatch'])}**
{table(R['unresolved']['mismatch'])}
- Exchange bars with no vendor bar, inside the overlap:
{table(R['unresolved']['missing_vendor_bar_in_overlap'])}
- Vendor bars on an exchange session day that the exchange file does not list: **{len(R['unresolved']['missing_exchange_bar'])}**
{table(R['unresolved']['missing_exchange_bar'])}
## Eligibility check
Research-eligible vendor bars compared with the exchange record:
{table(R['eligibility']['eligible_vs_exchange'])}
- Quarantined bars: **{R['eligibility']['quarantined']['n']}** (unchanged; no record was released).
"""
with open(OUT + ".md", "w") as f:
    f.write(md)
print(f"wrote {OUT}.md and {OUT}.json")
