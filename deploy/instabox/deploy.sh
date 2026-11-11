#!/usr/bin/env bash
# Deploy this local checkout to the three Instabox servers.  Run from anywhere:   deploy/instabox/deploy.sh [flags]
#
#   (no flags)         check the code locally, send it to the servers, rebuild only what changed, verify health
#   --dry-run          show exactly what would change; change nothing on the servers
#   --migrate          also apply NEW SQL migrations (a verified database backup is taken first)
#   --skip-migrations  deploy the code even though new migrations exist (only if the code does not need them)
#   --force            rebuild everything even if nothing changed
#   --rollback         put back the previous release (code only; database migrations are NOT reverted)
#   --skip-checks      skip the local build/type checks
#   --seed-state       ONE-OFF: tell the servers that what they run now equals this checkout
#   -y                 do not ask for confirmation
#
# Needs one credential on this Mac: ~/.config/halal/deploy.env (mode 600) with
#   DEPLOY_API_SSH_USER=ibssh_...      # the box-1 (api) Instabox SSH user; it is a password, never commit it
# Optional: DEPLOY_APP_URL=https://...   DEPLOY_GATEWAY=ssh.instabox.sh   DEPLOY_PORT=2222
# Box 1 relays to boxes 2 and 3, so their credentials never leave the servers.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
CFG="${HALAL_DEPLOY_ENV:-$HOME/.config/halal/deploy.env}"
# shellcheck disable=SC1090
[ -f "$CFG" ] && . "$CFG"
: "${DEPLOY_API_SSH_USER:?Missing DEPLOY_API_SSH_USER. Create $CFG (chmod 600) containing: DEPLOY_API_SSH_USER=ibssh_...}"
GATEWAY="${DEPLOY_GATEWAY:-ssh.instabox.sh}"; PORT="${DEPLOY_PORT:-2222}"
if [ -f "$CFG" ] && [ "$(stat -f %Lp "$CFG" 2>/dev/null || stat -c %a "$CFG")" != "600" ]; then echo "warning: $CFG should be mode 600 (chmod 600 $CFG)"; fi

REMOTE_FLAGS=(); SKIPCHECKS=0; YES=0; DRY=0
for a in "$@"; do case "$a" in
  --dry-run) DRY=1; REMOTE_FLAGS+=("$a");;
  --migrate|--skip-migrations|--force|--rollback|--seed-state) REMOTE_FLAGS+=("$a");;
  --skip-checks) SKIPCHECKS=1;;
  -y|--yes) YES=1;;
  -h|--help) sed -n 2,19p "$0"; exit 0;;
  *) echo "unknown flag: $a (see --help)"; exit 2;; esac; done

SSH=(ssh -p "$PORT" -o BatchMode=yes -o ConnectTimeout=30 -o ServerAliveInterval=15 -o ServerAliveCountMax=8 "$DEPLOY_API_SSH_USER@$GATEWAY")
say() { printf '%s\n' "$*"; }
fail() { printf '\nSTOPPED: %s\n' "$*" >&2; exit 1; }
ROLLBACK=0; for a in "${REMOTE_FLAGS[@]:-}"; do [ "$a" = --rollback ] && ROLLBACK=1; done

cd "$REPO"
LABEL="$(git rev-parse --short HEAD 2>/dev/null || echo nogit)"
[ -z "$(git status --porcelain 2>/dev/null)" ] || LABEL="$LABEL+local-changes"

# ---- 1. safety guard: live trading must never be enabled by a file we are about to ship
step() { printf '\n== %s\n' "$*"; }
step "safety guard"
if grep -rnE '^[[:space:]-]*(LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED)[[:space:]]*[=:][[:space:]]*"?(true|1|yes|on)"?[[:space:]]*$' \
     docker-compose.yml deploy .env.example 2>/dev/null | grep -v '^deploy/instabox/deploy.sh'; then
  fail "a file enables LIVE_ORDER_EXECUTION or BROKER_TRADING_ENABLED. Live trading must stay disabled; nothing was sent."
