#!/bin/sh
# Api+jobs boxes: pull the newest backup from the DB box, check the checksum, check it is a readable TimescaleDB dump, keep 7.
. "$(dirname "$0")/lib.sh"
D=$HALAL_HOME/backups; mkdir -p "$D"; L=$LOGDIR/standby.log
latest=$(RSSH_TIMEOUT=40 rssh db "ls -1t $HALAL_HOME/backups/*.dump 2>/dev/null | head -1") || { echo "$(now) cannot reach db box" >> "$L"; exit 1; }
[ -n "$latest" ] || { echo "$(now) no backup on db box yet" >> "$L"; exit 0; }
name=$(basename "$latest"); [ -f "$D/$name" ] && exit 0
RSSH_TIMEOUT=900 rssh db "cat $latest" > "$D/$name.tmp" || { echo "$(now) transfer failed $name" >> "$L"; rm -f "$D/$name.tmp"; exit 1; }
want=$(RSSH_TIMEOUT=60 rssh db "sha256sum $latest" | cut -d' ' -f1); got=$(sha256sum "$D/$name.tmp" | cut -d' ' -f1)
[ "$want" = "$got" ] || { echo "$(now) CHECKSUM MISMATCH $name" >> "$L"; rm -f "$D/$name.tmp"; exit 1; }
n=$(docker run --rm -i timescale/timescaledb:2.30.1-pg16 pg_restore -l < "$D/$name.tmp" 2>/dev/null | grep -c 'TABLE DATA')
[ "$n" -ge 100 ] || { echo "$(now) UNREADABLE dump $name (tables=$n)" >> "$L"; rm -f "$D/$name.tmp"; exit 1; }
mv "$D/$name.tmp" "$D/$name"; echo "$(now) pulled+verified $name $(du -h "$D/$name" | cut -f1) tables=$n sha=$got" >> "$L"
ls -1t "$D"/*.dump | tail -n +8 | xargs -r rm -f
