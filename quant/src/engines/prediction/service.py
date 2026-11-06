"""Train / generate / resolve: the operational surface of the intraday prediction engine."""
import hashlib
import io
import json
import math
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional

import joblib
import numpy as np
import pandas as pd
from psycopg2.extras import Json

from . import model as M
from . import walkforward as W
from .config import (
    CLASSES,
    FEATURES,
    HGB_PARAMS,
    HORIZONS,
    MAX_LATENCY_FRACTION_OF_HORIZON,
    MODEL_VERSION,
)
from .costs import cost_model_summary, round_trip_cost_fraction
from .data import IST, load_minute_bars, load_session_bars, session_close_ist, to_session_bars
from .features import add_labels, build_features


def _clean(o: Any) -> Any:
    """JSON-safe: numpy -> python, NaN/inf -> None (PostgreSQL JSONB rejects NaN)."""
    if isinstance(o, dict):
        return {str(k): _clean(v) for k, v in o.items()}
    if isinstance(o, (list, tuple)):
        return [_clean(v) for v in o]
    if isinstance(o, (np.integer,)):
        return int(o)
    if isinstance(o, (np.floating, float)):
        f = float(o)
        return None if math.isnan(f) or math.isinf(f) else f
    if isinstance(o, (np.bool_,)):
        return bool(o)
    return o


# ---------------------------------------------------------------- train

def train(conn, horizons: Optional[List[int]] = None) -> List[Dict[str, Any]]:
    cost = round_trip_cost_fraction()
    bars = load_session_bars(conn)  # live 1-minute bars plus the 5-minute history
    feats = build_features(bars) if not bars.empty else bars
    results = []
    for h_min in horizons or list(HORIZONS):
        h_bars = HORIZONS[h_min]
        ds = add_labels(feats, h_bars, cost) if not feats.empty else feats.assign(label=[], fwd=[])
        wf = W.run(ds, cost)
        metrics = dict(wf.metrics)
        artifact = sha = None

        if wf.verdict != "DATA_INSUFFICIENT":
            lab = ds[ds["label"] >= 0]
            y = lab["label"].to_numpy(int)
            final = M.fit(lab[FEATURES].to_numpy(float), y)
            cond = M.ConditionalReturns.from_oof(
                (wf.oof["p_up"] - wf.oof["p_down"]).to_numpy(), np.expm1(wf.oof["fwd"].to_numpy()))
            bundle = {"model": final, "features": FEATURES, "class_means": M.class_mean_returns(y, lab["fwd"].to_numpy(float)),
                      "conditional_returns": cond, "cost": cost, "h_bars": h_bars, "model_version": MODEL_VERSION}
            buf = io.BytesIO()
            joblib.dump(bundle, buf)
            artifact = buf.getvalue()
            sha = hashlib.sha256(artifact).hexdigest()
            last = wf.oof[wf.oof["date"] == wf.oof["date"].max()]
            metrics["example_signals_last_test_day"] = [
                {"symbol": r.symbol, "as_of": r.bar_end.isoformat(), "p_up": r.p_up, "p_down": r.p_down,
                 "expected_return": r.exp_ret, "realized_return": float(np.expm1(r.fwd)), "signalled": bool(r.signal)}
                for r in last.sort_values("exp_ret", ascending=False).head(10).itertuples()
            ]

        with conn.cursor() as cur:
            if artifact is not None:
                cur.execute("UPDATE prediction_model_runs SET is_active = false WHERE horizon_minutes = %s AND is_active", (h_min,))
            cur.execute(
                """INSERT INTO prediction_model_runs (model_version, horizon_minutes, data_from, data_to, n_symbols, verdict,
                       metrics, params, cost_model, artifact, artifact_sha256, is_active)
                   VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s) RETURNING id""",
                (MODEL_VERSION, h_min,
                 bars["date"].min() if not bars.empty else None, bars["date"].max() if not bars.empty else None,
                 int(bars["symbol_id"].nunique()) if not bars.empty else 0, wf.verdict,
                 Json(_clean(metrics)), Json(_clean({"hgb": HGB_PARAMS, "features": FEATURES, "h_bars": h_bars})),
                 Json(_clean(cost_model_summary())), artifact, sha, artifact is not None))
            run_id = cur.fetchone()[0]
        conn.commit()
        results.append({"horizon_minutes": h_min, "run_id": run_id, "verdict": wf.verdict,
                        "edge_checks": metrics.get("edge_checks"), "reason": metrics.get("reason")})
    return results


