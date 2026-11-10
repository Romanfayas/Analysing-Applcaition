# HalalEquity on Instabox (3-box fleet)

Layout since 2026-11-10 (second cut): **api / db / jobs**. Paper trading only: `PAPER_TRADING_ENABLED=true`, `LIVE_ORDER_EXECUTION=false`, `BROKER_TRADING_ENABLED=false`; the watchdog raises CRITICAL if either live flag is ever true.

| Role | Box | Runs | Public |
|---|---|---|---|
| **api** | `analysing-application` (box 1) | frontend, backend API, quant (on-demand calls), Redis (unused by the app), SSH tunnel to the DB box, watchdog, backup pull + weekly restore drill | https://3000-agq4juf6ixalcsity3la2ldyam.instabox.tech (login page) |
| **db** | `second-machine` (box 2) | Postgres 16 + TimescaleDB 2.30.1 only, data on `/data/workspace/halal/pgdata` (20 GB disk), nightly verified backup | `…8088…/health` only |
| **jobs** | `third-machine` (box 3) | worker (market ingestion, EOD, IPO, intraday predictions, breakout tracker, alerts) + quant, SSH tunnel to the DB box, backup pull | `…8088…/health` only |

Data flow: browser → frontend → backend (box 1) → **tunnel** → Postgres (box 2) ← **tunnel** ← worker (box 3).
The boxes have no private network. Each of box 1 and 3 runs a tiny container (`halal-tunnel`, image in `deploy/instabox/tunnel`) that takes over the compose service name **`postgres`** and forwards port 5432 through the Instabox SSH gateway to box 2 (measured ≈ 8 ms per request; killing the ssh process → reconnects in ~3 s, worker had no errors). Connection strings are therefore unchanged.

**Double-run protection:** each role's compose file puts services that don't belong on that box under a disabled profile (`worker` is off on the api box; `backend/frontend/redis` are off on the jobs box). A plain `./dc up -d` can't start a second worker.

## Access and secrets
- `ssh -p 2222 <ibssh_user>@ssh.instabox.sh`. The `ibssh_…` string **is the credential** (no key). It lives only in `~/.config/halal/fleet.env` and `DB_SSH_USER` in `.env` (both mode 600). Never commit; rotate in the Instabox dashboard if leaked. Anyone holding box 2's string can open a tunnel to Postgres (DB passwords still apply).
- App secrets are generated in `/data/workspace/halal/app/.env` (mode 600) and pushed by `fleet-sync.sh`. Self-registration is disabled (`ALLOW_REGISTRATION=false`).

## Layout on each box (`/data/workspace/halal/`)
`app/` code + `.env` · `role` (api|db|jobs) · `fleet/` scripts · `logs/` · `state/` · `backups/` · (db box) `pgdata/`
`app/dc` = role-aware compose wrapper (`docker-compose.yml` + `deploy/instabox/compose/docker-compose.<role>.yml`). The DB box uses `docker-compose.db.yml` standalone: `docker compose -p halaldb --project-directory /data/workspace/halal/app -f deploy/instabox/compose/docker-compose.db.yml …`.

## Automatic jobs (systemd user units, lingering → start at boot)
| Unit | Box | When | Does |
|---|---|---|---|
| halal-fleet-health.service | all | always | `:8088/health` (role-specific container status) |
| halal-fleet-ping.timer | all | 10 min | each box touches the other two (SSH exec + public HTTP) → `logs/fleet-ping.jsonl` |
| halal-stack.service | all | boot | bring that box's containers up |
| halal-db-backup.timer | db | 14:30 UTC | verified `pg_dump`, keep 7 |
| halal-backup-pull.timer | api, jobs | 15:15 / 15:20 UTC | pull newest dump, sha256 + readability check, keep 7 |
| halal-restore-drill.timer | api | Sun 16:00 UTC | restore newest backup into a throwaway DB, compare row counts with the DB box |
| halal-bhavcopy.timer | jobs | 14:30 UTC (20:00 IST) | load the last 6 days of NSE daily files into `exchange_daily_bars` (append-only evidence role) → `logs/bhavcopy.log` |
| halal-watchdog.timer | api | 5 min | all three boxes: containers, tunnels, API, quant hash, live flags, disk (root and /data), ingestion + prediction freshness in market hours, backup age; restarts a named broken container after 2 strikes (15 min cooldown) |

Results: `state/status.json` and `logs/alerts.log` on the **api** box. **No external notification channel yet.**

## Learning loop (see `docs/LEARNING_LOOP_SPEC.md`, page `/learning`)
Worker (box 3) at ≥ 21:00 IST: refresh breakout study → `POST quant /research/learning/run` → lessons, 24-candidate walk-forward, gates, promotion/probation, P&L audit, all stored append-only on box 2. First run 2026-11-10: **NO_CHANGE_GATES_NOT_MET** (143 out-of-sample trades; not positive after costs, did not beat control). Promoted versions are recorded decisions; no automated paper orders are placed from them yet. Watchdog warns if NSE data is > 4 days old or the learner has not run for 40 h.

