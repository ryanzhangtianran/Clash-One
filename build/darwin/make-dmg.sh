#!/bin/sh
# make-dmg.sh APP OUT.dmg: a drag-to-install disk image, the app on the left and Applications on
# the right over dmg-background.tiff (640×400 pt, with @2x). Finder lays the window out, so it
# runs on a Mac with a logged-in session (macOS may ask to let the terminal control Finder).
set -eu
app=$1 out=$2 name=Connaio
here=$(cd "$(dirname "$0")" && pwd)
stage=$(mktemp -d) rw=$(mktemp -u).dmg
trap 'rm -rf "$stage" "$rw"' EXIT

cp -R "$app" "$stage/$name.app"
ln -s /Applications "$stage/Applications"
mkdir "$stage/.background"
cp "$here/dmg-background.tiff" "$stage/.background/background.tiff"

hdiutil create -quiet -volname "$name" -srcfolder "$stage" -fs HFS+ -format UDRW -ov "$rw"
dev=$(hdiutil attach -readwrite -noverify -noautoopen "$rw" | awk '/Apple_HFS/ {print $1}')
osascript <<APPLESCRIPT
tell application "Finder"
	tell disk "$name"
		open
		set current view of container window to icon view
		set toolbar visible of container window to false
		set statusbar visible of container window to false
		set bounds of container window to {200, 120, 840, 548}
		set opts to icon view options of container window
		set arrangement of opts to not arranged
		set icon size of opts to 112
		set text size of opts to 13
		set background picture of opts to file ".background:background.tiff"
		set position of item "$name.app" of container window to {160, 200}
		set position of item "Applications" of container window to {480, 200}
		update without registering applications
		delay 1
		close
	end tell
end tell
APPLESCRIPT
sync
hdiutil detach -quiet "$dev"
hdiutil convert -quiet "$rw" -format UDZO -imagekey zlib-level=9 -ov -o "$out"
