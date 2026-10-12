package app

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
)

const loginItemLabel = "com.ryanzhangtianran.connaio"

func loginItemPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", loginItemLabel+".plist"), nil
}

// loginItemFile is the LaunchAgent that starts exe at login.
func loginItemFile(exe string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>%s</string>
	<key>ProgramArguments</key><array><string>%s</string></array>
	<key>RunAtLoad</key><true/>
</dict>
</plist>
`, loginItemLabel, html.EscapeString(exe))
}