fi
say "live trading flags are off in every file being shipped"

# ---- 2. local checks (fast; cached)
SEED=0; for a in "${REMOTE_FLAGS[@]:-}"; do [ "$a" = --seed-state ] && SEED=1; done
if [ $SKIPCHECKS = 0 ] && [ $ROLLBACK = 0 ] && [ $SEED = 0 ]; then
  step "local checks (use --skip-checks to skip)"
  (cd backend && go build ./... && go vet ./cmd/... ./internal/... >/dev/null) || fail "backend does not build or vet"; say "backend: build + vet ok"
  if [ -x frontend/node_modules/.bin/tsc ]; then (cd frontend && ./node_modules/.bin/tsc --noEmit) || fail "frontend has type errors"; say "frontend: type-check ok"; else say "frontend: tsc not installed locally, skipped"; fi
  python3 - <<'PY' || fail "a Python file in quant/ has a syntax error"
import ast, pathlib, sys
bad = 0
for p in pathlib.Path("quant/src").rglob("*.py"):
    try: ast.parse(p.read_text())
    except SyntaxError as e: print(f"  {p}: {e}"); bad = 1
sys.exit(bad)
PY
  say "quant: syntax ok"
fi

# ---- 3. package (no secrets, no build output) and send
step "packaging $LABEL"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
COPYFILE_DISABLE=1 tar --exclude=node_modules --exclude=.next --exclude=.git --exclude=backups --exclude=.env --exclude='.env.*' --exclude=.venv \
  --exclude=.DS_Store --exclude=worker.log --exclude=__pycache__ --exclude='*.pyc' --exclude=backend/bin --exclude=docker-compose.server.yml \
  -czf "$TMP/app.tgz" .
# last line of defence: the archive must not contain env files or key material
if tar -tzf "$TMP/app.tgz" | grep -E '(^|/)\.env($|\.local)|\.pem$|id_rsa|id_ed25519|fleet\.env|deploy\.env'; then fail "the archive contains a secret-looking file (listed above); nothing was sent"; fi
say "archive $(du -h "$TMP/app.tgz" | cut -f1), contains no env or key files"

if [ $YES = 0 ] && [ $DRY = 0 ]; then
  printf '\nSend to the servers now? [y/N] '; read -r ans; [ "$ans" = y ] || [ "$ans" = Y ] || { say "cancelled"; exit 0; }
fi
step "uploading"
"${SSH[@]}" 'mkdir -p /data/workspace/halal && cat > /data/workspace/halal/incoming.tgz' < "$TMP/app.tgz"

# ---- 4. run the orchestrator that ships with this very checkout (so changes to it take effect immediately)
step "deploying (output from the servers)"
set +e
"${SSH[@]}" "set -e; I=/data/workspace/halal/incoming; rm -rf \$I; mkdir -p \$I; tar -xzf /data/workspace/halal/incoming.tgz -C \$I --no-same-owner 2>/dev/null; test -f \$I/deploy/instabox/fleet-deploy.sh; rm -f /data/workspace/halal/incoming.tgz; set +e; bash \$I/deploy/instabox/fleet-deploy.sh \$I ${REMOTE_FLAGS[*]:-} --label '$LABEL'"
RC=$?
set -e
case $RC in
  0) say; if [ $DRY = 1 ]; then say "Dry run finished: nothing was changed."; else say "Deployed ${DEPLOY_APP_URL:+→ $DEPLOY_APP_URL}"; fi;;
  3) say; say "Nothing was changed on the servers (see the message above).";;
  4) say; say "Deployed, but a health check failed. Look at the output above; --rollback restores the previous release.";;
  *) say; say "Deploy did not complete (exit $RC). Output above. The servers log every deploy in /data/workspace/halal/logs/deploy.log on box 1.";;
esac
exit $RC
