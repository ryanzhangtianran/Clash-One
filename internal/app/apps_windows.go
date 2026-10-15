package app

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// runningApps lists the processes that have a window, as the user-facing apps. Exec is the image
// name with its extension, which is how the core's PROCESS-ANCESTOR sees Windows processes.
func runningApps() []ProcessChoice {
	out, _ := powershell("Get-Process | Where-Object MainWindowTitle | Select-Object -ExpandProperty ProcessName -Unique")
	return parseApps(out)
}

func parseApps(out string) []ProcessChoice {
	var apps []ProcessChoice
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			apps = append(apps, ProcessChoice{Name: name, Exec: name + ".exe", Source: "app"})
		}
	}
	slices.SortFunc(apps, func(a, b ProcessChoice) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return apps
}

// PickApp lets the user choose a program and returns its image name.
func (s *SettingsService) PickApp() (ProcessChoice, error) {
	path, err := application.Get().Dialog.OpenFile().
		SetTitle("选择应用").
		CanChooseFiles(true).
		AddFilter("应用程序", "*.exe").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return ProcessChoice{}, err
	}
	exe := filepath.Base(path)
	return ProcessChoice{Name: strings.TrimSuffix(exe, filepath.Ext(exe)), Exec: exe, Source: "app"}, nil
}

// AppIcon is empty on Windows: the window shows the client's glyph instead.
// ponytail: no icon extraction; add ExtractIconEx through x/sys/windows if the list looks bare.
func (s *SettingsService) AppIcon(string) string { return "" }

// procTable reads the process table: the windowed processes are the apps, as runningApps lists
// them; one script, so the table is read once.
func procTable() map[int]proc {
	out, _ := powershell(`$w = @{}; Get-Process | Where-Object MainWindowTitle | ForEach-Object { $w[($_.ProcessName + '.exe').ToLower()] = 1 }; ` +
		`Get-CimInstance Win32_Process | ForEach-Object { "$($_.ProcessId) $($_.ParentProcessId) $(if ($w[$_.Name.ToLower()]) {1} else {0}) $($_.Name)" }`)
	return windowsProcs(out)
}

// windowsProcs reads "pid ppid isapp name" lines.
func windowsProcs(out string) map[int]proc {
	procs := map[int]proc{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		procs[pid] = proc{ppid: ppid, comm: strings.Join(f[3:], " "), app: f[2] == "1"}
	}
	return procs
}
