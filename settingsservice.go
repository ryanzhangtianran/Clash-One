package main

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ryanzhangtianran/Clash-One/internal/daemon"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const appVersion = "0.1.0"

const loginItemLabel = "com.ryanzhangtianran.clashone"

// SettingsService covers the GUI-side settings: launch at login and the about page.
type SettingsService struct {
	core     *CoreService
	settings *settingsFile
}

type About struct {
	Version     string `json:"version"`
	CoreVersion string `json:"coreVersion"`
	DataDir     string `json:"dataDir"`
	Daemon      bool   `json:"daemon"`   // privileged service installed
	Attached    bool   `json:"attached"` // GUI is using it
}

func (s *SettingsService) About() About {
	return About{
		Version:     appVersion,
		CoreVersion: s.core.Status().Version,
		DataDir:     s.core.core.Dir,
		Daemon:      daemon.Installed(),
		Attached:    s.core.core.Daemon(),
	}
}

// OpenDataDir shows the data folder (config, logs, profiles) in Finder.
func (s *SettingsService) OpenDataDir() error {
	return exec.Command("open", s.core.core.Dir).Run()
}

func loginItemPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", loginItemLabel+".plist"), nil
}

func (s *SettingsService) LaunchAtLogin() bool {
	p, err := loginItemPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// TraySpeed reports whether the menu bar shows the current speed; it applies on the next second's update.
func (s *SettingsService) TraySpeed() bool { return !s.settings.Get().HideTraySpeed }

func (s *SettingsService) SetTraySpeed(on bool) error {
	return s.settings.Update(func(st *Settings) { st.HideTraySpeed = !on })
}

// SetLaunchAtLogin installs or removes a LaunchAgent that opens this app at login.
func (s *SettingsService) SetLaunchAtLogin(on bool) error {
	p, err := loginItemPath()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(loginItemPlist(exe)), 0o644)
}

func loginItemPlist(exe string) string {
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

// ProcessChoice is something the enhanced-mode scope can name: an app or a networked process.
type ProcessChoice struct {
	Name   string `json:"name"`   // shown to the user, e.g. "Lark"
	Exec   string `json:"exec"`   // executable name the PROCESS-ANCESTOR match uses, e.g. "Feishu"
	Source string `json:"source"` // "app": running app, "network": has live connections
}

// ProcessChoices lists running apps and processes with live connections, apps first.
func (s *SettingsService) ProcessChoices() []ProcessChoice {
	out, _ := exec.Command("ps", "-axo", "comm=").Output()
	choices := parseApps(string(out))
	seen := map[string]bool{}
	for _, c := range choices {
		seen[strings.ToLower(c.Exec)] = true
	}
	if conns, err := s.core.Connections(); err == nil {
		var network []ProcessChoice
		for _, c := range conns {
			if c.Process != "" && !seen[strings.ToLower(c.Process)] {
				seen[strings.ToLower(c.Process)] = true
				network = append(network, ProcessChoice{Name: c.Process, Exec: c.Process, Source: "network"})
			}
		}
		slices.SortFunc(network, func(a, b ProcessChoice) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
		choices = append(choices, network...)
	}
	return choices
}

// parseApps keeps the main executables of top-level app bundles from `ps -axo comm=` output,
// skipping helpers (apps nested inside another bundle).
func parseApps(ps string) []ProcessChoice {
	var apps []ProcessChoice
	seen := map[string]bool{}
	for _, line := range strings.Split(ps, "\n") {
		line = strings.TrimSpace(line)
		i := strings.Index(line, ".app/")
		if i < 0 {
			continue
		}
		bundle, rest := line[:i+len(".app")], line[i+len(".app/"):]
		exe, ok := strings.CutPrefix(rest, "Contents/MacOS/")
		if !ok || strings.Contains(exe, "/") || seen[bundle] {
			continue
		}
		seen[bundle] = true
		apps = append(apps, ProcessChoice{Name: strings.TrimSuffix(filepath.Base(bundle), ".app"), Exec: exe, Source: "app"})
	}
	slices.SortFunc(apps, func(a, b ProcessChoice) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return apps
}

// PickApp lets the user choose an app in Finder and returns its executable name.
func (s *SettingsService) PickApp() (ProcessChoice, error) {
	path, err := application.Get().Dialog.OpenFile().
		SetTitle("选择应用").
		SetDirectory("/Applications").
		CanChooseFiles(true).
		AddFilter("应用程序", "*.app").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return ProcessChoice{}, err
	}
	return appExecutable(path)
}

// appExecutable reads a bundle's CFBundleExecutable.
func appExecutable(bundle string) (ProcessChoice, error) {
	out, err := exec.Command("plutil", "-extract", "CFBundleExecutable", "raw", filepath.Join(bundle, "Contents", "Info.plist")).Output()
	if err != nil {
		return ProcessChoice{}, fmt.Errorf("%s 不是应用程序: %w", bundle, err)
	}
	return ProcessChoice{Name: strings.TrimSuffix(filepath.Base(bundle), ".app"), Exec: strings.TrimSpace(string(out)), Source: "app"}, nil
}
