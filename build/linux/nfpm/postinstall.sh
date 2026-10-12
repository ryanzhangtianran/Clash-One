#!/bin/sh
# lists Connaio in the desktop's application menu right away
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q /usr/share/applications
exit 0
