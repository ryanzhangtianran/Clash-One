#!/bin/sh
# lists Connaio in the application menu right away, and moves running root cores to the new binary
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q /usr/share/applications
if [ -d /run/systemd/system ]; then # not in containers without systemd
	systemctl daemon-reload
	systemctl try-restart 'connaio-core@*.service' || true
fi
exit 0