# ---------------------------------------------------------------- generate

def _load_active(conn) -> Dict[int, Dict[str, Any]]:
    with conn.cursor() as cur:
        cur.execute("""SELECT id, horizon_minutes, verdict, artifact, artifact_sha256
                       FROM prediction_model_runs WHERE is_active AND artifact IS NOT NULL""")
        rows = cur.fetchall()
    out = {}
    for run_id, h, verdict, art, sha in rows:
        art = bytes(art)
        if hashlib.sha256(art).hexdigest() != sha:
            raise RuntimeError(f"model run {run_id} artifact failed its integrity check")
        out[h] = {"run_id": run_id, "verdict": verdict, "bundle": joblib.load(io.BytesIO(art))}
    return out


def _shariah_by_symbol(conn) -> Dict[int, str]:
    with conn.cursor() as cur:
        cur.execute("""SELECT DISTINCT ON (symbol_id) symbol_id, status::text FROM shariah_screenings
                       ORDER BY symbol_id, screening_date DESC, id DESC""")
        return {sid: st for sid, st in cur.fetchall()}


def decide_action(p: np.ndarray, exp_ret: float, cost: float, verdict: str, latency_s: float,
                  horizon_min: int, shariah: str) -> (str, List[Dict[str, str]]):
    """Action plus every reason that shaped it. The model's raw view is always recorded."""
    reasons: List[Dict[str, str]] = []
    wants_buy = exp_ret > cost and p[2] > p[0]
    wants_exit = exp_ret < -cost and p[0] > p[2]
    reasons.append({"code": "MODEL_VIEW", "detail": "BUY" if wants_buy else "EXIT_OR_AVOID" if wants_exit else "NO_EDGE_AFTER_COSTS"})

    blocked = False
    if verdict != "ACTIONABLE":
        reasons.append({"code": "NO_DEMONSTRATED_EDGE",
                        "detail": "walk-forward testing has not shown a positive, statistically reliable edge after costs for this horizon"})
        blocked = True
    if latency_s > MAX_LATENCY_FRACTION_OF_HORIZON * horizon_min * 60:
        reasons.append({"code": "DATA_TOO_STALE",
                        "detail": f"forecast used data {int(latency_s // 60)} min old; too stale for a {horizon_min}-minute horizon"})
        blocked = True
    if wants_buy and shariah == "FAIL":
        reasons.append({"code": "SHARIAH_FAIL", "detail": "instrument failed Shariah screening; never a buy candidate"})
        blocked = True

    if blocked:
        return "NO_TRADE", reasons
    if wants_buy:
        if shariah != "PASS":
            reasons.append({"code": "SHARIAH_UNEVALUATED", "detail": "Shariah status is not certified PASS: verify before buying"})
        return "BUY_CANDIDATE", reasons
    if wants_exit:
        return "EXIT_OR_AVOID", reasons
    return "NO_TRADE", reasons


