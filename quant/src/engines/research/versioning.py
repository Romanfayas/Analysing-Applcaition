import hashlib
import json
from dataclasses import dataclass, asdict
from typing import Dict, Any

@dataclass
class StrategyConfig:
    strategy_id: str
    version: str
    parameters: Dict[str, Any]
    
    @property
    def parameter_hash(self) -> str:
        """Deterministically hashes the parameter dictionary."""
        sorted_params = json.dumps(self.parameters, sort_keys=True)
        return hashlib.sha256(sorted_params.encode('utf-8')).hexdigest()[:12]

    def to_dict(self) -> dict:
        d = asdict(self)
        d['parameter_hash'] = self.parameter_hash
        return d
