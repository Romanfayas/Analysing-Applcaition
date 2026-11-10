#!/bin/bash
# Runs on the API box (box 1), started by deploy.sh from your Mac. Do not run by hand unless you know what you are doing.
#   fleet-deploy.sh <incoming-dir> [--dry-run] [--migrate] [--skip-migrations] [--force] [--rollback] [--seed-state] [--label TEXT]
#
# What it does: decide what changed -> (optionally) apply new SQL migrations after a backup -> swap in the new code (keeping the
# old one under releases/) -> push config to the other boxes -> rebuild ONLY what changed on each box -> check health -> record.
set -uo pipefail
INC="${1:?incoming dir}"; shift
DRY=0; MIGRATE=0; SKIPMIG=0; FORCE=0; ROLLBACK=0; SEED=0; LABEL=""
while [ $# -gt 0 ]; do case "$1" in
  --dry-run) DRY=1;; --migrate) MIGRATE=1;; --skip-migrations) SKIPMIG=1;; --force) FORCE=1;; --rollback) ROLLBACK=1;; --seed-state) SEED=1;;
  --label) LABEL="${2:-}"; shift;; *) echo "unknown flag $1"; exit 2;; esac; shift; done

HALAL_HOME=/data/workspace/halal; APP=$HALAL_HOME/app; ST=$HALAL_HOME/state; REL=$HALAL_HOME/releases
# use the NEW helper library shipped with this deploy (it defines rssh, now, LOGDIR)
. "$INC/deploy/instabox/lib.sh"
[ "$FLEET_SELF" = api ] || { echo "must run on the api box"; exit 1; }
mkdir -p "$ST" "$REL" "$LOGDIR"
exec 9>/tmp/halal-deploy.lock; flock -n 9 || { echo "another deploy is running"; exit 1; }
export RSSH_TIMEOUT=300
say() { printf '%s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }
die() { printf '\nFAILED: %s\n' "$*"; exit 1; }
log() { echo "$(now) $*" >> "$LOGDIR/deploy.log"; }

# ---------------------------------------------------------------- hashing
# fhash: every file (decides whether to swap a directory); chash: only what ends up in an image (decides whether to rebuild).
fhash() { [ -d "$1" ] || { echo none; return; }; (cd "$1" && find . -type f ! -path '*/__pycache__/*' ! -name '.DS_Store' -print0 | sort -z | xargs -0 sha256sum 2>/dev/null | sha256sum | cut -c1-16); }
chash() { [ -d "$1" ] || { echo none; return; }; (cd "$1" && find . -type f ! -path '*/migrations/*' ! -name '*_test.go' ! -path '*/tests/*' ! -path '*/__pycache__/*' \
            ! -path '*/node_modules/*' ! -path '*/.next/*' ! -name '*.md' ! -name '.DS_Store' -print0 | sort -z | xargs -0 sha256sum 2>/dev/null | sha256sum | cut -c1-16); }
infra_hash() { (cd "$1" && cat docker-compose.yml 2>/dev/null; find deploy/instabox/compose deploy/instabox/tunnel -type f 2>/dev/null | sort | xargs cat 2>/dev/null) | sha256sum | cut -c1-16; }
bget() { sed -n "s/^$1=//p" "$ST/built.env" 2>/dev/null | head -1; }
bset() { touch "$ST/built.env"; grep -v "^$1=" "$ST/built.env" > "$ST/built.env.tmp" 2>/dev/null; echo "$1=$2" >> "$ST/built.env.tmp"; mv "$ST/built.env.tmp" "$ST/built.env"; }

# ---------------------------------------------------------------- planning (pure: prints what must be rebuilt from tree $1)
API_SVCS=""; JOBS_SVCS=""; API_ALL=0; JOBS_ALL=0; CH=""
plan() {
  local d="$1"; API_SVCS=""; JOBS_SVCS=""; API_ALL=0; JOBS_ALL=0; CH=""
  HB=$(chash "$d/backend"); HF=$(chash "$d/frontend"); HQ=$(chash "$d/quant"); HI=$(infra_hash "$d")
  [ "$FORCE" = 1 ] && { API_ALL=1; JOBS_ALL=1; CH="forced"; return; }
  if [ "$HI" != "$(bget infra)" ]; then API_ALL=1; JOBS_ALL=1; CH="$CH infra(compose/tunnel)"; return; fi
  [ "$HB" != "$(bget api_backend)" ]  && { API_SVCS="$API_SVCS backend";  CH="$CH backend"; }
  [ "$HF" != "$(bget api_frontend)" ] && { API_SVCS="$API_SVCS frontend"; CH="$CH frontend"; }
  [ "$HQ" != "$(bget api_quant)" ]    && { API_SVCS="$API_SVCS quant";    case "$CH" in *quant*) ;; *) CH="$CH quant";; esac; }
  [ "$HB" != "$(bget jobs_backend)" ] && JOBS_SVCS="$JOBS_SVCS worker"
  [ "$HQ" != "$(bget jobs_quant)" ]   && JOBS_SVCS="$JOBS_SVCS quant"
}
record_built() { # which boxes were built successfully
  [ "$1" = api ] && { if [ $API_ALL = 1 ]; then bset infra "$HI"; fi; bset api_backend "$HB"; bset api_frontend "$HF"; bset api_quant "$HQ"; }
  [ "$1" = jobs ] && { bset jobs_backend "$HB"; bset jobs_quant "$HQ"; }
}

