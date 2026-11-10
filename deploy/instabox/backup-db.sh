#!/bin/sh
# DB box: nightly logical backup next to the live data, verified before it replaces anything. Keeps the newest 7.
. "$(dirname "$0")/lib.sh"
D=$HALAL_HOME/backups; mkdir -p "$D"; f="$D/halal_equity_$(date -u +%Y%m%d_%H%M).dump"
docker exec halal_equity_postgres pg_dump -U halal_user -d halal_equity -Fc > "$f.tmp" 2>/dev/null || { echo "$(now) pg_dump FAILED" >> "$LOGDIR/backup.log"; rm -f "$f.tmp"; exit 1; }
n=$(docker exec -i halal_equity_postgres pg_restore -l < "$f.tmp" | grep -c 'TABLE DATA')
if [ "$n" -lt 100 ]; then echo "$(now) backup rejected: only $n table-data entries" >> "$LOGDIR/backup.log"; rm -f "$f.tmp"; exit 1; fi
mv "$f.tmp" "$f"; (cd "$D" && sha256sum "$(basename "$f")" > "$(basename "$f").sha256")
echo "$(now) backup ok $(basename "$f") $(du -h "$f" | cut -f1) tables=$n" >> "$LOGDIR/backup.log"
ls -1t "$D"/*.dump | tail -n +8 | while read -r old; do rm -f "$old" "$old.sha256"; done
