# HalalEquity P1.9.20.1 + P1.9.21–23 — Clean-up, Intraday Prediction Engine, Dashboard, Forward Tracking

**Bottom line:** the engine is built, tested and running. On the 7 trading days it has been tested on (days it
never trained on), **no horizon shows a profitable edge after costs**, so it currently issues **no BUY
recommendations**. That is the system working as designed, not a failure: it re-tests itself after every
trading day and will only mark a horizon ACTIONABLE when the evidence supports it.

No model can predict prices perfectly. What this engine provides is a probability, an expected move with a
likely range, and a long-only entry/target/stop **only when out-of-sample testing shows a real, cost-adjusted
edge** — plus an honest record of how every forecast turned out.

---

## 1. P1.9.20.1 — clean-up

| Item | Result |
| :--- | :--- |
| Leaked credentials | Full-history scan: the **Telegram bot token** (needs revoking by you in @BotFather) and an old dev JWT (now harmless: signed with the replaced default secret). Placeholder dev passwords only otherwise; no Gemini/GitHub/AWS keys. |
| Browser inspection | Drove the real UI in headless Chrome (sign-up → every page). Found and fixed: **stock page crashed** (`null.toFixed` on missing EPS); **stock page told FAIL stocks "Screening passed internal checks"**; IPO page labelled every IPO "Financial Services", guessed SME/Mainboard from issue size, said "(0)" above 3 issues, said "Pending SEBI clearance"/"DRHP stage" for issues already open, and printed **"Verified compliant against AAOIFI"** under status UNKNOWN; paper-trading page hardcoded **"100.0% Compliant"**; risk calculator priced TCS at a hardcoded ₹3,850 while TCS was ₹2,502; IPO feed status hardcoded `OPERATIONAL_EMPTY`; missing composite score shown as `0.0`; `₹N/A`. Final pass: **14 pages, 0 console errors, 0 failed requests**. |
| Git review | No secrets, dumps, logs or build output in the working tree. It mixes pre-existing uncommitted P1.9.16–19 work with P1.9.20/21 — commit them separately (see §6). Nothing has been committed. |

## 2. P1.9.21 — the engine (`quant/src/engines/prediction/`)

- **Data:** VALID 1-minute bars from `ohlcv_intraday` (SUSPECT bars excluded), aggregated to 5-minute session
  bars. **Every feature and label stays inside one trading session**, so overnight gaps — and therefore
  corporate actions — never enter the model. **The 23,132 quarantined daily bars are never read.**
- **Features (19, point-in-time):** multi-lag returns, realised volatility, range/ATR, position in the day's
  range, VWAP distance, return since open, relative volume, RSI, time of day, and market-wide context
  (universe return, breadth, relative strength).
- **Labels are cost-aware:** a move only counts as UP/DOWN if it beats the full round-trip cost
  (**0.182%**, computed by the platform's cost engine with an intraday fee schedule: STT 0.025% sell-side,
  stamp 0.003%, brokerage min(₹20, 0.03%), GST, 5 bps slippage per side).
- **Model:** gradient-boosted trees (deliberately shallow/regularised), probabilities for DOWN/NEUTRAL/UP,
  expected move, and an empirical 10–90% range built only from out-of-sample results.
- **Walk-forward validation:** for each test day, train only on earlier days. Compared against a
  base-rate baseline (probabilities) and momentum / always-long baselines (trading), with day-block
  bootstrap 95% confidence intervals.
- **Edge gate (all must hold):** ≥30 signals; probabilities beat the base rate (CI > 0); signalled trades
  profitable after costs (CI > 0). Otherwise `NO_DEMONSTRATED_EDGE` → action `NO_TRADE`, with the model's raw
  view still recorded so it can be tracked.
- **Other action rules:** Shariah `FAIL` is never a buy; unevaluated instruments carry an explicit "verify
  before buying" reason; a forecast whose data is stale relative to its horizon is `NO_TRADE`; long-only
  (a "sell" signal means exit/avoid, never short).

### Current evidence (2026-10-07 → 2026-10-30, 51 symbols, 7 out-of-sample days)

| Horizon | Signals | Net return / trade (95% CI) | Momentum | Always long | Verdict |
| :-- | --: | :-- | --: | --: | :-- |
| 5 min | 4 | +0.19% [−0.42%, +1.79%] | −0.18% | −0.18% | No demonstrated edge (too few signals; 82% of 5-min moves are smaller than costs) |
| 15 min | 24 | −0.23% [−0.43%, +0.25%] | −0.18% | −0.17% | No demonstrated edge |
| 60 min | 402 | −0.11% [−0.48%, +0.26%] | −0.17% | −0.18% | No demonstrated edge (better than baselines, still negative) |

Guard against self-deception: a test runs the engine on a pure random walk and asserts it does **not**
claim an edge.

## 3. P1.9.22 — dashboard (`/predictions`)

Per-horizon evidence cards (verdict, CIs, baselines, which checks pass), a live forecast table
(probabilities, expected move, range, entry/target/stop only for actionable buys, data age, Shariah status,
outcome), forward-tracking table, and a banner that states plainly when no horizon is actionable.

## 4. P1.9.23 — forward tracking

- Forecasts are **immutable once issued** (DB trigger) and the runtime role cannot delete them.
- A forecast is only stored if issued **before its target time** — hindsight is never recorded.
- After each horizon passes, the realised price/return/class is filled in once. The tracking view reports
  recommended buys, the raw model's buys, and the always-long baseline, all net of the same costs.
- The worker issues forecasts every 5 minutes during market hours, resolves outcomes, and **re-runs
  walk-forward validation every trading day at 16:15 IST** on the growing history.

## 5. Decisions only you can make

1. **Revoke the Telegram token** (@BotFather).
2. **Real-time data.** The free Yahoo feed is delayed and polled every 15 minutes (`INTRADAY_POLL_MINUTES`,
   now configurable). That makes 5- and 15-minute forecasts mostly stale on arrival. Serious short-horizon
   trading needs a licensed real-time feed (a TrueData adapter stub exists).
3. **Shariah view on intraday trading.** Short-horizon round trips are same-day trades; many AAOIFI-aligned
   scholars require taking delivery. Please confirm with your advisor. The forecasts can also be used purely
   to time delivery purchases.
4. **News.** Not implemented: there is no news source in the platform. Adding it needs a source decision
   (e.g. NSE corporate announcements, a licensed news API) and its own validation — sentiment is only worth
   including if it improves out-of-sample results.
5. **Next-day horizon** needs research-eligible daily history (19 bars/symbol today) → P1.9.24.

## 6. Verification and commit plan

- Go **232 pass / 0 fail**, Python **397 pass / 1 skip**, frontend lint/types/build clean,
  E2E **61/61** (`scripts/verify_p1920_e2e.py`) and **18/18** (`scripts/verify_e2e_delivery.py`),
  headless-browser pass over 14 pages with 0 errors.
- Migration 32 applied to both databases. 98 hindsight rows created during development (before the
  "issue before target" rule existed) were removed with triggers bypassed; nothing else was touched.
- Suggested commits (not done): (1) pre-existing P1.9.16–19 work, (2) P1.9.20 completion, (3) P1.9.20.1 +
  P1.9.21–23.
