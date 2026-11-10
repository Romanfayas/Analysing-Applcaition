# Learning loop specification

**Version:** `LEARNING_LOOP_SPEC_V1` · **Menu:** `LEARNER_SPACE_V1` · **Registered:** 2026-11-10 (before the first learner run on the server)
Implementation: `quant/src/engines/research/learning.py` · tables: migration `45-p1928-learning-loop.sql` · page: `/learning`

A change to anything below is a new version of this document (and a new menu version if the menu changes). Results under an older version are never relabelled.

## 1. Purpose and limits
The platform follows every confirmed candle breakout to its result and stores a lesson for it. The learning loop asks one question: *can a fixed procedure that picks a trade plan from past results do better, out of sample and after costs, than the frozen current plan?* If yes, that plan becomes the active **paper** plan; if it later falls behind, it is rolled back automatically.

It does **not**: invent new rules, edit a strategy in place, touch the frozen Strategy V2 parameters (`BUY 80 / SELL 45 / ATR 2.0`), place real orders, or claim that a strategy is profitable. Live trading stays disabled.

## 2. The menu (finite, pre-registered)
24 candidates = 6 exit plans × 2 entry filters × 2 market filters, taken unchanged from the breakout study (`breakout_evidence.PLANS / ENTRY_FILTERS / MARKET_FILTERS`):
exit plans `T1_10D, T2_20D, TRAIL_2ATR_20D, TRAIL_3ATR_40D, HOLD_20D, HOLD_40D` · entry `ANY_OPEN, WITHIN_1PCT` · market `ANY_MARKET, UPTREND` (NIFTY 50 above its 50-day average).
**Control:** `T1_10D · ANY_OPEN · ANY_MARKET` (Target 1 or stop, 10 sessions, no filters). It is never changed, retired or removed. Adding candidates requires a new menu version.

## 3. Trades and costs
Same simulation as the study: signal on a completed session, entry at the next session's open, stop = invalidation level, a bar touching both stop and target is taken as the stop, gaps fill at the open, one open trade per symbol. Net return uses `COST_MODEL_2026_Q4_V2` (itemized statutory + broker + assumed slippage) at ₹1,00,000 per trade. Data: the NSE exchange-adjusted research series (`NSE_EXCHANGE_ADJUSTED_V1`), research-grade, not certified.

## 4. The test (walk-forward on the procedure, not on a winner)
- Origins are calendar quarter starts from 2024-01-01.
- At each origin the procedure picks the candidate with the best mean net return on trades that had **finished before that date** (a trade still open at the origin is invisible), among candidates with ≥ 30 such trades; if none qualify it picks the control.
- That pick is then scored on trades **entered** in the quarter that follows, which it never saw. Pooled out-of-sample trades of the picks ("policy") are compared with the control's trades over the same quarters.
- Only **completed** quarters count. The quarter in progress is shown but never used, so nightly re-runs cannot peek.

## 5. Promotion gates (all required)
| Gate | Rule |
|---|---|
| G1 sample | ≥ 60 pooled out-of-sample trades over ≥ 4 completed quarters |
| G2 positive after costs | 95% BCa interval of the policy's mean net return lies entirely above 0 |
| G3 beats control | 95% bootstrap interval of (policy mean − control mean) lies entirely above 0 (5,000 resamples, seed 20261110) |
| G4 stable | the choice is the same non-control candidate over the last 3 quarters (including the in-progress choice) |
| G5 no repeat | the candidate has never failed probation or been rolled back by the owner in this menu |

Multiple testing: the gates judge the procedure's pooled out-of-sample trades, so choosing among 24 candidates inside the test cannot inflate them; the 24 candidates are disclosed in every run. Quarterly counting limits repeated looks.

## 6. Active version, probation, rollback
- Promotion appends `RETIRED` for the previous active version and `ACTIVE_PAPER` for the new one (append-only events; nothing edited).
- Probation: using trades **entered after the promoting run's data cut-off**, once the active version has ≥ 20 forward trades, if its mean net return is below the control's it receives `PROBATION_FAILED` and the control is back in charge. That version is never promoted again (G5).
- Owner rollback (`/learning`) appends `ROLLED_BACK` (actor OWNER) with the same effect.

## 7. What "active" means today
The active version is a **recorded decision** shown on `/learning`. No component places automated paper orders from it yet: forward evidence is gathered by the nightly **simulation** of the control and the active plan on newly arrived exchange data (probation). Automated paper-order placement from the active version is not built. The nightly learner can only see new outcomes when the NSE daily files are loaded into `exchange_daily_bars`.

## 8. Lessons
For each resolved tracked breakout one row is stored: outcome, gross best/worst move, sessions, volume, structure score, market move, and descriptive tags (e.g. `FAST_FAILURE`, `GAVE_BACK_GAINS`, `LOW_VOLUME_CONFIRMATION`, `MARKET_HEADWIND`). Tags are **not rules**. A pattern can only become a candidate through a new menu version and must pass section 4 like any other.

## 9. Audit
Every run stores: the full per-candidate table on finished trades, every quarter's pick and results, all gates with intervals, the P&L audit (each trade re-priced through the cost ledger and replayed chronologically on a ₹10,00,000 account; ending equity must equal capital plus ledger net P&L, and overlapping positions beyond what the account can afford are flagged), the multiple-testing note and the data limitations.

## 10. Known limitations
Universe is today's NIFTY 50 (survivorship); price return only; fills are simulated; the research series awaits human review of its corporate-action feed; with a few hundred trades the intervals are wide, and a "no change" verdict is the expected, honest outcome unless a real, persistent effect exists.
