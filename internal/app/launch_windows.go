package app

import (
	"os"
	"path/filepath"
)

// loginItemPath is a script in the user's Startup folder, which Explorer runs at sign-in.
func loginItemPath() (string, error) {
	dir, err := os.UserConfigDir() // %AppData%
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "Connaio.vbs"), nil
}

// loginItemFile starts exe without flashing a console window.
func loginItemFile(exe string) string {
	return `CreateObject("WScript.Shell").Run """` + exe + `""", 0, False` + "\r\n"
}
