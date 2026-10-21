"""
Prospective paper-observation engine (P1.7 rewrite).

Contract
--------
* A candidate is frozen at decision time (tz-aware). Its decision fields and a SHA-256 snapshot hash never change.
* `evaluate()` is a PURE, deterministic recomputation from the stored decision + the daily bars strictly AFTER the
  decision session. It never appends to prior state, so re-running it cannot duplicate or drift; a terminal record
  is returned untouched.
* Entry is NOT assumed. The entry rule is a buy-stop: filled on the first post-decision session whose high >= trigger,
  at max(open, trigger) (gap-up fills at the open). No fill within the window -> NOT_TRIGGERED (no trade, no return).
* Daily bars cannot order intraday events: if stop and target are both reachable on the same session (including the
  entry session) the stop wins. Gaps through a level fill at the open.
* A missing or invalid bar for a session that should already have data -> DATA_UNAVAILABLE (terminal, excluded from
  performance, counted separately). A caller-supplied terminal reason (delisting/suspension) -> CENSORED.
  Not-yet-available sessions leave the record PENDING.
* Costs come exclusively from the itemized CostEngine ledger at a fixed notional. No flat-bps constant exists.
* Unavailable metrics are None, never 0.0.
"""
import hashlib
import json
from dataclasses import dataclass, field, asdict, replace
from datetime import date, datetime, timedelta, timezone
from typing import Any, Dict, List, Optional, Tuple
from zoneinfo import ZoneInfo

from src.engines.research.cost_model import CostEngine, MODEL_VERSION
from src.engines.research.data_audit import expected_sessions, load_calendar

IST = ZoneInfo("Asia/Kolkata")
WINDOW_SESSIONS = 10
HORIZONS = (1, 3, 5, 10)
DEFAULT_NOTIONAL_INR = 100_000.0


class SetupType:
    APPROACHING_RESISTANCE = "APPROACHING_RESISTANCE"
    CONSOLIDATION = "CONSOLIDATION"
    CONFIRMED_BREAKOUT = "CONFIRMED_BREAKOUT"
    FAILED_BREAKOUT = "FAILED_BREAKOUT"


class Status:
    PENDING = "PENDING"
    COMPLETED = "COMPLETED"
    CENSORED = "CENSORED"
    INVALIDATED = "INVALIDATED"
    DATA_UNAVAILABLE = "DATA_UNAVAILABLE"
    TERMINAL = {COMPLETED, CENSORED, INVALIDATED, DATA_UNAVAILABLE}


class PaperOutcome:
    PENDING = "PENDING"
    TARGET_HIT = "TARGET_HIT"
    STOPPED_OUT = "STOPPED_OUT"
    EXPIRED_TIME_STOP = "EXPIRED_TIME_STOP"
    NOT_TRIGGERED = "NOT_TRIGGERED"
    CENSORED = "CENSORED"
    DATA_UNAVAILABLE = "DATA_UNAVAILABLE"
    INVALIDATED = "INVALIDATED"


@dataclass(frozen=True)
class Bar:
    session: date   # IST session date
    open: float
    high: float
    low: float
    close: float

    def valid(self) -> bool:
        return (self.open > 0 and self.low > 0 and self.high >= self.low and self.high >= max(self.open, self.close)
                and self.low <= min(self.open, self.close))


@dataclass
class PaperObservationRecord:
    # ---- immutable decision-time snapshot ----
    symbol: str
    setup_type: str
    candidate_timestamp: datetime
    signal_timestamp: datetime
    data_cutoff_at: datetime
    provider: str
    rule_version: str
    entry_trigger_price: float
    invalidation_price: float
    target_price: float
    risk_reward_ratio: float
    shariah_status: str
    sector: str
    liquidity_bucket: str
    market_regime: str
    estimated_costs_bps: float
    calendar_version: str
    cost_model_version: str
    notional_inr: float
    snapshot_sha256: str = ""
    # ---- evaluation state (written once, at the terminal transition, except horizons which are write-once per key) ----
    status: str = Status.PENDING
    outcome: str = PaperOutcome.PENDING
    status_reason: Optional[str] = None
    entry_fill_price: Optional[float] = None
    entry_session: Optional[date] = None
    exit_price: Optional[float] = None
    exit_session: Optional[date] = None
    horizons: Dict[str, Dict[str, Any]] = field(default_factory=lambda: {f"{k}d": {"status": "PENDING", "close": None, "gross_return": None} for k in HORIZONS})
    sessions_observed: List[Dict[str, Any]] = field(default_factory=list)
    realized_gross_return: Optional[float] = None
    realized_net_return: Optional[float] = None
    itemized_costs: Optional[Dict[str, Any]] = None
    evaluated_through_session: Optional[date] = None
    observation_id: Optional[int] = None

    @property
    def is_complete(self) -> bool:
        return self.status in Status.TERMINAL

    @property
    def decision_session(self) -> date:
        return self.candidate_timestamp.astimezone(IST).date()

    def to_dict(self) -> Dict[str, Any]:
        return asdict(self)


