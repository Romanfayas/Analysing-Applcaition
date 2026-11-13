#!/usr/bin/env bash
# Wakes the fleet from outside it. A suspended Instabox sandbox cannot wake itself, and all three stop together after ~8 h of run time.
# Connecting to box 1 starts it; its own keep-alive step (fleet-ping.sh) then connects to boxes 2 and 3 and starts them.
# Used by .github/workflows/fleet-wake.yml (credential in the FLEET_API_SSH_USER secret) and runnable by hand from the Mac:
#   deploy/instabox/wake.sh        (reads DEPLOY_API_SSH_USER from ~/.config/halal/deploy.env)
set -uo pipefail
USER_="${FLEET_API_SSH_USER:-${DEPLOY_API_SSH_USER:-}}"
if [ -z "$USER_" ] && [ -f "$HOME/.config/halal/deploy.env" ]; then . "$HOME/.config/halal/deploy.env"; USER_="${DEPLOY_API_SSH_USER:-}"; fi
[ -n "$USER_" ] || { echo "no credential: set FLEET_API_SSH_USER (GitHub secret) or DEPLOY_API_SSH_USER in ~/.config/halal/deploy.env"; exit 2; }
for try in 1 2 3; do
  out=$(ssh -p "${DEPLOY_PORT:-2222}" -o BatchMode=yes -o ConnectTimeout=45 -o ServerAliveInterval=10 -o StrictHostKeyChecking=accept-new "$USER_@${DEPLOY_GATEWAY:-ssh.instabox.sh}" \
    'sh /data/workspace/halal/fleet/fleet-ping.sh; tail -2 /data/workspace/halal/logs/fleet-ping.jsonl' 2>&1) && { echo "$out" | sed 's/ibssh_[A-Za-z0-9]*/ibssh_***/g'; exit 0; }
  echo "attempt $try failed: $(echo "$out" | tail -3 | tr '\n' ' ' | sed 's/ibssh_[A-Za-z0-9]*/ibssh_***/g')"; sleep 20
done
exit 1
