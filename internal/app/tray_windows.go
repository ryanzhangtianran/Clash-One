package app

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// showTray sets the cat, pink and faded while nothing is captured, only when that changes. The
// notification area shows no text, so the speed goes in the tooltip.
func showTray(tray *application.SystemTray, icon []byte, t Traffic, _ string, capturing, showSpeed bool) {
	setTrayIcon(tray, icon, capturing)
	tip := "Connaio"
	if showSpeed {
		tip = fmt.Sprintf("Connaio  ↑%s ↓%s", rate(t.Up), rate(t.Down))
	}
	tray.SetTooltip(tip)
}

// decorate draws details and icons beside macOS menu items; Windows menus show plain items.
func decorate(_, _ map[*application.MenuItem]string) {}
