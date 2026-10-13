package app

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// showTray sets the cat, pink and faded while nothing is captured, only when that changes, as each
// icon goes over D-Bus.
// The speed goes in the item's title. ponytail: no mode badge as on macOS; the menu shows the mode.
func showTray(tray *application.SystemTray, icon []byte, t Traffic, _ string, capturing, showSpeed bool) {
	setTrayIcon(tray, icon, capturing)
	if showSpeed {
		tray.SetLabel(fmt.Sprintf("↑%s ↓%s", rate(t.Up), rate(t.Down)))
	}
}

// decorate draws details and icons beside macOS menu items; Linux menus show plain items.
func decorate(_, _ map[*application.MenuItem]string) {}
