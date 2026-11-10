#!/bin/sh
# Install the systemd USER units for this box's role (lingering is enabled, so they start at boot and survive logout).
# Roles: api (frontend+backend), db (Postgres), jobs (worker+quant).
ROLE="${1:?role api|db|jobs}"; . "$(dirname "$0")/lib.sh"
U=$HOME/.config/systemd/user; mkdir -p "$U"; F=$HALAL_HOME/fleet
# remove units from the previous layout
systemctl --user disable --now halal-standby-pull.timer halal-watchdog.timer 2>/dev/null; rm -f "$U"/halal-standby-pull.* "$U"/halal-watchdog.*
[ "$ROLE" != db ]  && { systemctl --user disable --now halal-db-backup.timer 2>/dev/null; rm -f "$U"/halal-db-backup.*; }
[ "$ROLE" != jobs ] && { systemctl --user disable --now halal-bhavcopy.timer 2>/dev/null; rm -f "$U"/halal-bhavcopy.*; }
[ "$ROLE" != api ] && { systemctl --user disable --now halal-restore-drill.timer 2>/dev/null; rm -f "$U"/halal-restore-drill.*; }
unit() { cat > "$U/$1"; }
unit halal-fleet-health.service <<EOT
[Unit]
Description=Halal fleet health endpoint
[Service]
Environment=FLEET_SELF=$ROLE
ExecStart=/usr/bin/python3 $F/fleet-health.py
Restart=always
[Install]
WantedBy=default.target
EOT
timer() { # name, schedule lines, exec
  unit "halal-$1.service" <<EOT
[Unit]
Description=Halal $1
[Service]
Type=oneshot
ExecStart=$3
EOT
  unit "halal-$1.timer" <<EOT
[Unit]
Description=Halal $1 timer
[Timer]
$2
Persistent=true
[Install]
WantedBy=timers.target
EOT
}
stack() { unit halal-stack.service <<EOT
[Unit]
Description=Halal docker stack ($ROLE) ensure up at boot
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=$1
[Install]
WantedBy=default.target
EOT
}
timer fleet-ping "OnBootSec=2min
OnUnitActiveSec=10min" "$F/fleet-ping.sh"
ENABLE="halal-fleet-health.service halal-fleet-ping.timer"
case "$ROLE" in
  api)  stack "$HALAL_HOME/app/dc up -d"
        timer watchdog "OnBootSec=3min
OnUnitActiveSec=5min" "$F/fleet-watchdog.sh"
        timer backup-pull "OnCalendar=*-*-* 15:15:00" "$F/backup-pull.sh"
        timer check-open  "OnCalendar=Mon..Fri *-*-* 04:20:00" "$F/session-check.sh open"
        timer check-close "OnCalendar=Mon..Fri *-*-* 10:40:00" "$F/session-check.sh close"
        timer check-night "OnCalendar=Mon..Fri *-*-* 16:00:00" "$F/session-check.sh night"
        timer restore-drill "OnCalendar=Sun *-*-* 16:00:00" "$F/restore-drill.sh"
        ENABLE="$ENABLE halal-stack.service halal-watchdog.timer halal-backup-pull.timer halal-restore-drill.timer halal-check-open.timer halal-check-close.timer halal-check-night.timer";;
  db)   stack "docker compose -p halaldb --project-directory $HALAL_HOME/app -f $HALAL_HOME/app/deploy/instabox/compose/docker-compose.db.yml up -d"
        timer db-backup "OnCalendar=*-*-* 14:30:00" "$F/backup-db.sh"
        ENABLE="$ENABLE halal-stack.service halal-db-backup.timer";;
  jobs) stack "$HALAL_HOME/app/dc up -d"
        timer backup-pull "OnCalendar=*-*-* 15:20:00" "$F/backup-pull.sh"
        # NSE daily files are loaded at 20:00 IST (14:30 UTC), before the worker's 21:00 IST study + learning run
        timer bhavcopy "OnCalendar=*-*-* 14:30:00" "$F/bhavcopy-load.sh"
        ENABLE="$ENABLE halal-stack.service halal-backup-pull.timer halal-bhavcopy.timer";;
esac
systemctl --user daemon-reload && systemctl --user enable --now $ENABLE && systemctl --user list-timers --no-pager | grep halal