# ---------------------------------------------------------------- seed (one-off): trust that the running images match this tree
if [ $SEED = 1 ]; then
  step "seeding state from the current tree (assumes the running images were built from it)"
  plan "$INC"; HB=$(chash "$INC/backend"); HF=$(chash "$INC/frontend"); HQ=$(chash "$INC/quant"); HI=$(infra_hash "$INC")
  bset infra "$HI"; bset api_backend "$HB"; bset api_frontend "$HF"; bset api_quant "$HQ"; bset jobs_backend "$HB"; bset jobs_quant "$HQ"
  : > "$ST/migrations.applied"
  for f in "$INC"/backend/migrations/*.sql; do b=$(basename "$f"); [ "$b" = init.sql ] && continue; echo "$b $(sha256sum "$f" | cut -c1-16)" >> "$ST/migrations.applied"; done
  say "recorded $(wc -l < "$ST/migrations.applied") migrations as already applied"; cat "$ST/built.env"; log "seeded state"; exit 0
fi
[ -f "$ST/built.env" ] && [ -f "$ST/migrations.applied" ] || die "no deploy state yet. Run once with --seed-state (only if the servers already run this code)."

# ---------------------------------------------------------------- rollback: restore the newest release, then fall through to the build step
if [ $ROLLBACK = 1 ]; then
  step "rollback"
  last=$(ls -1d "$REL"/*/ 2>/dev/null | sort | tail -1); [ -n "$last" ] || die "no previous release is stored"
  say "restoring components from $(basename "$last"): $(ls "$last" | tr '\n' ' ')"
  [ $DRY = 1 ] && { say "(dry run: nothing changed)"; exit 0; }
  TS=rb_$(date -u +%Y%m%dT%H%M%S); mkdir -p "$REL/$TS"
  for c in $(ls "$last"); do mv "$APP/$c" "$REL/$TS/$c" && mv "$last/$c" "$APP/$c"; done
  rmdir "$last" 2>/dev/null
  say "NOTE: database migrations are NOT reverted. Restore a backup from $HALAL_HOME/backups only if the old code cannot work with the new schema."
  SRC="$APP"
else
  SRC="$INC"
fi

# ---------------------------------------------------------------- plan
step "what changed"
plan "$SRC"
say "code components needing a rebuild:${CH:- none}"
say "api box (frontend/backend/quant): $([ $API_ALL = 1 ] && echo 'everything' || echo "${API_SVCS:- nothing}")"
say "jobs box (worker/quant):          $([ $JOBS_ALL = 1 ] && echo 'everything' || echo "${JOBS_SVCS:- nothing}")"

