package app

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// showTray puts the speed in the tray item's title. ponytail: a Linux tray (StatusNotifierItem) has
// no badge or greyed state like the macOS item; draw them into the icon if they're missed.
func showTray(tray *application.SystemTray, _ []byte, t Traffic, _ string, _, showSpeed bool) {
	if showSpeed {
		tray.SetLabel(fmt.Sprintf("↑%s ↓%s", rate(t.Up), rate(t.Down)))
	}
}

// decorate draws details and icons beside macOS menu items; Linux menus show plain items.
func decorate(_, _ map[*application.MenuItem]string) {}
