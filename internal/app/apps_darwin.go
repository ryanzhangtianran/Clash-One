package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// runningApps lists the running apps' main executables, from the process table.
func runningApps() []ProcessChoice {
	out, _ := exec.Command("ps", "-axo", "comm=").Output()
	return parseApps(string(out))
}

// parseApps keeps the main executables of top-level app bundles from `ps -axo comm=` output.
func parseApps(ps string) []ProcessChoice {
	var apps []ProcessChoice
	seen := map[string]bool{}
	for _, line := range strings.Split(ps, "\n") {
		app, ok := appOfPath(strings.TrimSpace(line))
		if !ok || seen[strings.ToLower(app.Exec)] {
			continue
		}
		seen[strings.ToLower(app.Exec)] = true
		apps = append(apps, app)
	}
	slices.SortFunc(apps, func(a, b ProcessChoice) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return apps
}

// appOfPath is the app a top-level bundle's main executable belongs to; not for helpers (apps
// nested inside another bundle) or the system's own agents (Dock, ControlCenter, the framework
// agents), which nobody routes. /System/Applications (Mail, Messages, App Store) counts.
func appOfPath(path string) (ProcessChoice, bool) {
	bundle := bundleOf(path)
	if bundle == "" || systemDaemon(path) {
		return ProcessChoice{}, false
	}
	exe, ok := strings.CutPrefix(path[len(bundle)+1:], "Contents/MacOS/")
	if !ok || strings.Contains(exe, "/") {
		return ProcessChoice{}, false
	}
	return ProcessChoice{Name: strings.TrimSuffix(filepath.Base(bundle), ".app"), Exec: exe, Source: "app"}, true
}

// procTable reads the process table.
func procTable() map[int]proc {
	out, _ := exec.Command("ps", "-axo", "pid=,ppid=,comm=").Output()
	return darwinProcs(string(out))
}

// darwinProcs reads `ps -axo pid=,ppid=,comm=` output.
func darwinProcs(ps string) map[int]proc {
	procs := map[int]proc{}
	for _, line := range strings.Split(ps, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		p := proc{ppid: ppid, comm: strings.Join(f[2:], " ")} // ponytail: a run of spaces in a path collapses; slice the line if one ever shows up
		_, p.app = appOfPath(p.comm)
		procs[pid] = p
	}
	return procs
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

// appName names a Linux executable's application; here bundles (bundleOf) already fold processes into apps.
func appName(string) string { return "" }