NEWMIG=()
if [ $ROLLBACK = 0 ]; then
  for f in "$INC"/backend/migrations/*.sql; do b=$(basename "$f"); [ "$b" = init.sql ] && continue
    if ! grep -q "^$b " "$ST/migrations.applied"; then NEWMIG+=("$b")
    elif [ "$(sha256sum "$f" | cut -c1-16)" != "$(grep "^$b " "$ST/migrations.applied" | cut -d' ' -f2)" ]; then say "WARNING: applied migration $b changed on disk; it is NOT re-applied"; fi; done
  say "new SQL migrations: ${#NEWMIG[@]} ${NEWMIG[*]:-}"
fi
other=0; for c in docs scripts reports data; do [ "$(fhash "$INC/$c")" != "$(fhash "$APP/$c")" ] && other=1; done
[ -z "$CH" ] && [ ${#NEWMIG[@]} = 0 ] && [ "$(fhash "$INC/deploy")" = "$(fhash "$APP/deploy")" ] && [ $other = 0 ] && [ $FORCE = 0 ] && [ $ROLLBACK = 0 ] && { say; say "Nothing to deploy: the servers already match."; exit 0; }

if [ ${#NEWMIG[@]} -gt 0 ] && [ $MIGRATE = 0 ] && [ $SKIPMIG = 0 ] && [ $ROLLBACK = 0 ]; then
  say; say "STOPPED before changing anything: ${#NEWMIG[@]} new migration(s) are waiting:"; printf '   %s\n' "${NEWMIG[@]}"
  say "Re-run with --migrate to back up the database and apply them first (recommended),"
  say "or --skip-migrations to deploy the code without applying them (only if the code does not need them)."
  exit 3
fi
[ $DRY = 1 ] && { say; say "(dry run: nothing was changed)"; exit 0; }

# ---------------------------------------------------------------- migrations
if [ ${#NEWMIG[@]} -gt 0 ] && [ $MIGRATE = 1 ]; then
  step "database backup before migrating"
  rssh db "$HALAL_HOME/fleet/backup-db.sh" >/dev/null 2>&1; bk=$(rssh db "tail -1 $LOGDIR/backup.log")
  say "$bk"; case "$bk" in *"backup ok"*) ;; *) die "could not take a verified backup; no migration was applied";; esac
  for b in "${NEWMIG[@]}"; do
    step "applying $b"
    if rssh db "docker exec -i halal_equity_postgres psql -v ON_ERROR_STOP=1 -q -U halal_user -d halal_equity" < "$INC/backend/migrations/$b" > "$LOGDIR/migrate_$b.log" 2>&1; then
      echo "$b $(sha256sum "$INC/backend/migrations/$b" | cut -c1-16)" >> "$ST/migrations.applied"; say "ok"; log "migration $b applied"
    else tail -8 "$LOGDIR/migrate_$b.log"; log "migration $b FAILED"; die "migration $b failed; stopped. Nothing else was deployed. Restore with the backup above if the database changed."; fi
  done
fi

# ---------------------------------------------------------------- swap code in
if [ $ROLLBACK = 0 ]; then
  step "installing the new code"
  TS=$(date -u +%Y%m%dT%H%M%S); mkdir -p "$REL/$TS"
  for c in backend frontend quant deploy data; do
    if [ -d "$INC/$c" ] && [ "$(fhash "$INC/$c")" != "$(fhash "$APP/$c")" ]; then
      [ -d "$APP/$c" ] && mv "$APP/$c" "$REL/$TS/$c"; mv "$INC/$c" "$APP/$c"; say "  replaced $c"
    fi
  done
  # everything else (docs, scripts, compose file, readmes, reports): overlay, never deleting .env / dc / role / override files
  (cd "$INC" && tar -cf - --exclude=./backend --exclude=./frontend --exclude=./quant --exclude=./deploy --exclude=./data .) | tar -C "$APP" -xf - --no-same-owner 2>/dev/null
  rmdir "$REL/$TS" 2>/dev/null; ls -1d "$REL"/*/ 2>/dev/null | sort | head -n -3 | xargs -r rm -rf
  rm -rf "$INC"
