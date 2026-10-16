package app

import "github.com/wailsapp/wails/v3/pkg/application"

// showTray sets the cat on its state's tile, only when that changes. The notification area shows
// no text, so the tooltip carries the mode and whether anything is captured; no speed, as on Linux.
func showTray(tray *application.SystemTray, icon []byte, _ Traffic, mode, state string, _ bool) {
	setTrayIcon(tray, icon, state)
	tip := "Connaio"
	if label := modeLabel(mode); label != "" {
		tip += " · " + label
	}
	if state == "" {
		tip += " · 未接管流量"
	}
	tray.SetTooltip(tip)
}
