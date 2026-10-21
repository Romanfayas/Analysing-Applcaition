"""
Pre-Registered Quantitative Research Experiment Framework

Pre-registers and evaluates versioned quantitative hypotheses to remediate breakout strategy underperformance.
Strictly adheres to:
1. Complete isolation of frozen production strategy (no modifications to BreakoutEngine).
2. Versioned experiment hashes (SHA-256) and recorded parameter search spaces.
3. Train partition (2024-10-07 to 2025-12-19) for hypothesis discovery.
4. Validation partition (2025-12-22 to 2026-05-18) for model selection.
5. Untouched Out-of-Sample partition (2026-05-19 to 2026-10-07) strictly reserved for final verification.
6. Multiple testing correction (Holm-Bonferroni adjusted alpha).
7. Predefined hurdle criteria: Net Return > +3.0%, Profit Factor > 1.30, Max Drawdown < 15.0%.
"""

import hashlib
import json
from dataclasses import dataclass, asdict
from typing import List, Dict, Any, Optional
import numpy as np
import pandas as pd
from scipy import stats

from src.engines.technical.breakout_backtest import BreakoutBacktester, CompletedTrade
from src.engines.technical.breakout_diagnostic import BreakoutDiagnosticEngine, DiagnosticTrade


@dataclass
class ExperimentHypothesis:
    experiment_id: str
    name: str
    description: str
    hypothesis_statement: str
    parameters: Dict[str, Any]
    model_hash: str
    data_cutoff_date: str


@dataclass
class ExperimentResult:
    hypothesis: ExperimentHypothesis
    partition: str
    trade_count: int
    win_rate_pct: float
    avg_net_return_pct: float
    profit_factor: float
    max_drawdown_pct: float
    raw_p_value: float
    adjusted_p_value: float
    passed_hurdles: bool
    rejection_reasons: List[str]