fi
rm -rf "$HALAL_HOME/fleet" && mkdir -p "$HALAL_HOME/fleet" && cp -r "$APP/deploy/instabox/." "$HALAL_HOME/fleet/" && chmod +x "$HALAL_HOME"/fleet/*.sh "$HALAL_HOME"/fleet/*.py
chmod 600 "$APP/.env" 2>/dev/null
"$HALAL_HOME/fleet/install-units.sh" api >/dev/null 2>&1 || say "WARNING: could not refresh the api box's timers"
# the tree on disk is now authoritative: re-plan from it so the record below is exact
plan "$APP"

step "pushing config and scripts to the other boxes"
/data/workspace/halal/fleet/fleet-sync.sh 2>&1 | grep -vE '^(Created symlink|Removed)|halal-.*(timer|service)' | sed 's/^/   /'

# ---------------------------------------------------------------- rebuild
start_build() { # role  ("everything" or list)
  local role="$1" svcs="$2"
  rssh "$role" "bash -s" <<EOF
cd $APP && : > $LOGDIR/build.log
nohup sh -c "./dc up -d --build $svcs > $LOGDIR/build.log 2>&1; echo exit=\\\$? >> $LOGDIR/build.log" >/dev/null 2>&1 &
EOF
}
wait_build() { # role -> 0 ok
  local role="$1" i=0 r
  while [ $i -lt 120 ]; do
    r=$(rssh "$role" "grep -h '^exit=' $LOGDIR/build.log 2>/dev/null | tail -1" 2>/dev/null)
    [ -n "$r" ] && break; sleep 15; i=$((i+1))
  done
  [ "$r" = "exit=0" ] && { rssh "$role" "docker builder prune -af >/dev/null 2>&1; df -h / | tail -1 | awk '{print \"   disk \"\$5\" used\"}'"; return 0; }
  say "   build on $role did not finish OK (${r:-timeout}). Last log lines:"; rssh "$role" "tail -15 $LOGDIR/build.log | cut -c1-160" | sed 's/^/   | /'; return 1
}
step "rebuilding"
RC=0; pa=0; pj=0
if [ $API_ALL = 1 ] || [ -n "$API_SVCS" ]; then say "api box: $([ $API_ALL = 1 ] && echo everything || echo $API_SVCS)"; start_build api "$([ $API_ALL = 1 ] && echo '' || echo $API_SVCS)"; pa=1; fi
if [ $JOBS_ALL = 1 ] || [ -n "$JOBS_SVCS" ]; then say "jobs box: $([ $JOBS_ALL = 1 ] && echo everything || echo $JOBS_SVCS)"; start_build jobs "$([ $JOBS_ALL = 1 ] && echo '' || echo $JOBS_SVCS)"; pj=1; fi
[ $pa = 0 ] && [ $pj = 0 ] && say "no image needs rebuilding"
if [ $pa = 1 ]; then if wait_build api; then record_built api; say "   api box built"; else RC=1; fi; fi
if [ $pj = 1 ]; then if wait_build jobs; then record_built jobs; say "   jobs box built"; else RC=1; fi; fi
[ $RC = 0 ] || { log "deploy FAILED during build"; die "a build failed. The previous images are still running where the build did not finish; fix the code and deploy again, or use --rollback."; }

# ---------------------------------------------------------------- verify
step "health checks"
HC=0
ck() { if [ "$2" = "$3" ]; then say "   ok    $1"; else say "   FAIL  $1 (got '$2', want '$3')"; HC=1; fi; }
sleep 8
ck "api: backend /health"        "$(curl -s -m 10 -o /dev/null -w %{http_code} localhost:8081/health)" 200
ck "api: frontend /login"        "$(curl -s -m 15 -o /dev/null -w %{http_code} localhost:3000/login)" 200
ck "api: strategy hash pinned"   "$(curl -s -m 10 localhost:8000/health | grep -c '"strategy_matches_pinned":true')" 1
ck "api: tunnel to database"     "$(docker inspect -f '{{.State.Health.Status}}' halal_equity_postgres 2>/dev/null)" healthy
ck "jobs: worker running"        "$(rssh jobs "docker inspect -f '{{.State.Status}}' halal_equity_worker" 2>/dev/null)" running
ck "jobs: strategy hash pinned"  "$(rssh jobs "curl -s -m 10 localhost:8000/health | grep -c '\"strategy_matches_pinned\":true'" 2>/dev/null)" 1
ck "db: postgres"                "$(rssh db "docker inspect -f '{{.State.Health.Status}}' halal_equity_postgres" 2>/dev/null)" healthy
ck "live trading flags off"      "$( (for c in backend quant; do docker exec halal_equity_$c env; done; rssh jobs "docker exec halal_equity_worker env; docker exec halal_equity_quant env") 2>/dev/null | grep -cE '^(LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED)=true')" 0
/data/workspace/halal/fleet/fleet-watchdog.sh >/dev/null 2>&1; say "   watchdog: $(cat "$ST/status.json")"
log "deploy $([ $ROLLBACK = 1 ] && echo rollback || echo ok) label='$LABEL' changed:${CH:- none} migrations=${NEWMIG[*]:-none} health=$HC"
[ $HC = 0 ] && { step "done"; exit 0; } || { step "deployed, but a health check failed (see above). Use --rollback if the app is broken."; exit 4; }
