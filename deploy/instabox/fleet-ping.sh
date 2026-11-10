#!/bin/sh
# Keep-alive: every box touches the other two (SSH exec + public HTTP health) and logs the result as JSON lines.
. "$(dirname "$0")/lib.sh"
LOG=$LOGDIR/fleet-ping.jsonl
for role in api db jobs; do
  [ "$role" = "$FLEET_SELF" ] && continue
  s0=$(date +%s)
  out=$(RSSH_TIMEOUT=50 rssh "$role" 'echo up=$(cut -d" " -f1 /proc/uptime) disk=$(df --output=pcent / | tail -1 | tr -d " %")' 2>&1); rc=$?
  s1=$(date +%s)
  url=$(role_url "$role"); path=/health; [ "$role" = api ] && path=/login
  code=$(curl -s -m 25 -o /dev/null -w '%{http_code}' "$url$path" 2>/dev/null)
  printf '{"ts":"%s","from":"%s","to":"%s","ssh_rc":%s,"ssh_s":%s,"http":"%s","detail":"%s"}\n' \
    "$(now)" "$FLEET_SELF" "$role" "$rc" "$((s1-s0))" "${code:-000}" "$(echo "$out" | tail -1 | tr -d '"\\' | cut -c1-100)" >> "$LOG"
done
# keep the log bounded
[ "$(wc -l < "$LOG")" -gt 5000 ] && tail -n 2000 "$LOG" > "$LOG.tmp" && mv "$LOG.tmp" "$LOG"
exit 0