## Deploying from your Mac (one command)
```
deploy/instabox/deploy.sh --dry-run        # show what would change; touches nothing
deploy/instabox/deploy.sh                  # check locally, send, rebuild ONLY what changed, verify health
deploy/instabox/deploy.sh --migrate        # same, and first apply NEW backend/migrations/*.sql (verified DB backup taken first)
deploy/instabox/deploy.sh --rollback       # put back the previous code release (migrations are NOT reverted)
```
Other flags: `--skip-migrations`, `--force` (rebuild everything), `--skip-checks`, `-y` (no prompt). One-off `--seed-state` was run on 2026-11-10.
- **Credential on the Mac:** `~/.config/halal/deploy.env` (mode 600) with `DEPLOY_API_SSH_USER=<box-1 ibssh user>`. Box 1 relays to boxes 2 and 3, so only that one string is needed locally. It is outside the repo; never commit it.
- **What it does:** blocks if any shipped file enables live trading; runs `go build/vet`, `tsc`, a Python syntax check; ships a tarball with no env/key files (and refuses if one slips in); on box 1 `fleet-deploy.sh` compares content hashes with what was last built, stops if there are un-applied migrations (unless `--migrate`/`--skip-migrations`), swaps in the new code (old copy kept in `releases/`, last 3), pushes config to boxes 2/3, rebuilds only the affected services (frontend/backend/quant on box 1, worker/quant on box 3; everything if compose/tunnel files changed), then checks health on all three boxes and runs the watchdog. Every deploy is logged in `logs/deploy.log` on box 1.
- **Rebuild rules:** backend change → `backend` (box 1) + `worker` (box 3); frontend → `frontend`; quant → `quant` on both; compose/tunnel files → everything. Test files, `*.md` and SQL migrations never trigger a rebuild. Brief per-service restarts (seconds) while a rebuilt container is replaced.
- **State on box 1:** `state/built.env` (hash each image was built from) and `state/migrations.applied`. Do not edit by hand.
- **Exit codes:** 0 ok · 3 stopped before changing anything (e.g. pending migrations) · 4 deployed but a health check failed.

## Breakouts, news and automatic paper trading (P1.9.29)
- **Upcoming (BREAKOUT_CANDIDATE) and confirmed breakouts are both tracked.** The worker (box 3) records each evening's scan: confirmed signals into `breakout_tracking`, upcoming flags into `breakout_watch`, each with the engine's reasons frozen that day (immutable). Repeat signals of the same stock inside ~5 sessions stay visible but are flagged `counts_in_stats = false`.
- **Nightly (learner run, ≥ 21:00 IST):** one engine pass over the NSE series back-fills reasons, adds replayed history flags, resolves open flags (BROKE_OUT / INVALIDATED / EXPIRED after 10 sessions), stores per-stock test results (`stock_signal_stats`), the candidate-lift study (`research_runs` kind `CANDIDATE_LIFT`), then runs the learner.
- **Auto paper trader** (`internal/workers/autopaper`, every 15 min): trades counted confirmed signals at the next session's official NSE open with the plan's stop/target/time limit (parity-tested against the Python simulator), 5 bps slippage per side, real ledger fees, ₹1 lakh per trade, max 10 open, ₹12 lakh account. Portfolios have `kind = 'AUTO_STRATEGY'`; every decision (including skips with reasons) is in `auto_paper_actions`. The frozen control always runs; a learner-promoted version gets its own portfolio.
- **News/events** (`stock_news`, `stock_filings`, `stock_events`): NSE announcements / event calendar / corporate actions (official) and Google News RSS headlines (unofficial). The worker refreshes 10 stocks per 15 min (tracked breakout stocks first); a stock page refreshes on demand when data is older than 15 min.
- **Pages:** `/breakouts` (Today, Upcoming tracked, Confirmed tracked, Auto paper trading) and tabs Breakouts / News / Events on every `/stocks/<SYMBOL>` (`?tab=news`).
- **Session checks** (`session-check.sh`, api box, weekdays, UTC): `open` 04:20 (live bars + first forecasts), `close` 10:40 (bars complete), `night` 16:00 (NSE file loaded, auto paper processed, news fresh). Failures go to `logs/alerts.log`; replay a past day with `session-check.sh open 2026-11-09`.

## Common commands
```
# any box:  cd /data/workspace/halal/app && ./dc ps            ./dc logs -f worker
# code change:  tar the repo from your Mac (exclude .env node_modules .next .git .venv) → box 1 app/, then
#   box 1: ./dc up -d --build backend frontend        box 3: ./dc up -d --build worker quant      (+ docker builder prune -af)
# from box 1:   /data/workspace/halal/fleet/fleet-sync.sh      # push app tree/config/scripts to boxes 2 and 3
```
Never run migrations twice: apply by hand on the DB box: `docker exec -i halal_equity_postgres psql -v ON_ERROR_STOP=1 -U halal_user -d halal_equity < file.sql`.

## Failover (manual)
Box 2 is the single writer's store. If it is lost: restore the newest `backups/*.dump` from box 1 or 3 onto a new Postgres exactly as `restore-drill.sh` does (roles → `timescaledb_pre_restore()` → `pg_restore` → `timescaledb_post_restore()`), then point `DB_SSH_USER` at the new box and `./dc up -d` on boxes 1 and 3. Loss window ≤ 24 h. Box 1's old local Docker volume `app_postgres_data` (data as of the 2026-11-10 cutover) is kept as a rollback copy; delete it once you trust the new layout (`docker volume rm app_postgres_data`).

## Limits
- Free-tier idle pause: pings every 10 min; all three boxes stayed up > 1 h with no gaps, but long-term effect is unverified, and using several free accounts plus keep-alives may violate Instabox terms.
- Box 2 is now a single point of failure: if it pauses/stops, the API and all jobs stop (the tunnels reconnect by themselves when it returns).
- Yahoo is the only price source (public, ~15 min delayed, unlicensed). Nothing here validates the strategy or makes it live-ready.
- Prometheus/Grafana exist in compose under profile `monitoring` (off). Root disks are 16 GB: run `docker builder prune -af` after every build.
