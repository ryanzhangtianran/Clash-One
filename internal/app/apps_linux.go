package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// desktopApp is an installed application, from its .desktop entry.
type desktopApp struct {
	name, exec, icon string // exec: the executable's base name, as processes are matched
}

// desktopDirs hold .desktop entries: the system's, the user's, snaps' and flatpaks'.
func desktopDirs() []string {
	dirs := []string{"/usr/share/applications", "/usr/local/share/applications", "/var/lib/snapd/desktop/applications", "/var/lib/flatpak/exports/share/applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".local/share/applications"), filepath.Join(home, ".local/share/flatpak/exports/share/applications"))
	}
	return dirs
}

// desktopApps reads the installed applications once.
var desktopApps = sync.OnceValue(func() []desktopApp {
	var apps []desktopApp
	for _, dir := range desktopDirs() {
		paths, _ := filepath.Glob(filepath.Join(dir, "*.desktop"))
		for _, p := range paths {
			if b, err := os.ReadFile(p); err == nil {
				if a := parseDesktop(string(b)); a.exec != "" {
					apps = append(apps, a)
				}
			}
		}
	}
	return apps
})

// parseDesktop reads the [Desktop Entry] of a .desktop file; exec is empty for entries that aren't
// launchable applications (hidden, or with no command).
func parseDesktop(text string) desktopApp {
	var a desktopApp
	section := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || section != "[Desktop Entry]" {
			continue
		}
		switch k {
		case "Name":
			a.name = v
		case "Icon":
			a.icon = v
		case "Exec":
			a.exec = execName(v)
		case "NoDisplay", "Hidden":
			if v == "true" {
				return desktopApp{}
			}
		case "Type":
			if v != "Application" {
				return desktopApp{}
			}
		}
	}
	return a
}

// execName is the base name of the program an Exec line runs, past env and its assignments.
func execName(cmd string) string {
	for _, f := range strings.Fields(cmd) {
		f = strings.Trim(f, `"'`)
		if f == "env" || strings.Contains(f, "=") {
			continue
		}
		return filepath.Base(f)
	}
	return ""
}

// runningApps lists the installed applications that are running, by the executables in /proc.
func runningApps() []ProcessChoice {
	running := map[string]bool{}
	exes, _ := filepath.Glob("/proc/[0-9]*/exe")
	for _, p := range exes {
		if target, err := os.Readlink(p); err == nil { // other users' processes can't be read: skipped
			running[filepath.Base(strings.TrimSuffix(target, " (deleted)"))] = true
		}
	}
	var apps []ProcessChoice
	seen := map[string]bool{}
	for _, a := range desktopApps() {
		if running[a.exec] && !seen[a.exec] {
			seen[a.exec] = true
			apps = append(apps, ProcessChoice{Name: a.name, Exec: a.exec, Source: "app"})
		}
	}
	slices.SortFunc(apps, func(a, b ProcessChoice) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return apps
}

// PickApp lets the user choose an application's .desktop entry and returns its executable name.
func (s *SettingsService) PickApp() (ProcessChoice, error) {
	path, err := application.Get().Dialog.OpenFile().
		SetTitle("选择应用").
		SetDirectory("/usr/share/applications").
		CanChooseFiles(true).
		AddFilter("应用程序", "*.desktop").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return ProcessChoice{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ProcessChoice{}, err
	}
	a := parseDesktop(string(b))
	if a.exec == "" {
		return ProcessChoice{}, os.ErrInvalid
	}
	return ProcessChoice{Name: a.name, Exec: a.exec, Source: "app"}, nil
}

var appIcons sync.Map // executable path -> data URL ("" when none)

// AppIcon is the icon of the application whose executable is path, from its .desktop entry and
// the hicolor icon theme, as a data URL; "" when it has none, and the window shows its glyph.
func (s *SettingsService) AppIcon(path string) string {
	if v, ok := appIcons.Load(path); ok {
		return v.(string)
	}
	url := ""
	exe := filepath.Base(path)
	for _, a := range desktopApps() {
		if a.exec == exe && a.icon != "" {
			url = iconURL(a.icon)
			break
		}
	}
	appIcons.Store(path, url)
	return url
}

// iconURL finds a theme icon (or takes an icon path) and reads it as a data URL.
func iconURL(icon string) string {
	candidates := []string{icon}
	if !filepath.IsAbs(icon) {
		candidates = nil
		roots := []string{"/usr/share/icons/hicolor"}
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, filepath.Join(home, ".local/share/icons/hicolor"))
		}
		for _, root := range roots {
			for _, size := range []string{"scalable", "256x256", "128x128", "96x96", "64x64", "48x48"} {
				for _, ext := range []string{".svg", ".png"} {
					candidates = append(candidates, filepath.Join(root, size, "apps", icon+ext))
				}
			}
		}
		candidates = append(candidates, "/usr/share/pixmaps/"+icon+".png", "/usr/share/pixmaps/"+icon+".svg")
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		mime := "image/png"
		if strings.HasSuffix(p, ".svg") {
			mime = "image/svg+xml"
		}
		return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
	}
	return ""
}

// procTable reads the process table from /proc.
func procTable() map[int]proc {
	apps := map[string]bool{}
	for _, a := range desktopApps() {
		apps[a.exec] = true
	}
	procs := map[int]proc{}
	dirs, _ := filepath.Glob("/proc/[0-9]*")
	for _, d := range dirs {
		stat, err := os.ReadFile(d + "/stat")
		if err != nil {
			continue
		}
		exe, _ := os.Readlink(d + "/exe") // another user's: unreadable, the stat comm (15 chars) stands in
		pid, _ := strconv.Atoi(filepath.Base(d))
		p := linuxProc(string(stat), strings.TrimSuffix(exe, " (deleted)"))
		p.app = apps[filepath.Base(p.comm)]
		procs[pid] = p
	}
	return procs
}

// linuxProc reads a /proc/<pid>/stat line, "pid (comm) S ppid …", comm holding any character.
func linuxProc(stat, exe string) proc {
	i := strings.LastIndex(stat, ")")
	if i < 0 {
		return proc{}
	}
	f := strings.Fields(stat[i+1:])
	p := proc{comm: exe}
	if len(f) > 1 {
		p.ppid, _ = strconv.Atoi(f[1])
	}
	if p.comm == "" {
		p.comm = stat[strings.Index(stat, "(")+1 : i]
	}
	return p
}
