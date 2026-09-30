from typing import List, Dict, Any, Callable
from src.engines.research.versioning import StrategyConfig

class GridSearchOptimizer:
    def __init__(self, param_space: Dict[str, List[Any]], max_combinations: int = 100):
        self.param_space = param_space
        self.max_combinations = max_combinations
        
    def _cartesian_product(self) -> List[Dict[str, Any]]:
        import itertools
        keys = list(self.param_space.keys())
        values = list(self.param_space.values())
        combinations = []
        for instance in itertools.product(*values):
            combinations.append(dict(zip(keys, instance)))
        return combinations

    def generate_candidates(self, base_strategy_id: str) -> List[StrategyConfig]:
        configs = self._cartesian_product()
        if len(configs) > self.max_combinations:
            raise ValueError(f"Search space too large: {len(configs)} exceeds {self.max_combinations}. Redesign experiment.")
        
        candidates = []
        for idx, p in enumerate(configs):
            candidates.append(StrategyConfig(
                strategy_id=base_strategy_id,
                version=f"candidate_{idx:03d}",
                parameters=p
            ))
        return candidates
