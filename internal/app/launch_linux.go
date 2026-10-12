package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// loginItemPath is the XDG autostart entry GNOME (and other desktops) start at login.
func loginItemPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "connaio.desktop"), nil
}

// loginItemFile is the autostart entry that starts exe at login.
func loginItemFile(exe string) string {
	return fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Connaio\nExec=%q\nIcon=connaio\nX-GNOME-Autostart-enabled=true\n", exe)
}
