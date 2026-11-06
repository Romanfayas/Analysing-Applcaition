# HalalEquity P1.9.26 - what we took from five trading repositories, and what we did not

Reviewed 2026-11-05 by reading the source (shallow clones, nothing executed): StockSharp, FinRobot, TensorTrade, PyBroker, Lumibot.

## Licences decide how each can be used

| Repo | Licence | How we used it |
| :-- | :-- | :-- |
| StockSharp | Proprietary (EULA on its website) | **Ideas only.** No code copied. |
| PyBroker | Apache 2.0 **with Commons Clause** (no selling a product whose value comes substantially from it) | **Ideas only**, implemented independently from textbook definitions. |
| Lumibot | **GPL v3** (copyleft) | **Ideas only.** Copying code would force the whole app under GPL. |
| FinRobot, TensorTrade | Apache 2.0 | Ideas; no code was needed. |

## All five are built for US markets - what did not carry over
US session/holidays, US fee model (per-share or percent), T+0 US-style margin, SEC EDGAR, Alpaca/IBKR brokers.
Nothing in them knows NSE circuits, STT, stamp duty, DP charges, T+1 settlement, bhavcopy corporate-action quirks or Shariah screening. Those were already specific to this app and are the harder part.

## What we adopted (adapted for India)

| Idea (source) | Built here |
| :-- | :-- |
| Stop-loss / take-profit / trailing stop that fill against the bars that followed, gap-through fills at the open (PyBroker, StockSharp risk rules) | `backend/internal/services/paper/stops.go`: no-lookahead fills; a bar touching both stop and target assumes the stop (pessimistic); tested on 1-min, else daily bars; missing sessions are reported. |
| Slippage models: fixed + volume/volatility based (PyBroker `slippage.py`) | Square-root market-impact on the stock's own ADV and volatility from NSE bhavcopy volume, on top of the platform's 5 bps. Warns when an order is >=1% of ADV. |
| Per-order fee modes (PyBroker, Lumibot `TradingFee`) | Added an **intraday (MIS)** Indian schedule (STT 0.025% sell only, stamp 0.003%, brokerage min(Rs20, 0.03%), no DP charge) next to the delivery schedule. |
| Bootstrap confidence on metrics, BCa intervals, drawdown distribution (PyBroker `eval.py`) | `quant/src/engines/research/performance.py`: expectancy, profit factor, drawdown-by-reordering, Ulcer index, cost drag, excess vs NIFTY, each with a 95% BCa interval and an explicit "too few trades" state. A random-walk test proves noise is not reported as skill. |
| Benchmark-relative evaluation (Lumibot, FinRobot) | Official NSE index closes (`index_daily_bars`, loaded from `ind_close_all_*.csv`, honest client name, stops if refused). Used for the simulator, the performance report and the market overview (replacing "no index data"). |
| Account-level risk limits (StockSharp `Algo/Risk`) | Max position % of account, daily-loss pause, orders per day; sells never blocked; configurable by env, 0 disables. |
| Cost-aware thinking (TensorTrade commission study: a model can predict direction yet lose to costs through overtrading) | Prediction cards now show gross return per trade, round-trip cost and signals per day - which is exactly our finding (signals real but below 0.18% cost). |
| Provenance of every number (FinRobot "numbers are code-calculated, narratives LLM-assisted", Lumibot data provenance) | Already our design; extended: index rows carry source URL + SHA-256; simulator labels every price basis. |

## What we deliberately did not adopt, and why
- **Reinforcement learning (TensorTrade):** its own docs report overfitting and cost-destroyed profit at ~4,000 hourly candles. We have ~60 sessions of 5-minute data; RL would manufacture results.
- **LLM research agents / DCF reports (FinRobot):** needs an LLM key and an Indian filings source (XBRL/BSE/NSE announcements) that is not available; an LLM guessing Indian fundamentals would violate the no-fabrication rule. Its WACC/DCF operators use US inputs (risk-free, ERP). Possible later, with an owner decision on an LLM provider and data.
- **Live broker execution (Lumibot, StockSharp):** live trading is disabled by policy. Indian brokers (Zerodha, Upstox, Angel) have no adapter in any of the five.
- **Their backtest engines:** ours is gated until daily history is certified; swapping the engine would not remove that gate.
- **Parameter optimisation (PyBroker/Optuna, TensorTrade/Optuna):** Strategy V2 parameters are frozen and hash-attested; tuning on the same data is the overfitting these repos warn about.

## Indian-market assumptions that are parameters, not facts
- 248 trading days/year; risk-free 6.5% (`RISK_FREE_RATE_PCT`, set to the current 91-day T-bill yield).
- Intraday brokerage assumes a discount broker plan; verify against your contract note.
- Intraday (same-day) trading: many scholars require delivery - the simulator says so; owner decision.
