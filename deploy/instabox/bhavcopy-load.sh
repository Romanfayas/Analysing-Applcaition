#!/bin/sh
# JOBS box: load NSE's daily files (exchange end-of-day record) for the last few days into exchange_daily_bars, using the append-only
# evidence role. Safe to repeat: dates already present are skipped; holidays return 404 and are counted, not errors.
. "$(dirname "$0")/lib.sh"
from=$(date -u -d '-6 days' +%F); to=$(date -u +%F)
out=$(docker exec halal_equity_worker sh -c "DATABASE_URL=\"\$EVIDENCE_DATABASE_URL\" /app/bin/exchange_evidence bhavcopy -from $from -to $to" 2>&1); rc=$?
echo "$(now) rc=$rc $from..$to $(echo "$out" | grep -E '^bhavcopy:' | tail -1)" >> "$LOGDIR/bhavcopy.log"
[ $rc -eq 0 ] || { echo "$(now) FAIL: $(echo "$out" | tail -2 | tr '\n' ' ' | cut -c1-200)" >> "$LOGDIR/bhavcopy.log"; exit 1; }
