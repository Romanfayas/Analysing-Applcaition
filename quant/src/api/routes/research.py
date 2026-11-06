from fastapi import APIRouter, HTTPException, Query
from pydantic import BaseModel
from typing import Dict, Any, List, Optional
from datetime import datetime, timezone

from src.engines.research.universe import UniverseManager, UniverseTier
from src.engines.research.provider_readiness import ProviderAuditor
from src.engines.research.paper_observation import (
    PaperObservationEngine,
    PaperObservationRecord,
    PaperOutcome,
    SetupType,
)

router = APIRouter()

# Global in-memory paper observation engine instance
paper_engine = PaperObservationEngine()


class ExperimentCreateRequest(BaseModel):
    dataset_version: str
    train_start: str
    train_end: str
    val_start: str
    val_end: str
    oos_start: str
    oos_end: str


class RecordPaperObservationRequest(BaseModel):
    symbol: str
    setup_type: str
    entry_trigger_price: float
    invalidation_price: float
    target_price: float
    shariah_status: str
    data_cutoff_at: str
    provider: Optional[str] = "yahoo_finance"
    rule_version: Optional[str] = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
    sector: Optional[str] = "Unknown"
    liquidity_bucket: Optional[str] = "LARGE_CAP"
    market_regime: Optional[str] = "RANGING"


class EvaluatePaperObservationRequest(BaseModel):
    symbol: str
    setup_type: str
    entry_trigger_price: float
    invalidation_price: float
    target_price: float
    shariah_status: str
    data_cutoff_at: str
    future_opens: List[float]
    future_highs: List[float]
    future_lows: List[float]
    future_closes: List[float]
    future_dates: Optional[List[str]] = None
    provider: Optional[str] = "yahoo_finance"
    rule_version: Optional[str] = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
    sector: Optional[str] = "Unknown"
    liquidity_bucket: Optional[str] = "LARGE_CAP"
    market_regime: Optional[str] = "RANGING"


@router.get("/universe")
async def get_universe_coverage(tier: str = Query("TIER_1_CORE")):
    """Audit actual clean daily coverage for a selected universe tier."""
    try:
        tier_enum = UniverseTier(tier.upper())
    except ValueError:
        tier_enum = UniverseTier.TIER_1_CORE

    mgr = UniverseManager()
    try:
        rep = mgr.audit_coverage(tier_enum)
    except Exception as e:
        raise HTTPException(status_code=503, detail=f"coverage audit unavailable (database error): {type(e).__name__}")
    return {
        "selected_tier": rep.selected_tier,
        "total_symbols_in_tier": rep.total_symbols_in_tier,
        "reconciled_clean_symbols": rep.reconciled_clean_symbols,
        "clean_daily_sessions_count": rep.clean_daily_sessions_count,
        "total_clean_bars": rep.total_clean_bars,
        "missing_sessions_count": rep.missing_sessions_count,
        "genuine_trading_sessions_count": rep.genuine_trading_sessions_count,
        "holiday_phantom_bars_count": rep.holiday_phantom_bars_count,
        "arithmetic_reconciliation": rep.arithmetic_reconciliation,
        "corporate_action_issues": rep.corporate_action_issues,
        "feed_provenance": rep.feed_provenance,
        "disclosure": rep.disclosure,
        "symbols": [s.__dict__ for s in rep.symbol_details],
    }


@router.get("/providers/readiness")
async def get_provider_readiness():
    """Audit provider product access, operational status, feed latency, and licensing."""
    return ProviderAuditor.audit_all()


def _db():
    import os, psycopg2
    return psycopg2.connect(os.environ["DATABASE_URL"])