SNAPSHOT_FIELDS = ("symbol", "setup_type", "candidate_timestamp", "signal_timestamp", "data_cutoff_at", "provider", "rule_version",
                   "entry_trigger_price", "invalidation_price", "target_price", "risk_reward_ratio", "shariah_status", "sector",
                   "liquidity_bucket", "market_regime", "estimated_costs_bps", "calendar_version", "cost_model_version", "notional_inr")


def snapshot_dict(r: PaperObservationRecord) -> Dict[str, Any]:
    out = {}
    for k in SNAPSHOT_FIELDS:
        v = getattr(r, k)
        out[k] = v.astimezone(timezone.utc).isoformat() if isinstance(v, datetime) else v
    return out


def snapshot_hash(r: PaperObservationRecord) -> str:
    return hashlib.sha256(json.dumps(snapshot_dict(r), sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def _require_aware(name: str, ts: datetime) -> datetime:
    if ts.tzinfo is None or ts.utcoffset() is None:
        raise ValueError(f"{name} must be timezone-aware")
    return ts


def evaluate(record: PaperObservationRecord, bars: List[Bar], data_through: date,
             terminal_reason: Optional[str] = None, cost_engine: Optional[CostEngine] = None) -> PaperObservationRecord:
    """Pure function. Returns the record unchanged if already terminal, otherwise a NEW recomputed record.

    bars: daily bars for this symbol (any range). data_through: the last session for which the daily table is
    known to be complete (bars after it are not yet expected).
    """
    if record.is_complete:
        return record
    ce = cost_engine or CostEngine()
    cal = load_calendar()
    dsess = record.decision_session
    # Only sessions strictly after the decision session are ever consulted (no look-back into the decision day's own bar).
    by_date = {b.session: b for b in bars}
    window = expected_sessions(dsess + timedelta(days=1), dsess + timedelta(days=40), cal)[:WINDOW_SESSIONS]

    rec = replace(record, horizons={f"{k}d": {"status": "PENDING", "close": None, "gross_return": None} for k in HORIZONS},
                  sessions_observed=[], entry_fill_price=None, entry_session=None, exit_price=None, exit_session=None)
    entry = None
    exit_info: Optional[Tuple[float, str, int]] = None
    trig, stop, tgt = rec.entry_trigger_price, rec.invalidation_price, rec.target_price
    last_idx = 0

    for idx, s in enumerate(window, start=1):
        if s > data_through:
            break
        b = by_date.get(s)
        if b is None or not b.valid():
            if terminal_reason:
                return _terminal(rec, Status.CENSORED, PaperOutcome.CENSORED, terminal_reason, s)
            return _terminal(rec, Status.DATA_UNAVAILABLE, PaperOutcome.DATA_UNAVAILABLE,
                             f"no valid daily bar for expected session {s.isoformat()} (session #{idx} after decision)", s)
        last_idx = idx
        rec.sessions_observed.append({"n": idx, "session": s.isoformat(), "open": b.open, "high": b.high, "low": b.low, "close": b.close})
        rec.evaluated_through_session = s

        entered_today = False
        if entry is None:
            if b.high >= trig:
                fill = max(b.open, trig)
                entry = (fill, s, idx)
                entered_today = True
                rec.entry_fill_price, rec.entry_session = round(fill, 4), s
        if entry is not None and exit_info is None:
            fill = entry[0]
            gap_fill = entered_today and b.open >= trig
            stop_touch = b.low <= stop
            tgt_touch = b.high >= tgt
            if not entered_today and b.open <= stop:
                exit_info = (b.open, PaperOutcome.STOPPED_OUT, idx)           # gap down through stop
            elif not entered_today and b.open >= tgt:
                exit_info = (b.open, PaperOutcome.TARGET_HIT, idx)            # gap up through target
            elif gap_fill and b.open >= tgt and not stop_touch:
                exit_info = (b.open, PaperOutcome.TARGET_HIT, idx)
            elif stop_touch:                                                  # includes both-touched ambiguity -> stop wins
                exit_info = (stop if b.open > stop else b.open, PaperOutcome.STOPPED_OUT, idx)
            elif tgt_touch:
                exit_info = (tgt, PaperOutcome.TARGET_HIT, idx)
            elif idx == WINDOW_SESSIONS:
                exit_info = (b.close, PaperOutcome.EXPIRED_TIME_STOP, idx)
            if exit_info:
                rec.exit_price, rec.exit_session = round(exit_info[0], 4), s

        for k in HORIZONS:
            if idx == k:
                key = f"{k}d"
                if entry is None:
                    rec.horizons[key] = {"status": "NOT_ENTERED", "close": None, "gross_return": None}
                elif exit_info is not None and exit_info[2] <= k:
                    rec.horizons[key] = {"status": "CLOSED_EARLY", "close": None, "gross_return": None}
                else:
                    rec.horizons[key] = {"status": "OBSERVED", "close": b.close, "gross_return": round(b.close / entry[0] - 1.0, 6)}

        if exit_info:
            break

    if exit_info:
        return _finish_trade(rec, entry, exit_info, ce)
    if last_idx == WINDOW_SESSIONS:
        if entry is None:
            rec.horizons = {k: (v if v["status"] != "PENDING" else {"status": "NOT_ENTERED", "close": None, "gross_return": None}) for k, v in rec.horizons.items()}
            return _terminal(rec, Status.COMPLETED, PaperOutcome.NOT_TRIGGERED,
                             "entry trigger never reached within the 10-session window; no trade", rec.evaluated_through_session)
    if terminal_reason and last_idx < WINDOW_SESSIONS:
        return _terminal(rec, Status.CENSORED, PaperOutcome.CENSORED, terminal_reason, rec.evaluated_through_session)
    return rec  # still PENDING


def _terminal(rec, status, outcome, reason, session):
    rec.status, rec.outcome, rec.status_reason = status, outcome, reason
    rec.evaluated_through_session = session or rec.evaluated_through_session
    rec.realized_gross_return = rec.realized_net_return = None
    return rec


def _finish_trade(rec, entry, exit_info, ce: CostEngine):
    fill, _, _ = entry
    exit_p, outcome, idx = exit_info
    qty = max(1, int(rec.notional_inr // fill))
    calc = ce.calculate_trade(rec.symbol, fill, exit_p, quantity=qty, holding_days=idx)
    rec.status, rec.outcome = Status.COMPLETED, outcome
    rec.status_reason = None
    rec.realized_gross_return = round(exit_p / fill - 1.0, 6)
    rec.realized_net_return = round(calc.net_pnl / (qty * fill), 6)
    rec.itemized_costs = calc.to_dict()
    # horizons not yet resolved after an exit are closed
    for k in HORIZONS:
        key = f"{k}d"
        if rec.horizons[key]["status"] == "PENDING":
            rec.horizons[key] = {"status": "CLOSED_EARLY", "close": None, "gross_return": None}
    return rec


class PaperObservationEngine:
    def __init__(self, records: Optional[List[PaperObservationRecord]] = None, cost_engine: Optional[CostEngine] = None,
                 notional_inr: float = DEFAULT_NOTIONAL_INR):
        self.records: List[PaperObservationRecord] = records or []
        self.cost_engine = cost_engine or CostEngine()
        self.notional_inr = notional_inr

    def record_candidate(self, symbol: str, setup_type: str, entry_trigger: float, invalidation_price: float, target_price: float,
                         shariah_status: str, data_cutoff_at: datetime, rule_version: str, provider: str = "yahoo_finance",
                         sector: str = "Unknown", liquidity_bucket: str = "LARGE_CAP", market_regime: str = "RANGING",
                         candidate_timestamp: Optional[datetime] = None) -> PaperObservationRecord:
        if shariah_status.upper() != "PASS":
            raise ValueError(f"Shariah hard gate violated: status '{shariah_status}' cannot be recorded as paper candidate")
        if not (invalidation_price < entry_trigger < target_price):
            raise ValueError("require invalidation < entry < target")
        ts = _require_aware("candidate_timestamp", candidate_timestamp or datetime.now(timezone.utc))
        _require_aware("data_cutoff_at", data_cutoff_at)
        if data_cutoff_at > ts:
            raise ValueError("data_cutoff_at must not be after the decision timestamp (future leakage)")
        rr = round((target_price - entry_trigger) / (entry_trigger - invalidation_price), 4)
        cal = load_calendar()
        est = self.cost_engine.calculate_trade(symbol, entry_trigger, target_price, quantity=max(1, int(self.notional_inr // entry_trigger)))
        rec = PaperObservationRecord(
            symbol=symbol, setup_type=setup_type, candidate_timestamp=ts, signal_timestamp=ts, data_cutoff_at=data_cutoff_at,
            provider=provider, rule_version=rule_version, entry_trigger_price=round(entry_trigger, 4),
            invalidation_price=round(invalidation_price, 4), target_price=round(target_price, 4), risk_reward_ratio=rr,
            shariah_status="PASS", sector=sector, liquidity_bucket=liquidity_bucket, market_regime=market_regime,
            estimated_costs_bps=round(est.effective_cost_bps, 2), calendar_version=cal["calendar_version"],
            cost_model_version=self.cost_engine.config.version, notional_inr=self.notional_inr,
        )
        rec.snapshot_sha256 = snapshot_hash(rec)
        self.records.append(rec)
        return rec

    def evaluate_record(self, i: int, bars: List[Bar], data_through: date, terminal_reason: Optional[str] = None):
        before = self.records[i]
        after = evaluate(before, bars, data_through, terminal_reason, self.cost_engine)
        assert snapshot_hash(after) == before.snapshot_sha256, "decision snapshot mutated"
        self.records[i] = after
        return after

    def summarize_results(self) -> Dict[str, Any]:
        return summarize(self.records)


def independent_subset(records: List[PaperObservationRecord]) -> List[PaperObservationRecord]:
    """Greedy earliest-first: keep an observation only if its 10-session window does not overlap an already-kept one
    for the same symbol. (Cross-symbol correlation is NOT removed; see pre-registered spec.)"""
    kept: List[PaperObservationRecord] = []
    last_end: Dict[str, date] = {}
    for r in sorted(records, key=lambda x: x.candidate_timestamp):
        end = r.decision_session + timedelta(days=16)  # 10 sessions <= 16 calendar days upper bound
        if r.symbol in last_end and r.decision_session <= last_end[r.symbol]:
            continue
        kept.append(r)
        last_end[r.symbol] = end
    return kept


def _stats(vals: List[float]) -> Dict[str, Any]:
    n = len(vals)
    if n == 0:
        return {"n": 0, "mean_pct": None, "win_rate_pct": None}
    return {"n": n, "mean_pct": round(sum(vals) / n * 100, 4), "win_rate_pct": round(sum(1 for v in vals if v > 0) / n * 100, 2)}


def summarize(records: List[PaperObservationRecord]) -> Dict[str, Any]:
    by_status: Dict[str, int] = {}
    for r in records:
        by_status[r.status] = by_status.get(r.status, 0) + 1
    trades = [r for r in records if r.status == Status.COMPLETED and r.outcome in
              (PaperOutcome.TARGET_HIT, PaperOutcome.STOPPED_OUT, PaperOutcome.EXPIRED_TIME_STOP)]
    indep = independent_subset(trades)
    horizon = {}
    for k in HORIZONS:
        key = f"{k}d"
        obs = [r.horizons[key]["gross_return"] for r in records if r.status != Status.INVALIDATED
               and r.horizons.get(key, {}).get("status") == "OBSERVED"]
        pend = sum(1 for r in records if r.status == Status.PENDING and r.horizons.get(key, {}).get("status") == "PENDING")
        horizon[key] = {**_stats(obs), "pending": pend,
                        "closed_early": sum(1 for r in records if r.horizons.get(key, {}).get("status") == "CLOSED_EARLY"),
                        "not_entered": sum(1 for r in records if r.horizons.get(key, {}).get("status") == "NOT_ENTERED"),
                        "metric_available": len(obs) > 0}
    gross = [r.realized_gross_return for r in trades]
    net = [r.realized_net_return for r in trades]

    def group(attr):
        out: Dict[str, Any] = {}
        for g in sorted({getattr(r, attr) for r in trades}):
            out[g] = {"completed_trades": sum(1 for r in trades if getattr(r, attr) == g),
                      "independent": sum(1 for r in indep if getattr(r, attr) == g),
                      **{f"net_{k}": v for k, v in _stats([r.realized_net_return for r in trades if getattr(r, attr) == g]).items()}}
        return out

    return {
        "counts": {"candidates_total": len(records), "by_status": by_status,
                   "valid_candidates": len([r for r in records if r.status != Status.INVALIDATED]),
                   "completed_trades": len(trades), "independent_trades": len(indep),
                   "not_triggered": sum(1 for r in records if r.outcome == PaperOutcome.NOT_TRIGGERED)},
        "gross": _stats(gross), "net": _stats(net),
        "horizons": horizon,
        "by_setup_type": group("setup_type"), "by_market_regime": group("market_regime"),
        "cost_model_version": MODEL_VERSION,
        "significance_gate": {"min_independent_trades_operational_gate": 100, "independent_trades": len(indep),
                              "gate_met": len(indep) >= 100,
                              "note": "Meeting the count gate is necessary, NOT evidence of significance."},
    }
