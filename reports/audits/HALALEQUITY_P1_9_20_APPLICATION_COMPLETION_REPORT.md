# HalalEquity P1.9.20 — Application Completion Report

**Scope:** inspect the running application, find unfinished features and defects, and fix them before any
model work (P1.9.21+). No prediction model, no data-eligibility change, no Shariah automation was added.

**Standing constraints preserved:** 23,132 historical bars remain quarantined (`adjusted_close IS NULL`);
Shariah status stays *unevaluated* (0 certified PASS); `LIVE_ORDER_EXECUTION=false`,
`BROKER_TRADING_ENABLED=false`; no short selling.

---

## 1. Verification summary (all measured on 2026-10-31)

| Check | Result |
| :--- | :--- |
| Go suite (`scripts/run_go_tests.sh ./...`) | **229 pass, 0 fail, 0 skip** (was 9 failing on arrival) |
| `go vet ./...` | clean |
| Python suite (`pytest` in `halal_equity_quant`) | **381 pass, 1 skip** (the 1 skip is the existing mount check) |
| Frontend `eslint` | **0 errors, 0 warnings** (was 61 errors, 34 warnings) |
| Frontend `tsc --noEmit` / `next build` | clean / 20 routes + request gate |
| New live E2E `scripts/verify_p1920_e2e.py` | **52/52** |
| `scripts/verify_e2e_delivery.py` | **18/18** |
| `scripts/test_workflows_live.py` | all safety rules fire (Shariah gate, short-sell, input validation) |
| Live DB after all runs | `ohlcv_daily` = 24,017 (unchanged); quarantined = 23,132 (unchanged); test accounts removed |

**Not verified:** nothing was viewed in a real browser. Pages return HTTP 200 and the TypeScript/lint/build are
clean, but visual layout and client-side runtime behaviour should be eyeballed once.

---

## 2. Defects found and fixed

### Data & ingestion
| # | Severity | Finding | Fix |
| :-: | :-: | :--- | :--- |
| 1 | **HIGH** | **Intraday ingestion had stored 0 candles since 2026-10-21.** The worker runs as the least-privilege `halal_app_user`, but `UpsertSymbol` did `INSERT … ON CONFLICT DO UPDATE` on `symbols` (SELECT-only for that role). Every run logged `permission denied` per symbol. | `UpsertSymbol` resolves existing canonical symbols with a `SELECT` and never rewrites them. Grants were **not** widened. Live worker now stores ~1,800 bars/symbol (`ohlcv_intraday` +90,509 rows). Tests run as the real role. |
| 2 | **HIGH** | **UI showed 9-day-old prices.** `/stocks` preferred the latest *intraday* bar and only fell back to daily when none existed, so a dead intraday feed pinned every price to Oct 21 although Oct 30 closes were stored. | New `GetLatestPrice` returns the newer of intraday/daily. |
| 3 | **HIGH** | **IPO feed parsed wrongly.** JSON tags were snake_case; NSE sends camelCase, so every record had an empty name and all collapsed into one junk row (`MAXENGS`, id 8) while the run logged "SUCCESS stored 6". Status was hardcoded `UPCOMING`; dates/price band discarded; Telegram "New IPO" re-sent for every IPO on every 4-hourly run. `issueSize` is a *share count* but was destined for a ₹-crore column. | Rewrote the parser (`nsedata/ipo.go`, tested against a captured payload): real status/dates/band, ₹-crore derived from shares × upper band (NULL if band unknown), rejects bad records, alerts only on newly inserted IPOs. Migration 29 removes the junk row and adds a non-blank CHECK. Migration 30 widens `ipos.status` (`CLOSED_AWAITING_LISTING` is 23 chars; column was 20 — caught by the live run). |
| 4 | MED | `signal_weight_configs` was never granted to the runtime role (hidden because weights were a hardcoded literal). | Migration 31 (SELECT only). Scanned every table named in Go SQL: no other missing grants. |

### Fabricated / dishonest output (violates "no fabricated data")
| # | Severity | Finding | Fix |
| :-: | :-: | :--- | :--- |
| 5 | **HIGH** | `GetMarketOverview` returned **hardcoded index values** (NIFTY 50 24,850.30 +0.58%, NIFTY 500 23,180.45) presented as live. | Indices now `null` / `DATA_UNAVAILABLE` (no index provider exists). |
| 6 | **HIGH** | Frontend defaulted **"Shariah Pass Rate" to 100.0%** with no data, and the portfolio page hardcoded **"Shariah Compliance 100.0% — zero non-compliant holdings"**. | Dashboard shows certified / failed / unevaluated counts; portfolio computes from actual holdings. |
| 7 | MED | Research gate: status hardcoded `RESEARCH_BLOCKED`, `is_strategy_hash_valid: true` a literal, "27 core equities / 52 / 47" string constants, `equities_lacking_primary_evidence` actually counted all symbols. Backtest page had matching `|| 23132 / 27 / 52` fallbacks. | One measured `computeGate` (bars, events, strategy attestation fetched live from the quant engine); both run endpoints and the page use it. Gate stays closed if attestation is unreachable. |
| 8 | MED | Stub endpoints returning fake success: `POST /alerts` "created", `DELETE` "deleted", `PUT /settings` "updated", `POST /backtesting/strategies` "created", `PUT /signals/weights` "updated" — nothing persisted. Python `/api/quantitative/*`, `/technical/trend` (`"NEUTRAL"`), `/support-resistance` returned empty-but-OK bodies. | Alerts/settings/strategies/runs now persist. Weights are **frozen** → `PUT` answers 409 (changing them would invalidate the pinned Strategy V2 hash). Unimplemented Python routes answer **501**. |
| 9 | MED | Stock `candles / fundamentals / technicals / risk / analytics` and `search` returned `{"status":"placeholder"}` or `[]`. | Implemented. Technicals/risk use **research-eligible bars only**; insufficient history is reported (`DATA_INSUFFICIENT` / `skipped`), never estimated. Candles carry a `research_eligible` flag so quarantined bars display but never feed calculations. |
| 10 | LOW | "Daily P&L" was really unrealized P&L since cost; `StatCard` appended "vs last month" to every figure; dashboard/portfolio read field names the API never sent (`avg_price`, `current_price`, `pnl`), so P&L was always 0. | Renamed/relabelled; fields fixed. |

