from datetime import datetime
from typing import Dict, Any, List, Optional
from src.engines.research.versioning import StrategyConfig

class DatasetSplit:
    def __init__(self, train_start: str, train_end: str, val_start: str, val_end: str, oos_start: str, oos_end: str):
        self.train_start = train_start
        self.train_end = train_end
        self.val_start = val_start
        self.val_end = val_end
        self.oos_start = oos_start
        self.oos_end = oos_end

class StrategyCandidate:
    def __init__(self, candidate_id: str, config: StrategyConfig):
        self.candidate_id = candidate_id
        self.config = config
        self.train_results = None
        self.val_results = None
        self.oos_results = None

class ExperimentTracker:
    def __init__(self, experiment_id: str, dataset_version: str, split: DatasetSplit):
        self.experiment_id = experiment_id
        self.dataset_version = dataset_version
        self.split = split
        self.candidates: Dict[str, StrategyCandidate] = {}
        self.start_time = datetime.utcnow()

    def add_candidate(self, candidate: StrategyCandidate):
        self.candidates[candidate.candidate_id] = candidate

    def record_train(self, candidate_id: str, results: Dict[str, Any]):
        if candidate_id in self.candidates:
            self.candidates[candidate_id].train_results = results

    def record_val(self, candidate_id: str, results: Dict[str, Any]):
        if candidate_id in self.candidates:
            self.candidates[candidate_id].val_results = results

    def record_final_oos(self, candidate_id: str, results: Dict[str, Any]):
        """
        CAUTION: This should only be called once the candidate is frozen as Strategy V2.
        """
        if candidate_id in self.candidates:
            self.candidates[candidate_id].oos_results = results
