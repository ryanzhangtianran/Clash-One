#!/bin/sh
# render.sh renders the activity page from frontend/dist with demo data (mock.py) in headless Chrome,
# light and dark, into docs/preview_*.png for the README. Build the frontend first.
set -e
cd "$(dirname "$0")/../.."
port=8765
python3 -I build/preview/mock.py "$PWD" $port & server=$!
trap 'kill $server' EXIT
sleep 1
chrome="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
profile=$(mktemp -d)
for theme in light:1 dark:0; do
	"$chrome" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=2 --window-size=1440,890 \
		--virtual-time-budget=10000 --blink-settings=preferredColorScheme=${theme#*:} --user-data-dir="$profile" \
		--screenshot="docs/preview_${theme%:*}.png" http://127.0.0.1:$port/ >/dev/null 2>&1
done
rm -rf "$profile"
