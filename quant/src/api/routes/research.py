from fastapi import APIRouter
from pydantic import BaseModel
from typing import Dict, Any, List

router = APIRouter()

class ExperimentCreateRequest(BaseModel):
    dataset_version: str
    train_start: str
    train_end: str
    val_start: str
    val_end: str
    oos_start: str
    oos_end: str

@router.post("/experiments")
async def create_experiment(req: ExperimentCreateRequest):
    return {"status": "created", "experiment_id": "exp_001"}

@router.get("/experiments/{experiment_id}")
async def get_experiment(experiment_id: str):
    return {"experiment_id": experiment_id, "status": "running"}

@router.get("/experiments/{experiment_id}/candidates")
async def get_candidates(experiment_id: str):
    return {"candidates": []}

@router.get("/experiments/{experiment_id}/walk-forward")
async def get_walk_forward(experiment_id: str):
    return {"blocks": []}

@router.get("/experiments/{experiment_id}/sensitivity")
async def get_sensitivity(experiment_id: str):
    return {"sensitivity": {}}

@router.get("/experiments/{experiment_id}/regimes")
async def get_regimes(experiment_id: str):
    return {"regimes": {}}

@router.get("/experiments/{experiment_id}/robustness")
async def get_robustness(experiment_id: str):
    return {"overfitting_metrics": {}}