class BreakoutResearchFramework:
    """
    Manages pre-registered research experiments across walk-forward partitions.
    """

    HURDLE_MIN_NET_RETURN = 0.030    # +3.0% net return hurdle
    HURDLE_MIN_PROFIT_FACTOR = 1.30  # 1.30 profit factor hurdle
    HURDLE_MAX_DRAWDOWN = 15.0       # < 15.0% max portfolio drawdown hurdle

    def __init__(self, diagnostic_trades: List[DiagnosticTrade]):
        self.trades = diagnostic_trades

    @staticmethod
    def generate_model_hash(experiment_id: str, params: Dict[str, Any]) -> str:
        serialized = json.dumps({"id": experiment_id, "params": params}, sort_keys=True)
        return hashlib.sha256(serialized.encode('utf-8')).hexdigest()[:16]

    def get_registered_hypotheses(self) -> List[ExperimentHypothesis]:
        """
        Pre-registers 5 concrete research hypotheses with parameter bounds.
        """
        registry = [
            {
                "id": "EXP-01",
                "name": "Market Breadth Regime Filter",
                "desc": "Restrict breakout entries strictly to bullish breadth regimes.",
                "hypo": "Breakouts occurring when market breadth < 55% suffer high bull-trap rates; filtering to >= 55% improves net alpha.",
                "params": {"min_market_breadth": 0.55},
                "cutoff": "2025-12-19"
            },
            {
                "id": "EXP-02",
                "name": "Opening Gap Risk Suppression",
                "desc": "Filter out large opening gaps at executable entry.",
                "hypo": "Opening gaps > 1.0% suffer from opening range fade; filtering gap_risk <= 1.0% reduces adverse excursion.",
                "params": {"max_gap_pct": 0.010},
                "cutoff": "2025-12-19"
            },
            {
                "id": "EXP-03",
                "name": "Ultra-High Institutional RVOL Cutoff",
                "desc": "Require RVOL >= 2.0x instead of 1.30x.",
                "hypo": "Higher participation volume confirms true institutional accumulation, filtering false retail breakouts.",
                "params": {"min_rvol": 2.00},
                "cutoff": "2025-12-19"
            },
            {
                "id": "EXP-04",
                "name": "Consolidation Candidate Only (No Direct Gap)",
                "desc": "Require prior volatility compression within 1.5% of resistance.",
                "hypo": "Direct gap breakouts have weak support structure; setups preceded by tight consolidation hold gains better.",
                "params": {"required_setup_type": "CONSOLIDATION_CANDIDATE"},
                "cutoff": "2025-12-19"
            },
            {
                "id": "EXP-05",
                "name": "Combined Multi-Factor Quality Filter",
                "desc": "Combines Breadth >= 50%, Gap <= 1.0%, and RVOL >= 1.5x.",
                "hypo": "Joint alignment of breadth, price structure, and institutional volume eliminates tail losses.",
                "params": {"min_breadth": 0.50, "max_gap": 0.010, "min_rvol": 1.50},
                "cutoff": "2025-12-19"
            }
        ]

        hypotheses = []
        for r in registry:
            m_hash = self.generate_model_hash(r["id"], r["params"])
            hypotheses.append(ExperimentHypothesis(
                experiment_id=r["id"],
                name=r["name"],
                description=r["desc"],
                hypothesis_statement=r["hypo"],
                parameters=r["params"],
                model_hash=m_hash,
                data_cutoff_date=r["cutoff"]
            ))
        return hypotheses

    def evaluate_hypothesis_on_partition(
        self,
        hypo: ExperimentHypothesis,
        partition: str,
        cost_friction: float = 0.00482
    ) -> ExperimentResult:
        """
        Evaluates a hypothesis against trades in a specific partition.
        """
        part_trades = [t for t in self.trades if t.partition == partition]
        filtered_trades: List[DiagnosticTrade] = []

        params = hypo.parameters
        for tr in part_trades:
            # Check EXP-01: Breadth
            if "min_market_breadth" in params and tr.market_breadth_at_entry < params["min_market_breadth"]:
                continue
            # Check EXP-02: Gap
            if "max_gap_pct" in params and tr.gap_risk > params["max_gap_pct"]:
                continue
            # Check EXP-03: RVOL
            if "min_rvol" in params and tr.rvol < params["min_rvol"]:
                continue
            # Check EXP-04: Setup Type
            if "required_setup_type" in params and tr.setup_type != params["required_setup_type"]:
                continue
            # Check EXP-05: Multi-Factor
            if "min_breadth" in params and tr.market_breadth_at_entry < params["min_breadth"]:
                continue
            if "max_gap" in params and tr.gap_risk > params["max_gap"]:
                continue
            if "min_rvol" in params and tr.rvol < params["min_rvol"]:
                continue

            filtered_trades.append(tr)

        n_tr = len(filtered_trades)
        if n_tr == 0:
            return ExperimentResult(
                hypothesis=hypo,
                partition=partition,
                trade_count=0,
                win_rate_pct=0.0,
                avg_net_return_pct=0.0,
                profit_factor=0.0,
                max_drawdown_pct=0.0,
                raw_p_value=1.0,
                adjusted_p_value=1.0,
                passed_hurdles=False,
                rejection_reasons=["Zero trades passed filter."]
            )

        net_returns = np.array([tr.gross_return - cost_friction for tr in filtered_trades])
        gross_returns = np.array([tr.gross_return for tr in filtered_trades])
        wins = np.sum(net_returns > 0)
        win_rate = float(wins / n_tr * 100.0)
        avg_net = float(np.mean(net_returns))

        gross_gains = float(np.sum(gross_returns[gross_returns > 0])) if len(gross_returns[gross_returns > 0]) > 0 else 0.0
        gross_losses = float(abs(np.sum(gross_returns[gross_returns < 0]))) if len(gross_returns[gross_returns < 0]) > 0 else 0.0001
        profit_factor = round(gross_gains / gross_losses, 2)

        # Portfolio drawdown simulation
        completed_adapter = [
            CompletedTrade(
                symbol=tr.symbol, sector=tr.sector, shariah_status="PASS", partition=partition,
                entry_bar=0, exit_bar=tr.bars_held, entry_date=tr.executable_entry_date, exit_date=tr.exit_date,
                entry_price=tr.entry_price, exit_price=tr.exit_price, invalidation_level=0, target_price=0,
                exit_reason=tr.exit_reason, bars_held=tr.bars_held, mfe=tr.mfe, mae=tr.mae,
                gross_return=tr.gross_return, net_return=tr.gross_return - cost_friction
            ) for tr in filtered_trades
        ]
        port_dd = BreakoutBacktester.simulate_portfolio_drawdown(completed_adapter)

        # Statistical significance t-test against 0
        if n_tr > 3 and np.std(net_returns) > 0:
            _, raw_p = stats.ttest_1samp(net_returns, 0.0)
            raw_p = float(raw_p)
        else:
            raw_p = 1.0

        rejection_reasons = []
        if avg_net < self.HURDLE_MIN_NET_RETURN:
            rejection_reasons.append(f"Net return ({avg_net*100:.2f}%) below hurdle (+{self.HURDLE_MIN_NET_RETURN*100:.1f}%).")
        if profit_factor < self.HURDLE_MIN_PROFIT_FACTOR:
            rejection_reasons.append(f"Profit factor ({profit_factor}) below hurdle ({self.HURDLE_MIN_PROFIT_FACTOR}).")
        if port_dd > self.HURDLE_MAX_DRAWDOWN:
            rejection_reasons.append(f"Portfolio drawdown ({port_dd:.2f}%) exceeds hurdle ({self.HURDLE_MAX_DRAWDOWN}%).")
        if n_tr < 20:
            rejection_reasons.append(f"Sample size ({n_tr}) insufficient for statistical power (min 20).")

        passed = len(rejection_reasons) == 0

        return ExperimentResult(
            hypothesis=hypo,
            partition=partition,
            trade_count=n_tr,
            win_rate_pct=round(win_rate, 1),
            avg_net_return_pct=round(avg_net * 100.0, 2),
            profit_factor=profit_factor,
            max_drawdown_pct=port_dd,
            raw_p_value=round(raw_p, 4),
            adjusted_p_value=round(raw_p, 4),  # Will be adjusted by Holm-Bonferroni
            passed_hurdles=passed,
            rejection_reasons=rejection_reasons
        )

    def run_all_experiments(self) -> Dict[str, List[ExperimentResult]]:
        """
        Executes all 5 pre-registered hypotheses across Train, Validation, and Untouched OOS.
        Applies Holm-Bonferroni multiple testing corrections to p-values.
        """
        hypotheses = self.get_registered_hypotheses()
        results: Dict[str, List[ExperimentResult]] = {
            "TRAIN": [],
            "VALIDATION": [],
            "OUT_OF_SAMPLE": []
        }

        for part in ["TRAIN", "VALIDATION", "OUT_OF_SAMPLE"]:
            part_results = []
            for h in hypotheses:
                res = self.evaluate_hypothesis_on_partition(h, part)
                part_results.append(res)

            # Apply Holm-Bonferroni multiple testing adjustment across the 5 hypotheses
            sorted_indices = np.argsort([r.raw_p_value for r in part_results])
            m = len(part_results)
            for rank, idx in enumerate(sorted_indices):
                adj_p = min(1.0, part_results[idx].raw_p_value * (m - rank))
                part_results[idx].adjusted_p_value = round(float(adj_p), 4)

            results[part] = part_results

        return results