@router.post("/paper-observations/record")
async def record_paper_observation(req: RecordPaperObservationRequest):
    """Record a screener candidate at decision time. Server-side timestamp, frozen rule_version, price sanity vs stored data."""
    from src.engines.research.strategy_config import PINNED_HASH
    from src.engines.research.paper_observation_store import insert_candidate
    if (req.rule_version or PINNED_HASH) != PINNED_HASH:
        raise HTTPException(status_code=400, detail="rule_version must equal the pinned Strategy V2 hash")
    conn = _db()
    try:
        cur = conn.cursor()
        cur.execute("SELECT id FROM symbols WHERE symbol=%s", (req.symbol,))
        row = cur.fetchone()
        if not row:
            raise HTTPException(status_code=400, detail=f"unknown symbol {req.symbol}")
        cur.execute("SELECT close, (timestamp AT TIME ZONE 'Asia/Kolkata')::date FROM ohlcv_daily WHERE symbol_id=%s ORDER BY timestamp DESC LIMIT 1", (row[0],))
        last = cur.fetchone()
        if not last:
            raise HTTPException(status_code=422, detail="no daily history for symbol; cannot validate candidate")
        if abs(req.entry_trigger_price / float(last[0]) - 1.0) > 0.25:
            raise HTTPException(status_code=422, detail=f"entry_trigger {req.entry_trigger_price} is >25% away from last stored close {float(last[0])} ({last[1]}); rejected as implausible")
        cutoff = datetime.fromisoformat(req.data_cutoff_at.replace("Z", "+00:00"))
        eng = PaperObservationEngine()
        try:
            rec = eng.record_candidate(symbol=req.symbol, setup_type=req.setup_type, entry_trigger=req.entry_trigger_price,
                                       invalidation_price=req.invalidation_price, target_price=req.target_price,
                                       shariah_status=req.shariah_status, data_cutoff_at=cutoff, rule_version=PINNED_HASH,
                                       provider=req.provider or "yahoo_finance", sector=req.sector or "Unknown",
                                       liquidity_bucket=req.liquidity_bucket or "LARGE_CAP", market_regime=req.market_regime or "RANGING")
        except ValueError as e:
            raise HTTPException(status_code=400, detail=str(e))
        oid, created = insert_candidate(conn, row[0], rec)
        conn.commit()
        return {"id": oid, "created": created, "duplicate": not created, "snapshot_sha256_inputs": rec.snapshot_sha256}
    finally:
        conn.close()


@router.post("/paper-observations/evaluate")
async def evaluate_paper_observations():
    """Evaluate all PENDING observations against stored daily bars. Idempotent; terminal rows are never touched."""
    from src.engines.research.paper_observation_store import evaluate_pending
    conn = _db()
    try:
        return evaluate_pending(conn)
    finally:
        conn.close()


@router.get("/paper-observations/summary")
async def get_paper_observations_summary():
    """Summary computed from stored observations. Unavailable metrics are null, never 0."""
    from src.engines.research.paper_observation_store import load_records
    from src.engines.research.paper_observation import summarize
    conn = _db()
    try:
        return summarize(load_records(conn))
    finally:
        conn.close()


# Legacy experiment routes
@router.post("/experiments")
async def create_experiment(req: ExperimentCreateRequest):
    raise HTTPException(status_code=501, detail="not implemented: no experiment registry exists; this route used to return static placeholder data")


@router.get("/experiments/{experiment_id}")
async def get_experiment(experiment_id: str):
    raise HTTPException(status_code=501, detail="not implemented: no experiment registry exists; this route used to return static placeholder data")


@router.get("/experiments/{experiment_id}/candidates")
async def get_candidates(experiment_id: str):
    raise HTTPException(status_code=501, detail="not implemented: no experiment registry exists; this route used to return static placeholder data")


@router.get("/experiments/{experiment_id}/walk-forward")
async def get_walk_forward(experiment_id: str):
    raise HTTPException(status_code=501, detail="not implemented: no experiment registry exists; this route used to return static placeholder data")


@router.get("/experiments/{experiment_id}/sensitivity")
async def get_sensitivity(experiment_id: str):
    raise HTTPException(status_code=501, detail="not implemented: no experiment registry exists; this route used to return static placeholder data")


@router.get("/experiments/{experiment_id}/regimes")
async def get_regimes(experiment_id: str):
    raise HTTPException(status_code=501, detail="not implemented: no experiment registry exists; this route used to return static placeholder data")


@router.get("/experiments/{experiment_id}/robustness")
async def get_robustness(experiment_id: str):
    raise HTTPException(status_code=501, detail="not implemented: no experiment registry exists; this route used to return static placeholder data")


@router.post("/breakout-evidence")
async def refresh_breakout_evidence():
    """Re-run the breakout study on the NSE series and store it. Takes about a minute."""
    import asyncio
    import os

    import psycopg2

    from src.engines.research.breakout_evidence import run_and_store

    def job():
        conn = psycopg2.connect(os.environ["DATABASE_URL"])
        try:
            return run_and_store(conn)
        finally:
            conn.close()

    return await asyncio.get_running_loop().run_in_executor(None, job)
