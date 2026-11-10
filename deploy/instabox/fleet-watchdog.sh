#!/bin/sh
# API box: checks every flow on all three boxes every 5 min. Writes state/status.json and logs/alerts.log.
# Restarts only a specific broken container (2 consecutive strikes, 15 min cooldown per box).
. "$(dirname "$0")/lib.sh"
ST=$HALAL_HOME/state; AL=$LOGDIR/alerts.log
alert() { echo "$(now) $1 $2" >> "$AL"; }
PSQL='docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X -At'
CONT='for c in %s; do echo "c_$c=$(docker inspect -f "{{.State.Status}}/{{if .State.Health}}{{.State.Health.Status}}{{else}}-{{end}}/{{.RestartCount}}" halal_equity_$c 2>/dev/null || echo missing)"; done'
FLAGS='echo "live_flags_true=$(for c in %s; do docker exec halal_equity_$c env | grep -E "^(LIVE_ORDER_EXECUTION|BROKER_TRADING_ENABLED)=true"; done | wc -l)"'
DISK='echo "disk_root=$(df --output=pcent / | tail -1 | tr -d " %")"; echo "disk_data=$(df --output=pcent /data | tail -1 | tr -d " %")"'
get() { echo "$1" | sed -n "s/^$2=//p" | head -1; }
check_box() { # role containers flag-containers extra-script
  role="$1"; out=$(RSSH_TIMEOUT=90 rssh "$role" "$(printf "$CONT" "$2"); $(printf "$FLAGS" "$3"); $DISK; $4" 2>&1); rc=$?
  if [ $rc -ne 0 ] || ! echo "$out" | grep -q '^disk_root='; then
    n=$(( $(cat $ST/unreach_$role 2>/dev/null || echo 0) + 1 )); echo $n > $ST/unreach_$role
    alert WARN "$role box unreachable over ssh (rc=$rc, strike $n)"; eval "OUT_$role="; return 1; fi
  echo 0 > $ST/unreach_$role; eval "OUT_$role=\$out"
  bad=""
  for c in $2; do v=$(get "$out" c_$c); case "$v" in running/healthy/*|running/-/*) ;; *) bad="$bad $c($v)";; esac; done
  [ "$(get "$out" live_flags_true)" = 0 ] || alert CRITICAL "$role: LIVE_ORDER_EXECUTION/BROKER_TRADING_ENABLED is TRUE in a service"
  for k in disk_root disk_data; do d=$(get "$out" $k); [ "${d:-0}" -ge 85 ] && alert WARN "$role $k ${d}% used"; done
  eval "BAD_$role=\"$bad\""; return 0
}
ist_dow=$(TZ=Asia/Kolkata date +%u); ist_hm=$(TZ=Asia/Kolkata date +%H%M)
market_open=0; [ "$ist_dow" -le 5 ] && [ "$ist_hm" -ge 0935 ] && [ "$ist_hm" -le 1520 ] && market_open=1

check_box db "postgres" "postgres" "echo \"pg_conns=\$($PSQL -c 'select count(*) from pg_stat_activity'  2>/dev/null)\"; echo \"last_backup=\$(ls -1t $HALAL_HOME/backups/*.dump 2>/dev/null | head -1 | xargs -r stat -c %Y)\"; echo \"intraday_age_min=\$($PSQL -c 'select coalesce(extract(epoch from now()-max(retrieved_at))/60,-1)::int from ohlcv_intraday' 2>/dev/null)\"; echo \"pred_age_min=\$($PSQL -c 'select coalesce(extract(epoch from now()-max(issued_at))/60,-1)::int from predictions' 2>/dev/null)\"; echo \"bhav_lag_days=\$($PSQL -c 'select coalesce(current_date-max(trade_date),-1) from exchange_daily_bars' 2>/dev/null)\"; echo \"learn_age_h=\$($PSQL -c 'select coalesce(extract(epoch from now()-max(created_at))/3600,-1)::int from learning_runs' 2>/dev/null)\""
check_box api "postgres redis backend frontend quant" "backend quant" "echo backend=\$(curl -s -m5 -o /dev/null -w %{http_code} localhost:8081/health); echo frontend=\$(curl -s -m8 -o /dev/null -w %{http_code} localhost:3000/login); echo strategy_ok=\$(curl -s -m5 localhost:8000/health | grep -c '\"strategy_matches_pinned\":true')"
check_box jobs "postgres worker quant" "worker quant" "echo strategy_ok=\$(curl -s -m5 localhost:8000/health | grep -c '\"strategy_matches_pinned\":true'); echo worker_up_s=\$(docker inspect -f '{{.State.StartedAt}}' halal_equity_worker 2>/dev/null | xargs -I{} date -d {} +%s 2>/dev/null)"

# cross-box checks
[ -n "$OUT_api" ] && { [ "$(get "$OUT_api" backend)" = 200 ] || BAD_api="$BAD_api backend-http"; [ "$(get "$OUT_api" frontend)" = 200 ] || BAD_api="$BAD_api frontend-http"; [ "$(get "$OUT_api" strategy_ok)" = 1 ] || alert CRITICAL "api: strategy hash != pinned Strategy V2"; }
[ -n "$OUT_jobs" ] && [ "$(get "$OUT_jobs" strategy_ok)" != 1 ] && alert CRITICAL "jobs: strategy hash != pinned Strategy V2"
if [ -n "$OUT_db" ]; then
  age=$(get "$OUT_db" intraday_age_min); pa=$(get "$OUT_db" pred_age_min); b=$(get "$OUT_db" last_backup)
  [ $market_open = 1 ] && [ "${age:-0}" -gt 20 ] && alert WARN "ingestion stalled: newest intraday row touched ${age} min ago during market hours (jobs box worker?)"
  [ $market_open = 1 ] && [ "${pa:-0}" -gt 30 ] && alert WARN "prediction engine stalled: newest forecast issued ${pa} min ago during market hours"
  [ -n "$b" ] && [ $(( $(date +%s) - b )) -gt 100000 ] && alert WARN "last database backup older than 27 h"
  bl=$(get "$OUT_db" bhav_lag_days); [ "${bl:--1}" -gt 4 ] && alert WARN "NSE daily data is ${bl} days old: the learning loop cannot see new outcomes (check logs/bhavcopy.log on the jobs box)"
  la=$(get "$OUT_db" learn_age_h); [ "${la:--1}" -gt 40 ] && alert WARN "the learning loop has not run for ${la} h"
fi
# self-heal: restart a named broken container after 2 strikes
for role in api db jobs; do
  eval "bad=\$BAD_$role"; f=$ST/strikes_$role
  if [ -n "$bad" ]; then
    n=$(( $(cat $f 2>/dev/null || echo 0) + 1 )); echo $n > $f; alert WARN "$role unhealthy:$bad (strike $n)"
    last=$(cat $ST/restart_$role 2>/dev/null || echo 0)
    if [ $n -ge 2 ] && [ $(( $(date +%s) - last )) -gt 900 ]; then
      for item in $bad; do c=${item%%(*}; case $c in postgres|redis|backend|worker|quant|frontend) rssh $role "docker restart halal_equity_$c" >/dev/null 2>&1; alert ACTION "$role: restarted $c";; esac; done
      date +%s > $ST/restart_$role
    fi
  else echo 0 > $f; fi
done
printf '{"ts":"%s","market_open":%s,"api_bad":"%s","db_bad":"%s","jobs_bad":"%s","intraday_age_min":"%s","pred_age_min":"%s"}\n' \
  "$(now)" "$market_open" "$(echo $BAD_api | tr -d '"')" "$(echo $BAD_db | tr -d '"')" "$(echo $BAD_jobs | tr -d '"')" "$(get "$OUT_db" intraday_age_min)" "$(get "$OUT_db" pred_age_min)" > $ST/status.json
exit 0
