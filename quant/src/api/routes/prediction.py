"""Intraday prediction engine routes (train / generate / resolve). Reads for the UI are served by the Go API."""
import os

import psycopg2
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from src.engines.prediction import service
from src.engines.prediction.config import HORIZONS

router = APIRouter()


class TrainRequest(BaseModel):
    horizons: list[int] | None = None


def _conn():
    return psycopg2.connect(os.environ["DATABASE_URL"])


@router.post("/train")
def train(req: TrainRequest | None = None):
    horizons = (req.horizons if req else None) or list(HORIZONS)
    bad = [h for h in horizons if h not in HORIZONS]
    if bad:
        raise HTTPException(status_code=422, detail=f"unsupported horizons {bad}; supported: {sorted(HORIZONS)}")
    with _conn() as conn:
        return {"runs": service.train(conn, horizons)}


@router.post("/generate")
def generate():
    with _conn() as conn:
        return service.generate(conn)


@router.post("/resolve")
def resolve():
    with _conn() as conn:
        return service.resolve(conn)
