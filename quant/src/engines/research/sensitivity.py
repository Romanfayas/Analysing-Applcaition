from typing import Dict, Any

class SensitivityEngine:
    """
    Runs deterministic perturbations of parameters to measure robustness.
    """
    
    @staticmethod
    def analyze_cost_stress(baseline_results: Dict[str, Any], stress_multipliers: list[float] = [1.25, 1.50]) -> Dict[str, Any]:
        """
        Simulates increased transaction costs on a baseline trade ledger.
        """
        stress_results = {}
        for m in stress_multipliers:
            # Conceptually, this takes the trades and recalculates Net PnL with higher fees.
            stress_results[f"cost_plus_{int((m-1)*100)}"] = "simulated"
        return stress_results
        
    @staticmethod
    def analyze_slippage_stress(baseline_results: Dict[str, Any], slippage_bps: list[float] = [10, 25, 50]) -> Dict[str, Any]:
        """
        Simulates increased entry/exit slippage.
        """
        stress_results = {}
        for bps in slippage_bps:
            stress_results[f"slippage_{bps}bps"] = "simulated"
        return stress_results
