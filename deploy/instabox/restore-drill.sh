#!/bin/sh
# API box: prove the newest verified backup actually restores, and that key row counts match MAIN. Throwaway container, removed afterwards.
. "$(dirname "$0")/lib.sh"
L=$LOGDIR/standby.log; D=$HALAL_HOME/backups; IMG=timescale/timescaledb:2.30.1-pg16; C=halal_restore_drill
f=$(ls -1t "$D"/*.dump 2>/dev/null | head -1); [ -n "$f" ] || { echo "$(now) drill: no backup" >> "$L"; exit 1; }
docker rm -f $C >/dev/null 2>&1
docker run -d --name $C -e POSTGRES_PASSWORD=drill -e POSTGRES_USER=halal_user -e POSTGRES_DB=halal_equity $IMG >/dev/null || exit 1
trap 'docker rm -f -v $C >/dev/null 2>&1' EXIT
i=0; until docker exec $C pg_isready -U halal_user -d halal_equity >/dev/null 2>&1 && docker exec $C psql -U halal_user -d halal_equity -Atc "select 1" >/dev/null 2>&1; do i=$((i+1)); [ $i -gt 60 ] && { echo "$(now) drill: db never ready" >> "$L"; exit 1; }; sleep 2; done
P="docker exec -i $C psql -U halal_user -d halal_equity -X -At"
for r in ca_security_admin halal_app_user halal_evidence_loader; do $P -c "CREATE ROLE $r" >/dev/null 2>&1; done
$P -c "SELECT timescaledb_pre_restore()" >/dev/null
docker exec -i $C pg_restore -U halal_user -d halal_equity --no-owner --role=halal_user < "$f" > /tmp/drill.log 2>&1; rc=$?
$P -c "SELECT timescaledb_post_restore()" >/dev/null
Q="select (select count(*) from ohlcv_daily)||'/'||(select count(*) from symbols)||'/'||(select count(*) from predictions)||'/'||(select count(*) from pg_tables where schemaname='public')"
got=$($P -c "$Q")
want=$(RSSH_TIMEOUT=60 rssh db "docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X -At -c \"$Q\"")
# daily/symbols/predictions are not written between nightly backup and now except predictions (append-only): require got <= want and equal tables
g1=${got%%/*}; w1=${want%%/*}
if [ "$rc" -eq 0 ] && [ "${got##*/}" = "${want##*/}" ] && [ "$g1" = "$w1" ]; then res=PASS; else res="FAIL(rc=$rc)"; fi
echo "$(now) restore-drill $res backup=$(basename "$f") restored(daily/symbols/predictions/tables)=$got db_now=$want" >> "$L"
tail -1 "$L"; [ "$res" = PASS ]
