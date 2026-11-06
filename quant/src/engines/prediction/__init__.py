"""Intraday prediction engine (P1.9.21).

Probabilistic, cost-aware forecasts of the next 5/15/60 minutes, validated walk-forward on data the
model never saw. Every feature and label lives inside a single trading session, so overnight gaps
(and therefore corporate actions) never enter the models, and quarantined daily history is never read.
"""
