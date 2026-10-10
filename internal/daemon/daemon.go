// Package daemon installs clash-one-core as a root LaunchDaemon, which enhanced mode (TUN) needs.
//
// Everything root touches is root-owned: the binary is copied out of the (user-writable) app
// bundle, and the daemon's data dir and socket live under /Library. The GUI sends configs as
// payloads instead of paths, so the root process never follows paths inside the user's home.
package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	Label  = "com.ryanzhangtianran.clashone.core"
	Helper = "/Library/PrivilegedHelperTools/" + Label
	Plist  = "/Library/LaunchDaemons/" + Label + ".plist"
	Dir    = "/Library/Application Support/Clash One Core"
	Socket = Dir + "/core.sock"
)

// BootConfig is what the daemon runs while no GUI is attached: no TUN, nothing captured.
const BootConfig = "mixed-port: 7899\nmode: rule\nrules:\n  - MATCH,DIRECT\n"

// Installed reports whether the LaunchDaemon is set up.
func Installed() bool {
	_, err := os.Stat(Plist)
	return err == nil
}

func plist(uid int) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key><string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + Helper + `</string>
		<string>-d</string><string>` + Dir + `</string>
		<string>-ext-ctl-unix</string><string>` + Socket + `</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict><key>CLASH_ONE_SOCKET_UID</key><string>` + strconv.Itoa(uid) + `</string></dict>
	<key>RunAtLoad</key><true/>
	<key>KeepAlive</key><true/>
	<key>StandardOutPath</key><string>` + Dir + `/core.log</string>
	<key>StandardErrorPath</key><string>` + Dir + `/core.log</string>
</dict>
</plist>
`
}

// installScript runs as root. Its inputs come from user-writable places, so after copying, the
// root-owned copies are checked against hashes taken before the prompt: a file swapped in between
// is removed instead of run.
func installScript(core, coreSum, stage, plistSum string) string {
	q := shellQuote
	verify := func(sum, path string) string {
		return "echo " + q(sum+"  "+path) + " | shasum -a 256 -c -s || { rm -f " + q(path) + "; echo 'checksum mismatch: " + path + "' >&2; exit 1; }"
	}
	return strings.Join([]string{
		"set -e",
		"launchctl bootout system/" + Label + " 2>/dev/null || true",
		"mkdir -p /Library/PrivilegedHelperTools " + q(Dir),
		"chmod 755 " + q(Dir),
		"install -o root -g wheel -m 755 " + q(core) + " " + q(Helper),
		verify(coreSum, Helper),
		"install -o root -g wheel -m 644 " + q(filepath.Join(stage, "daemon.plist")) + " " + q(Plist),
		verify(plistSum, Plist),
		"[ -f " + q(Dir+"/config.yaml") + " ] || install -o root -g wheel -m 644 " + q(filepath.Join(stage, "config.yaml")) + " " + q(Dir+"/config.yaml"),
		"launchctl bootstrap system " + q(Plist),
	}, "\n")
}

func uninstallScript() string {
	return strings.Join([]string{
		"launchctl bootout system/" + Label + " 2>/dev/null || true",
		"rm -f " + shellQuote(Plist) + " " + shellQuote(Helper),
	}, "\n")
}

// Install copies core (the app's bundled binary) to a root-owned location and starts the
// LaunchDaemon for the current user, after one administrator prompt.
func Install(core string) error {
	stage, err := os.MkdirTemp("", "clash-one-daemon-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	p := []byte(plist(os.Getuid()))
	if err := os.WriteFile(filepath.Join(stage, "daemon.plist"), p, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "config.yaml"), []byte(BootConfig), 0o644); err != nil {
		return err
	}
	coreSum, err := fileSum(core)
	if err != nil {
		return err
	}
	return asAdmin(installScript(core, coreSum, stage, sum(p)), "Clash One 需要安装特权服务以开启增强模式（TUN）。")
}

// Uninstall stops and removes the LaunchDaemon; its data dir is kept.
func Uninstall() error {
	return asAdmin(uninstallScript(), "Clash One 需要移除特权服务。")
}

// asAdmin runs a shell script as root through the standard macOS administrator prompt.
func asAdmin(script, prompt string) error {
	apple := fmt.Sprintf("do shell script %s with prompt %s with administrator privileges", appleQuote(script), appleQuote(prompt))
	out, err := exec.Command("osascript", "-e", apple).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "-128") { // the user pressed Cancel
			return fmt.Errorf("已取消授权")
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func fileSum(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sum(b), nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func appleQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
