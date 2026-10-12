package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// runningApps lists the running apps' main executables, from the process table.
func runningApps() []ProcessChoice {
	out, _ := exec.Command("ps", "-axo", "comm=").Output()
	return parseApps(string(out))
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
