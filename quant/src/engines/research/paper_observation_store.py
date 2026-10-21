"""PostgreSQL persistence for prospective paper observations (P1.7).

All state transitions go through one UPDATE guarded by `WHERE status='PENDING'` plus the po_guard trigger
(migration 05), and workers lock rows with FOR UPDATE SKIP LOCKED so concurrent runs cannot double-process.
"""
import json
from datetime import date, datetime, timezone
from typing import Any, Dict, List, Optional, Tuple

from src.engines.research.paper_observation import (
    Bar, PaperObservationRecord, Status, evaluate, snapshot_dict,
)
from src.engines.research.cost_model import CostEngine


def insert_candidate(conn, symbol_id: int, rec: PaperObservationRecord) -> Tuple[int, bool]:
    """Returns (id, created). A duplicate (symbol_id, candidate_timestamp, rule_version) returns the existing id and
    does NOT touch the stored row (not even updated_at)."""
    snap = snapshot_dict(rec)
    snap_json = json.dumps(snap, sort_keys=True)
    cur = conn.cursor()
    cur.execute("""
        INSERT INTO paper_observations (symbol_id, symbol, setup_type, candidate_timestamp, signal_timestamp, data_cutoff_at, provider,
            rule_version, entry_trigger_price, invalidation_price, target_price, risk_reward_ratio, shariah_status, sector,
            liquidity_bucket, market_regime, estimated_costs_bps, calendar_version, cost_model_version, notional_inr,
            decision_snapshot, decision_snapshot_sha256, subsequent_observations, predetermined_horizons)
        VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s::jsonb, encode(sha256(convert_to(%s::jsonb::text,'UTF8')),'hex'),'[]'::jsonb,%s::jsonb)
        ON CONFLICT DO NOTHING
        RETURNING id""",
        (symbol_id, rec.symbol, rec.setup_type, rec.candidate_timestamp, rec.signal_timestamp, rec.data_cutoff_at, rec.provider,
         rec.rule_version, rec.entry_trigger_price, rec.invalidation_price, rec.target_price, rec.risk_reward_ratio, rec.shariah_status,
         rec.sector, rec.liquidity_bucket, rec.market_regime, rec.estimated_costs_bps, rec.calendar_version, rec.cost_model_version,
         rec.notional_inr, snap_json, snap_json, json.dumps(rec.horizons)))
    row = cur.fetchone()
    if row:
        return row[0], True
    cur.execute("""SELECT id FROM paper_observations WHERE symbol_id=%s AND rule_version=%s AND (candidate_timestamp=%s OR
                   (setup_type=%s AND (candidate_timestamp AT TIME ZONE 'Asia/Kolkata')::date = (%s AT TIME ZONE 'Asia/Kolkata')::date))
                   ORDER BY id LIMIT 1""", (symbol_id, rec.rule_version, rec.candidate_timestamp, rec.setup_type, rec.candidate_timestamp))
    return cur.fetchone()[0], False


def _row_to_record(r: Dict[str, Any]) -> PaperObservationRecord:
    rec = PaperObservationRecord(
        symbol=r["symbol"], setup_type=r["setup_type"], candidate_timestamp=r["candidate_timestamp"],
        signal_timestamp=r["signal_timestamp"], data_cutoff_at=r["data_cutoff_at"], provider=r["provider"],
        rule_version=r["rule_version"], entry_trigger_price=float(r["entry_trigger_price"]),
        invalidation_price=float(r["invalidation_price"]), target_price=float(r["target_price"]),
        risk_reward_ratio=float(r["risk_reward_ratio"]), shariah_status=r["shariah_status"], sector=r["sector"] or "Unknown",
        liquidity_bucket=r["liquidity_bucket"] or "", market_regime=r["market_regime"] or "",
        estimated_costs_bps=None if r["estimated_costs_bps"] is None else float(r["estimated_costs_bps"]), calendar_version=r["calendar_version"] or "",
        cost_model_version=r["cost_model_version"] or "", notional_inr=float(r["notional_inr"] or 100000),
        snapshot_sha256=r["decision_snapshot_sha256"], status=r["status"], outcome=r["outcome"], observation_id=r["id"])
    return rec


def _dict_rows(cur):
    cols = [c[0] for c in cur.description]
    return [dict(zip(cols, row)) for row in cur.fetchall()]


def data_through(conn) -> Optional[date]:
    """Last session for which the daily table is considered complete: >=80% of the median recent per-session symbol count."""
    cur = conn.cursor()
    cur.execute("""
        WITH c AS (SELECT (timestamp AT TIME ZONE 'Asia/Kolkata')::date d, count(*) n FROM ohlcv_daily GROUP BY 1 ORDER BY 1 DESC LIMIT 30),
             m AS (SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY n) med FROM c)
        SELECT max(d) FROM c, m WHERE n >= 0.8 * med""")
    r = cur.fetchone()
    return r[0] if r else None


