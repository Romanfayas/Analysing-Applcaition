# HalalEquity P1.9.24 — Remediation Plan (written after inspection, before implementation)

## What inspection established (2026-10-31)

| # | Finding | Evidence |
| :-: | :--- | :--- |
| F1 | Daily history: 24,017 bars, 47 instruments, 512 sessions (2024-10-07 → 2026-10-30); 23,132 quarantined, 885 eligible (19/symbol, all ≥ 2026-10-05). | DB counts |
| F2 | Quarantined bars are Yahoo bars whose vendor *adjusted* close ≠ close. Their OHLC is stored. Release requires an update from `NSE_CORPORATE_ACTION/PRIMARY_EXCHANGE` matching a verified, undisputed factor on a `VERIFIED_EXCHANGE_PRIMARY` event. | `fn_ohlcv_daily_quarantine_guard` |
| F3 | The quarantine guard hardcodes `NEW.timestamp::date >= '2026-10-05'`: **no bar before 2026-10-05 can ever be released, whatever evidence exists.** | guard source |
| F4 | **8/47 instruments' stored historical "close" is not the traded price** — Yahoo back-adjusted it for splits/bonuses/demergers (BAJFINANCE ×0.1, DRREDDY/KOTAKBANK ×0.2, HDFCBANK/NESTLEIND/RELIANCE/WIPRO ×0.5, ITC ×0.9629). 39/47 match the exchange exactly. | NSE bhavcopy 2024-10-07 vs `ohlcv_daily` |
| F5 | None of those split/bonus/demerger events exist in `corporate_action_events` (57 cash dividends, 1 Tata Motors demerger, 6 "OTHER"). NSE's own feed lists them (e.g. RELIANCE Bonus 1:1 ex 28-Oct-2024; BAJFINANCE Bonus 4:1 + split 2→1 ex 16-Jun-2025; ITC demerger ex 06-Jan-2025) and the implied factors equal the observed vendor ratios. | NSE `corporates-corporateActions` API |
| F6 | The 6 `AMBIGUOUS/OTHER` events are parser fragments ("CML", "70875)", "2026", "LTM_RENAME") of the LTIM rename / Tata Motors demerger circular references, not corporate actions. | event subjects |
| F7 | All 23 evidence artifacts are `CONTROLLED_ARCHIVE:` local files (cannot prove origin → can never verify). One (`NSE_CA_TESTHARV_PRIMARY`) is a test fixture in the production evidence table (append-only, cannot be removed). | artifacts table |
| F8 | Authenticated primary sources **are reachable** with valid TLS: NSE CM bhavcopy archives (UDiFF since 2024-07-08, legacy before) and the NSE corporate-actions API. The official acquisition pipeline's host allowlist omits `nsearchives.nseindia.com`. | live probes |
| F9 | Intraday: minutes 15:16–15:28 are missing for ~98% of symbol-days (provider omission, unrecoverable by re-polling). | gap profile |
| F10 | Intraday `retrieved_at` is overwritten on every re-poll → first-receipt time is lost → feed latency cannot be audited. | upsert SQL, data |
| F11 | Intraday recovery only fetches forward from the newest bar; earlier holes are never re-requested while still inside the provider's ~7-day window (how Oct 22–23 was lost). | pipeline.go |
| F12 | TrueData: adapter exists (fixture mode + live client), **no credentials configured** → cannot be evaluated live. | `.env`, provider code |
| F13 | Walk-forward evidence spans only 7 test days; the edge gate does not require a minimum number of independent days or any regime breakdown. | walkforward.py |

## Plan

1. **Authenticated daily evidence (F8, F4, F1).** New append-only tables `exchange_source_files` / `exchange_daily_bars` populated **only by an operator command** from NSE bhavcopies (TLS-verified, SHA-256 + TLS certificate fingerprint recorded). `ohlcv_daily` is not modified. Backfill 2024-10-07 → 2026-10-30, and extend earlier to grow authenticated nominal coverage.
2. **Vendor-vs-exchange reconciliation (F4).** View classifying every stored bar: `NOMINAL_MATCH`, `VENDOR_ADJUSTED` (constant ratio), `MISMATCH`, `MISSING_*`.
3. **Corporate-action evidence (F5–F7).** Fetch NSE's corporate-actions feed per symbol through the official pipeline; store as append-only artifacts at `AUTHENTICITY_PENDING` (never promoted by software). Record a reconciliation run: canonical events confirmed by the feed, canonical events absent from it, feed actions missing from the canonical table (with implied factor cross-checked against observed price ratios), and parser fragments. **No event or bar is modified.**
4. **Point-in-time readiness (F2, F3).** Per-symbol readiness report stating exactly what blocks research eligibility and which artifacts an operator must review. Next-day prediction research stays `DATA_INSUFFICIENT` until eligible history exists. The `2026-10-05` floor is reported as a decision for the owner, not changed.
5. **Intraday feed reliability (F9–F12).** Add `first_received_at` (never overwritten); gap-aware recovery inside the provider window; a feed-health endpoint (coverage, structural gaps, measured latency going forward). TrueData evaluation limited to what can be established without credentials.
6. **Out-of-sample strength (F13).** Add per-day and per-market-regime breakdowns and require ≥ 20 independent test days before any horizon can be ACTIONABLE (stricter only). Model, features, Strategy V2 parameters and hash unchanged.
7. Keep backtesting blocked, live trading disabled, Shariah unevaluated, BUY gates intact. Full test suite and completion report.
