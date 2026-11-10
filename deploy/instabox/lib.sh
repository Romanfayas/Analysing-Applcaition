#!/bin/sh
# Shared helpers. Credentials live ONLY in ~/.config/halal/fleet.env (mode 600), never in the repo.
# Roles: api = box 1 (frontend+backend), db = box 2 (Postgres), jobs = box 3 (worker + quant).
. "$HOME/.config/halal/fleet.env"
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
HALAL_HOME=/data/workspace/halal
LOGDIR=$HALAL_HOME/logs; mkdir -p "$LOGDIR" "$HALAL_HOME/state"
now() { date -u +%Y-%m-%dT%H:%M:%SZ; }
role_user() { case "$1" in api) echo "$FLEET_MAIN";; db) echo "$FLEET_STANDBY";; jobs) echo "$FLEET_MONITOR";; esac; }
role_url()  { case "$1" in api) echo "$FLEET_URL_MAIN";; db) echo "$FLEET_URL_STANDBY";; jobs) echo "$FLEET_URL_MONITOR";; esac; }
# rssh <role> <command...>: run a command on a fleet box (local shell if it is this box)
rssh() { r="$1"; shift
  if [ "$r" = "$FLEET_SELF" ]; then sh -c "$*"; return; fi
  timeout "${RSSH_TIMEOUT:-60}" ssh -p "$FLEET_PORT" -o BatchMode=yes -o ConnectTimeout=20 -o ServerAliveInterval=10 \
    -o StrictHostKeyChecking=accept-new "$(role_user "$r")@$FLEET_GATEWAY" "$@"; }
