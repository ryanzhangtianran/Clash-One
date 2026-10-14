package app

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// showTray sets the cat, pink and faded while nothing is captured, only when that changes. The
// notification area shows no text, so the tooltip carries the mode, whether anything is captured
// and the speed; the menu's first row shows the speed live.
func showTray(tray *application.SystemTray, icon []byte, t Traffic, mode string, capturing, showSpeed bool) {
	setTrayIcon(tray, icon, capturing)
	tip := "Connaio"
	if label := modeLabel(mode); label != "" {
		tip += " · " + label
	}
	if !capturing {
		tip += " · 未接管流量"
	}
	if showSpeed {
		tip += fmt.Sprintf("\n↑ %s\n↓ %s", rate(t.Up), rate(t.Down))
	}
	tray.SetTooltip(tip)
}
