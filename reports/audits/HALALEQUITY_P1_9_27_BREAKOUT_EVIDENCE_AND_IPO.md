# HalalEquity P1.9.27 - Breakout evidence, NSE research series, full IPO records

Written 2026-11-05.

## What changed the picture

**A research price series from NSE's own files.** `exchange_series.py` adjusts bhavcopy prices with NSE's corporate-action factors and checks each one
against the actual price step on the ex-date. Result: 15 of 15 actions confirmed, 0 unexplained breaks, about 57,900 bars for 48 stocks from 2022-01-03.
It does not touch the quarantined vendor history or the Strategy V2 gate. It is research grade, not certified (see Limits).

**Do confirmed breakouts work? On this data, no.**

| | Trades | Reached Target 1 | Avg net per trade (95% range) | vs NIFTY 50 |
| :-- | --: | --: | --: | --: |
| All stocks, 2022-2026 | 640 | 57.7% | -0.68% (-0.93 to -0.48) | -0.57% |
| 2025 onward | 205 | 52.2% | -0.89% (-1.31 to -0.50) | -0.73% |
| Shariah-compliant stocks | 338 | 58.3% | -0.51% (-0.80 to -0.23) | -0.61% |

24 alternative plans (exits, entry filters, market filter) were chosen on data up to 2024-12-31 and judged on later data. None passed. The best for
Shariah-compliant stocks made +2.13% per trade on the earlier data and -0.37% on unseen data. No rule was adopted. BreakoutEngine and Strategy V2 are unchanged.

## Built

| Area | What |
| :-- | :-- |
| Live position | Every signal now states where the latest price is against its plan, and the reward-to-risk from that price. EICHERMOT at 7,690: 0.6R, not the planned 1.5R. |
| Breakout monitor | `breakout_tracking`: each confirmed breakout recorded once with its plan frozen by a database trigger, entered at the next open, followed for 10 sessions. 23 recent signals seeded as HISTORY_REPLAY. |
| Backtesting | Real results on the NSE series, separate from the Strategy V2 gate, which stays closed. |
| IPO | Consolidated NSE book (BCSSL 60.77x, was 32.73x), bHNI/sHNI, fresh issue, OFS, lot, lead manager, registrar, RHP link; GMP and SME share counts from IPOWatch, labelled unofficial; View details dialog; listing detected from real trades. |
| Paper trading | Backdated orders replay the whole ledger and are refused only if history would be impossible. Prices across the 15:16-15:28 feed gap use the last valid bar, labelled. |
| Predictions | Forecast date shown, price then, price range, what actually happened, explainer. Polling every 3 minutes (was 15) so 5-minute forecasts can actually be issued. |
| Data quality | Removed phantom bars shown as resolved, one line per date. Honest provider and status labels. A corporate-actions query that silently returned 0 is fixed. |
| Automation | NSE bhavcopy loads daily through a role that can only append evidence. 5-minute bars refresh nightly. The study re-runs each evening. |

## Corrections to earlier work

- **"BSE SME" label was wrong.** NSE's `isBse` flag was read as a listing venue without checking. VPFL, HGS and Maxwell are NSE SME. Removed.
- **Subscription was understated.** `bidDetails` is NSE's own window. `activeCat` is consolidated.
- **Issue size was understated.** It multiplied shares net of anchor by the band. BCSSL is 183.06 Cr, not 135.57 Cr.

## Limits

- Research series: feed artifacts await human review, dividends are not added back, the universe is today's members (survivorship bias flatters results).
- Live forecasts and intraday tracking need the app running during market hours. No forecasts exist for 2026-11-05 because Docker was off.
- GMP is unofficial. Derived SME multiples assume the 2:1 bHNI/sHNI split.
- Strategy V2 stays gated: it needs a person to review the corporate-action evidence.
