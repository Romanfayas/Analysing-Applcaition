#!/usr/bin/env bash
# Runs the Go test suite against the Dockerised Postgres.
#
# Why this exists: a host-installed Postgres (e.g. Homebrew) often owns 127.0.0.1:5432 and shadows the
# container, so "localhost" reaches the wrong server ("role halal_user does not exist"). The container is
# reachable on the machine's LAN address instead, which changes between networks, so it is discovered here.
set -euo pipefail
cd "$(dirname "$0")/../backend"

if [[ -z "${DB_HOST:-}" ]]; then
  for iface in en0 en1 eth0; do
    DB_HOST="$(ipconfig getifaddr "$iface" 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}' || true)"
    [[ -n "$DB_HOST" ]] && break
  done
fi
if [[ -z "${DB_HOST:-}" ]]; then
  echo "Could not determine DB_HOST; set it to an address that reaches the halal_equity_postgres container." >&2
  exit 2
fi
export DB_HOST
echo "Using DB_HOST=$DB_HOST"
exec go test -p 1 -count=1 "${@:-./...}"