def load_bars(conn, symbol_id: int, after: date) -> List[Bar]:
    cur = conn.cursor()
    cur.execute("""SELECT (timestamp AT TIME ZONE 'Asia/Kolkata')::date, open, high, low, close FROM ohlcv_daily
                   WHERE symbol_id=%s AND (timestamp AT TIME ZONE 'Asia/Kolkata')::date > %s ORDER BY 1""", (symbol_id, after))
    return [Bar(d, float(o), float(h), float(l), float(c)) for d, o, h, l, c in cur.fetchall()]


def evaluate_pending(conn, cost_engine: Optional[CostEngine] = None, limit: int = 500) -> Dict[str, int]:
    """One transaction per observation. Safe to run concurrently and repeatedly."""
    through = data_through(conn)
    summary = {"examined": 0, "updated": 0, "still_pending": 0, "skipped_locked_or_terminal": 0}
    if through is None:
        return summary
    cur = conn.cursor()
    cur.execute("SELECT id FROM paper_observations WHERE status='PENDING' ORDER BY id LIMIT %s", (limit,))
    ids = [r[0] for r in cur.fetchall()]
    conn.commit()
    for oid in ids:
        cur = conn.cursor()
        cur.execute("SELECT * FROM paper_observations WHERE id=%s AND status='PENDING' FOR UPDATE SKIP LOCKED", (oid,))
        rows = _dict_rows(cur)
        if not rows:
            summary["skipped_locked_or_terminal"] += 1
            conn.commit()
            continue
        row = rows[0]
        summary["examined"] += 1
        rec = _row_to_record(row)
        new = evaluate(rec, load_bars(conn, row["symbol_id"], rec.decision_session), through, cost_engine=cost_engine)
        # write-once horizons: a previously OBSERVED horizon is never overwritten; a differing recomputation is a data revision
        stored_h = row["predetermined_horizons"] or {}
        for key, old in stored_h.items():
            if isinstance(old, dict) and old.get("status") == "OBSERVED":
                if new.horizons.get(key) != old:
                    summary["revision_conflicts"] = summary.get("revision_conflicts", 0) + 1
                new.horizons[key] = old
        if new.status == Status.PENDING and new.evaluated_through_session == row["evaluated_through_session"]:
            summary["still_pending"] += 1   # nothing new since last evaluation: no write at all
            conn.commit()
            continue
        cur.execute("""UPDATE paper_observations SET status=%s, outcome=%s, status_reason=%s, is_complete=%s,
                completed_at=CASE WHEN %s THEN NOW() ELSE NULL END, entry_fill_price=%s, entry_session_date=%s, exit_price=%s,
                exit_session_date=%s, realized_gross_return=%s, realized_net_return=%s, itemized_costs=%s::jsonb,
                predetermined_horizons=%s::jsonb, subsequent_observations=%s::jsonb, evaluated_through_session=%s
                WHERE id=%s AND status='PENDING'""",
            (new.status, new.outcome, new.status_reason, new.is_complete, new.is_complete, new.entry_fill_price, new.entry_session,
             new.exit_price, new.exit_session, new.realized_gross_return, new.realized_net_return,
             json.dumps(new.itemized_costs) if new.itemized_costs else None, json.dumps(new.horizons),
             json.dumps(new.sessions_observed), new.evaluated_through_session, oid))
        summary["updated" if cur.rowcount else "skipped_locked_or_terminal"] += 1
        if new.status == Status.PENDING:
            summary["still_pending"] += 1
        conn.commit()
    return summary


def load_records(conn) -> List[PaperObservationRecord]:
    """All stored observations as full records (for summaries). Read-only."""
    cur = conn.cursor()
    cur.execute("SELECT * FROM paper_observations ORDER BY id")
    out = []
    for row in _dict_rows(cur):
        rec = _row_to_record(row)
        h = row["predetermined_horizons"] or {}
        for k in ("1d", "3d", "5d", "10d"):
            v = h.get(k)
            rec.horizons[k] = v if isinstance(v, dict) else {"status": "PENDING", "close": None, "gross_return": None}
        rec.realized_gross_return = None if row["realized_gross_return"] is None else float(row["realized_gross_return"])
        rec.realized_net_return = None if row["realized_net_return"] is None else float(row["realized_net_return"])
        rec.status_reason = row["status_reason"]
        out.append(rec)
    return out
