#!/bin/bash
# API box: verifies each stage of the daily pipeline actually happened, and says so loudly when it did not.
#   session-check.sh open  [YYYY-MM-DD]   ~09:50 IST: live bars arriving, first forecasts issued
#   session-check.sh close [YYYY-MM-DD]   ~16:10 IST: the session's bars are complete, forecasts resolved
#   session-check.sh night [YYYY-MM-DD]   ~21:30 IST: NSE file loaded, breakouts recorded, auto paper trader and learner ran
# The optional date lets you replay a past day to see what the check would have said.
. "$(dirname "$0")/lib.sh"
MODE="${1:?open|close|night}"; DAY="${2:-$(TZ=Asia/Kolkata date +%F)}"
CAL="$HALAL_HOME/app/quant/data/nse_calendar_v1.json"
OUT="$LOGDIR/session-check.log"; AL="$LOGDIR/alerts.log"
say() { echo "$(now) [$MODE $DAY] $*" | tee -a "$OUT"; }
# a replay of a named past day reports problems but does not raise real alerts
bad() { if [ -n "${2:-}" ] || [ -n "${REPLAY:-}" ]; then echo "$(now) (replay) ALERT [$MODE $DAY] $1" | tee -a "$OUT"; else echo "$(now) ALERT [$MODE $DAY] $1" | tee -a "$OUT" >> "$AL"; fi; FAILED=1; }
FAILED=0
[ -n "${2:-}" ] && REPLAY=1
dow=$(date -d "$DAY" +%u)
if [ "$dow" -ge 6 ] && ! grep -q "\"$DAY\"" "$CAL"; then say "weekend: market closed, nothing to check"; exit 0; fi
if python3 - "$DAY" "$CAL" <<'PY'
import json, sys
d, cal = sys.argv[1], json.load(open(sys.argv[2]))
sys.exit(0 if d in cal["full_day_holidays"] else 1)
PY
then say "NSE holiday: market closed, nothing to check"; exit 0; fi
PSQL() { rssh db "docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X -At -c \"$1\"" 2>/dev/null; }
D="(timestamp at time zone 'Asia/Kolkata')::date = '$DAY'"

case "$MODE" in
 open)
   syms=$(PSQL "select count(distinct symbol_id) from ohlcv_intraday where $D")
   age=$(PSQL "select coalesce(round(extract(epoch from now()-max(timestamp))/60),-1) from ohlcv_intraday where $D")
   preds=$(PSQL "select count(*) from predictions where (issued_at at time zone 'Asia/Kolkata')::date = '$DAY'")
   say "stocks with live bars today: ${syms:-0}; newest bar ${age:-?} min old; forecasts issued today: ${preds:-0}"
   [ "${syms:-0}" -ge 45 ] || bad "live bars arrived for only ${syms:-0} of ~51 stocks"
   [ "${age:--1}" -ge 0 ] && [ "${age:-99}" -le 12 ] || bad "newest live bar is ${age:-unknown} min old (limit 12)"
   [ "${preds:-0}" -gt 0 ] || bad "no forecasts issued yet today: prediction engine is silent"
   ;;
 close)
   med=$(PSQL "select coalesce(round(percentile_cont(0.5) within group (order by n)),0) from (select count(*) n from ohlcv_intraday where $D group by symbol_id) x")
   short=$(PSQL "select count(*) from (select symbol_id, count(*) n from ohlcv_intraday where $D group by symbol_id having count(*) < 330) x")
   res=$(PSQL "select count(*) filter (where resolved_at is not null) || '/' || count(*) from predictions where (issued_at at time zone 'Asia/Kolkata')::date = '$DAY'")
   say "median 1-minute bars per stock: ${med:-0} (full session ~375); stocks with under 330: ${short:-?}; forecasts resolved/issued: ${res:-0/0}"
   [ "${med:-0}" -ge 360 ] || bad "the session's bars are incomplete (median ${med:-0} of ~375)"
   ;;
 night)
   bh=$(PSQL "select count(*) from exchange_daily_bars where trade_date = '$DAY'")
   lr=$(PSQL "select count(*) from learning_runs where (created_at at time zone 'Asia/Kolkata')::date >= '$DAY'")
   tr=$(PSQL "select count(*) from breakout_tracking where signal_date = '$DAY'")
   wf=$(PSQL "select count(*) from breakout_watch where flagged_date = '$DAY'")
   ap=$(PSQL "select coalesce(string_agg(to_char(processed_through,'YYYY-MM-DD'), ','), 'none') from auto_paper_state")
   nw=$(PSQL "select count(*) from stock_news where fetched_at > now() - interval '3 hours'")
   say "NSE daily file rows for the day: ${bh:-0}; confirmed breakouts recorded: ${tr:-0}; upcoming flags recorded: ${wf:-0}; auto paper processed through: ${ap}; learner runs since: ${lr:-0}; headlines fetched in last 3h: ${nw:-0}"
   [ "${bh:-0}" -ge 40 ] || bad "the NSE daily file for $DAY is not loaded (${bh:-0} rows): the learner and auto paper trader cannot see today"
   case "$ap" in *"$DAY"*) ;; *) bad "the automatic paper trader has not processed $DAY (at: $ap)";; esac
   [ "${nw:-0}" -gt 0 ] || bad "no news headlines were fetched in the last 3 hours"
   ;;
esac
[ $FAILED = 0 ] && say "OK" || say "FAILED: see ALERT lines above"
exit $FAILED