def generate(conn, now: Optional[datetime] = None) -> Dict[str, Any]:
    now = now or datetime.now(timezone.utc)
    active = _load_active(conn)
    if not active:
        return {"issued": 0, "reason": "no trained model with an artifact is active"}

    with conn.cursor() as cur:
        cur.execute("SELECT max(timestamp) FROM ohlcv_intraday WHERE quality_status = 'VALID' AND interval_minutes = 1")
        latest = cur.fetchone()[0]
    if latest is None:
        return {"issued": 0, "reason": "no intraday data"}
    session_day = latest.astimezone(IST).date()
    bars = to_session_bars(load_minute_bars(conn, since=session_day, until=session_day))
    if bars.empty:
        return {"issued": 0, "reason": "no bars in the latest session"}
    feats = build_features(bars)
    # The provider returns the still-forming current minute. Treat the newest minute as incomplete so a 5-minute bar
    # is never assembled from a partial minute (training only ever saw finished minutes).
    latest_end = pd.Timestamp(latest).tz_convert(IST)
    complete = feats[feats["bar_end"] <= latest_end]
    last = complete.sort_values("bar_start").groupby("symbol_id").tail(1)
    shariah = _shariah_by_symbol(conn)
    close_t = session_close_ist(session_day)

    issued, skipped = 0, {}
    with conn.cursor() as cur:
        for h_min, run in active.items():
            b = run["bundle"]
            rows = last[last["bar_end"] + pd.Timedelta(minutes=h_min) <= close_t]
            if rows.empty:
                skipped[h_min] = "horizon would run past the session close"
                continue
            # A forecast must be issued before its target time; anything else is hindsight and is never stored.
            rows = rows[rows["bar_end"] + pd.Timedelta(minutes=h_min) > pd.Timestamp(now)]
            if rows.empty:
                skipped[h_min] = "target time already passed: data too old to issue a genuine forecast"
                continue
            P = M.predict_full(b["model"], rows[b["features"]].to_numpy(float))
            exp = P @ b["class_means"]
            for (_, r), p, e in zip(rows.iterrows(), P, exp):
                as_of = r["bar_end"].to_pydatetime()
                latency = (now - as_of).total_seconds()
                sh = shariah.get(int(r["symbol_id"]), "UNEVALUATED")
                action, reasons = decide_action(p, float(e), b["cost"], run["verdict"], latency, h_min, sh)
                q = b["conditional_returns"].lookup(float(p[2] - p[0]))
                entry = float(r["close"])
                cur.execute(
                    """INSERT INTO predictions (model_run_id, symbol_id, horizon_minutes, as_of, target_time, issued_at,
                           reference_price, p_down, p_neutral, p_up, expected_return, return_q10, return_q50, return_q90,
                           action, action_reasons, entry_price, target_price, stop_price, shariah_status)
                       VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
                       ON CONFLICT (model_run_id, symbol_id, as_of) DO NOTHING""",
                    (run["run_id"], int(r["symbol_id"]), h_min, as_of, as_of + timedelta(minutes=h_min), now,
                     entry, round(float(p[0]), 6), round(float(p[1]), 6), round(1 - round(float(p[0]), 6) - round(float(p[1]), 6), 6),
                     float(np.expm1(e)), q["q10"], q["q50"], q["q90"], action, Json(reasons),
                     entry if action == "BUY_CANDIDATE" else None,
                     round(entry * (1 + q["q75"]), 2) if action == "BUY_CANDIDATE" else None,
                     round(entry * (1 + q["q10"]), 2) if action == "BUY_CANDIDATE" else None,
                     sh))
                issued += cur.rowcount
    conn.commit()
    return {"issued": issued, "session": str(session_day), "latest_bar": str(latest), "skipped": skipped}


# ---------------------------------------------------------------- resolve

def resolve(conn, now: Optional[datetime] = None, grace: timedelta = timedelta(minutes=2)) -> Dict[str, int]:
    """Record what actually happened once each forecast's horizon has elapsed (forward tracking)."""
    now = now or datetime.now(timezone.utc)
    cost = round_trip_cost_fraction()
    done = unresolvable = 0
    with conn.cursor() as cur:
        cur.execute("""SELECT id, symbol_id, target_time, reference_price FROM predictions
                       WHERE resolved_at IS NULL AND target_time <= %s ORDER BY target_time LIMIT 5000""", (now - grace,))
        pending = cur.fetchall()
        for pid, sid, target, ref in pending:
            # close of the last VALID minute inside the 5-minute bar that ends at target_time
            cur.execute("""SELECT close FROM ohlcv_intraday WHERE symbol_id = %s AND quality_status = 'VALID'
                             AND timestamp < %s AND timestamp >= %s ORDER BY timestamp DESC LIMIT 1""",
                        (sid, target, target - timedelta(minutes=5)))
            row = cur.fetchone()
            if row is None:
                if now - target > timedelta(days=2):  # the data is never coming: close the record honestly
                    cur.execute("UPDATE predictions SET resolved_at = %s WHERE id = %s", (now, pid))
                    unresolvable += 1
                continue
            realized = float(row[0])
            lr = math.log(realized / float(ref))
            cls = "UP" if lr > cost else "DOWN" if lr < -cost else "NEUTRAL"
            cur.execute("""UPDATE predictions SET realized_price = %s, realized_return = %s, realized_class = %s,
                           resolved_at = %s WHERE id = %s""", (realized, math.expm1(lr), cls, now, pid))
            done += 1
    conn.commit()
    return {"resolved": done, "unresolvable": unresolvable, "pending_checked": len(pending)}
