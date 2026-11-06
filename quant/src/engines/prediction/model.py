"""Model fitting, full-probability prediction, and the empirical conditional return distribution."""
from dataclasses import dataclass, field
from typing import Dict, List, Optional

import numpy as np
from sklearn.ensemble import HistGradientBoostingClassifier

from .config import HGB_PARAMS

N_CLASSES = 3  # DOWN, NEUTRAL, UP
QUANTILES = (0.10, 0.25, 0.50, 0.75, 0.90)


def fit(X: np.ndarray, y: np.ndarray) -> HistGradientBoostingClassifier:
    model = HistGradientBoostingClassifier(**HGB_PARAMS)
    model.fit(X, y)
    return model


def predict_full(model, X: np.ndarray) -> np.ndarray:
    """n x 3 probabilities even if a class was absent in training (its probability is then 0)."""
    p = np.zeros((len(X), N_CLASSES))
    p[:, model.classes_.astype(int)] = model.predict_proba(X)
    return p


def class_frequencies(y: np.ndarray) -> np.ndarray:
    counts = np.bincount(y.astype(int), minlength=N_CLASSES).astype(float)
    return counts / counts.sum()


def class_mean_returns(y: np.ndarray, fwd: np.ndarray) -> np.ndarray:
    """Mean forward log return of each class in the training data (0 for an absent class)."""
    out = np.zeros(N_CLASSES)
    for c in range(N_CLASSES):
        sel = y == c
        if sel.any():
            out[c] = float(np.mean(fwd[sel]))
    return out


@dataclass
class ConditionalReturns:
    """Empirical forward-return quantiles conditioned on the model's score (p_up - p_down).

    Built only from out-of-sample predictions, so the ranges reflect how the model actually performed
    on unseen days rather than how well it fits its own training data.
    """
    edges: List[float] = field(default_factory=list)          # interior bin edges on score
    quantiles: List[Dict[str, float]] = field(default_factory=list)  # one dict per bin
    counts: List[int] = field(default_factory=list)

    @classmethod
    def from_oof(cls, score: np.ndarray, fwd: np.ndarray, bins: int = 10) -> Optional["ConditionalReturns"]:
        if len(score) < bins * 30:
            bins = max(1, len(score) // 30)
        if bins < 1:
            return None
        edges = np.unique(np.quantile(score, np.linspace(0, 1, bins + 1)[1:-1])) if bins > 1 else np.array([])
        idx = np.digitize(score, edges)
        qs, counts = [], []
        for b in range(len(edges) + 1):
            r = fwd[idx == b]
            counts.append(int(len(r)))
            if len(r) == 0:
                r = fwd
            qs.append({f"q{int(q * 100)}": float(np.quantile(r, q)) for q in QUANTILES} | {"mean": float(np.mean(r))})
        return cls(edges=[float(e) for e in edges], quantiles=qs, counts=counts)

    def lookup(self, score: float) -> Dict[str, float]:
        return self.quantiles[int(np.digitize([score], self.edges)[0])]
