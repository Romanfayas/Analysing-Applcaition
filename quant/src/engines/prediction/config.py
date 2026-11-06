"""Frozen configuration for the intraday prediction engine. Change MODEL_VERSION whenever anything here changes."""
from datetime import time

MODEL_VERSION = "INTRADAY_HGB_V1"

BASE_BAR_MINUTES = 5
# horizon (minutes) -> number of 5-minute bars ahead
HORIZONS = {5: 1, 15: 3, 60: 12}

SESSION_OPEN = time(9, 15)
SESSION_CLOSE = time(15, 30)
MIN_TICKS_PER_BAR = 3  # a 5-minute bar built from fewer 1-minute bars is too sparse to trust

# Walk-forward: train only on days strictly before each test day.
MIN_TRAIN_DAYS = 8
MIN_TEST_DAYS = 5

# Classes: a move only counts as UP/DOWN if it beats the round-trip trading cost.
CLASSES = ("DOWN", "NEUTRAL", "UP")

# Order size used to express costs as a fraction of notional.
REFERENCE_ORDER_VALUE_INR = 100_000

# A horizon's predictions are only "ACTIONABLE" when ALL hold out-of-sample:
EDGE_MIN_TRADES = 30                 # enough signals to judge
EDGE_MIN_TEST_DAYS = 20              # P1.9.24: independent out-of-sample sessions required before ACTIONABLE
EDGE_BOOTSTRAP_RESAMPLES = 2000      # day-block bootstrap
EDGE_CONFIDENCE = 0.95               # lower bound of the CI must be > 0 for both tests below
#   1. probabilistic skill: log loss beats the climatology baseline (class frequencies)
#   2. economic edge: mean net-of-cost return per signalled trade is positive
#   3. breadth (P1.9.24): at least EDGE_MIN_TEST_DAYS independent test sessions
# Gate changes only make it stricter; they do not change the model, so MODEL_VERSION is unchanged.
GATE_VERSION = "EDGE_GATE_V2"

# A forecast is too stale to act on when the data it used is older than this fraction of its horizon.
MAX_LATENCY_FRACTION_OF_HORIZON = 0.3

HGB_PARAMS = dict(
    max_depth=3,
    learning_rate=0.05,
    max_iter=150,
    l2_regularization=1.0,
    min_samples_leaf=200,
    random_state=0,
)

FEATURES = [
    "ret_1", "ret_3", "ret_6", "ret_12",
    "vol_6", "vol_12", "range_pct", "atr_6",
    "day_pos", "vwap_dist", "ret_from_open", "vol_ratio", "rsi_14",
    "minutes_since_open",
    "mkt_ret_1", "mkt_ret_3", "mkt_ret_12", "rel_ret_3", "mkt_breadth_3",
]
