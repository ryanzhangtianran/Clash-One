#!/bin/sh
# on removal (not upgrade) stops and disables every user's root core, whose binary is going away
[ "${1:-}" = remove ] || exit 0
if [ -d /run/systemd/system ]; then
	for unit in $(systemctl list-unit-files --no-legend 'connaio-core@*.service' 2>/dev/null | awk '{print $1}') \
		$(cd /etc/systemd/system/multi-user.target.wants 2>/dev/null && ls connaio-core@*.service 2>/dev/null); do
		case $unit in connaio-core@.service) ;; *) systemctl disable --now "$unit" || true ;; esac
	done
fi
exit 0