### Authentication, security, multi-tenancy
| # | Severity | Finding | Fix |
| :-: | :-: | :--- | :--- |
| 11 | **HIGH** | **Auth was a stub** (`login` returned the string `"placeholder"`; `users` empty). The frontend worked by injecting a **JWT hardcoded in source** (twice: the proxy and a script), and the proxy was disabled outright in production. | Real register/login/refresh (bcrypt, constant-time login, no user enumeration, strict body limits, optional registration). Next.js now has a login page, an `httpOnly` session cookie, a `proxy.ts` gate, and a BFF route that attaches the token server-side. Hardcoded JWTs removed. |
| 12 | **HIGH** | **Every user shared one portfolio** (`GetOrCreatePortfolio(ctx, 1)` ×12; `portfolios.user_id` an `integer` "1 for single-user MVP"). `/portfolios/{id}/…` ignored `{id}`. | Migration 28: per-account `owner_id UUID`. Ownership enforced on every portfolio route; another account's id is a 404. |
| 13 | **HIGH** | **Paper ledger accepted a client-chosen fill price** (buy at ₹1), and filled on prices of any age. | Price field refused; fills use the stored market price and require `FRESH`/`LAST_AVAILABLE`. |
| 14 | **HIGH** | **Telegram bot token + chat ID committed** in `.env.example` and hardcoded in `scripts/test_telegram.py`; repo has a GitHub remote. | Removed from the working tree (**still in git history — see §4**). |
| 15 | MED | `next.config.ts` rewrite exposed the backend at `/api/v1/*`, bypassing the proxy. `JWT_SECRET` unset → compose uses a public default. | Rewrite removed. Random `JWT_SECRET` generated into the (gitignored) `.env`. |

### Tests & tooling
| # | Finding | Fix |
| :-: | :--- | :--- |
| 16 | 9 Go tests failed: hand-maintained whitelists of "valid bar counts" (`[]int{23782, …, 23970}`) break every trading day; the changelog's "202/202" had only been fixed on the Python side. | `assertForwardSessionGrowth` asserts `floor + 47·N` (floors unchanged, so no assertion got weaker). |
| 17 | Go DB tests hardcode a LAN IP; a Homebrew Postgres owns `127.0.0.1:5432` and shadows the container (`role "halal_user" does not exist`); the IP had changed since the last run. | `scripts/run_go_tests.sh` discovers the address. |
| 18 | 61 ESLint errors, dead code (`internal/api/`, `lib/api.ts`), 8 of 12 pages unreachable from the nav. | Fixed / removed / full nav + user menu + sign-out. |

---

## 3. What was added

- **Migrations 27–31** (additive; applied to `halal_equity` and `halal_equity_test`; a `pg_dump` was taken first):
  alerts + settings tables, per-user portfolios, IPO integrity, status width, weights grant.
- **Features:** login/sign-out, working Alerts (price above/below, evaluated every 5 min by the worker, fire-once, ignores prices > 4 days old; other alert types are rejected rather than silently never firing), working Settings, strategy/run persistence, a "Technicals & Risk" card on the stock page.
- **Tests:** +27 Go (auth, alerts, ownership, gate, IPO parser/upsert, app-role grants), +7 Python (501 routes), `verify_p1920_e2e.py`.

---

## 4. Needs your action

1. **Revoke the leaked Telegram bot token** in @BotFather and issue a new one. It remains in git history (and on GitHub if the repo is public). Rewriting history is your call; I did not touch git history or push.
2. Set a strong `JWT_SECRET` anywhere beyond this machine (a random one is now in the local `.env`).
3. Existing sessions/old dev tokens are invalid after the secret change — sign in (or register) at `/login`.
4. The legacy portfolio row (`id 1`, no positions/trades) is left orphaned, not adopted by any account.
5. **Not committed.** All changes are in the working tree; nothing was committed or pushed.

## 5. Still blocked on you / external (unchanged from your list)

| Item | Status |
| :--- | :--- |
| Authenticated NSE/BSE corporate-action documents | Pending — needs your acquisition. The gate reports 58 unauthenticated events. |
| Audited disclosures for 31 equities; manual Shariah analysis | Deferred. BUY stays blocked for all 51 (0 certified). |
| Making historical bars research-eligible | Blocked on the two items above. Not touched. |
| Live broker | Later phase; disabled in config. |

## 6. Honest readiness for P1.9.21 (prediction)

- **Infrastructure is ready:** a working worker, authenticated API, `ohlcv_intraday` ingesting again (268k rows), an alerts/dashboard surface to attach forecasts to.
- **Data is the constraint, not code:** only **19 research-eligible daily bars per symbol** exist (the rest are quarantined), so daily-horizon models cannot be validated yet, and `ohlcv_intraday` is missing two trading days (2026-10-22 and 10-23, ~36k bars) that the provider's short intraday window no longer lets us backfill. (Intraday was dead 10-22→10-25; 10-26 onward was recovered when the fix went live.) Intraday models can start on forward data, but out-of-sample evidence will need to accumulate before any accuracy claim.
- Known small limitation: `POST /risk/calculate` does its arithmetic in Go, against the "all calculations in Python" principle; left as-is.
