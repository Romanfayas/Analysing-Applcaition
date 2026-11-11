#!/bin/sh
# Forwards 0.0.0.0:5432 in this container -> 127.0.0.1:5432 on the DB box, through the Instabox SSH gateway. Reconnects forever.
: "${DB_SSH_USER:?}" "${FLEET_GATEWAY:=ssh.instabox.sh}" "${FLEET_PORT:=2222}"
delay=3
while true; do
  t0=$(date +%s)
  echo "$(date -u +%FT%TZ) tunnel: connecting to DB box"
  ssh -N -p "$FLEET_PORT" -L '*:5432:127.0.0.1:5432' \
    -o BatchMode=yes -o ExitOnForwardFailure=yes -o ServerAliveInterval=15 -o ServerAliveCountMax=3 \
    -o ConnectTimeout=20 -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/tmp/known_hosts \
    "$DB_SSH_USER@$FLEET_GATEWAY"
  rc=$?
  # a link that stayed up for a minute was healthy: start again quickly; repeated instant failures (gateway cooling down) back off to 2 min
  if [ $(( $(date +%s) - t0 )) -ge 60 ]; then delay=3; else delay=$(( delay * 2 > 120 ? 120 : delay * 2 )); fi
  echo "$(date -u +%FT%TZ) tunnel: dropped (rc=$rc), retrying in ${delay}s"; sleep "$delay"
done
