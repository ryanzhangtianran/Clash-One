package app

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// proc is a row of the process table: parent, executable (Windows: image name), and whether it is
// a user-facing app as runningApps lists them, for finding the app a process was started from.
type proc struct {
	ppid int
	comm string
	app  bool
}

// procs is the process table, read at most every 5s: the tray asks per connection every second.
var procs struct {
	sync.Mutex
	at      time.Time
	table   map[int]proc
	parents map[string]string
}

// parentOf is the executable of the nearest app up a process's parent chain (Ghostty for a claude
// started in it), which is what capturing that app covers; "" for one under no app.
func parentOf(name string) string {
	procs.Lock()
	defer procs.Unlock()
	if time.Since(procs.at) > 5*time.Second {
		procs.table, procs.parents, procs.at = procTable(), map[string]string{}, time.Now()
	}
	if _, ok := procs.parents[name]; !ok {
		parent, found := parentIn(procs.table, name)
		if !found { // started since the table was read: read it again (the result is cached, so once per name per window)
			procs.table, procs.at = procTable(), time.Now()
			parent, _ = parentIn(procs.table, name)
		}
		procs.parents[name] = parent
	}
	return procs.parents[name]
}

// parentIn is parentOf over a table; found says whether the process is in it at all.
func parentIn(table map[int]proc, name string) (parent string, found bool) {
	for _, p := range table {
		if !strings.EqualFold(filepath.Base(p.comm), name) {
			continue
		}
		found = true
		for cur, depth := p.ppid, 0; cur > 1 && depth < 32; cur, depth = table[cur].ppid, depth+1 {
			if a := table[cur]; a.app {
				return a.comm, true
			}
		}
	}
	return "", found
}

// systemDaemon says whether an executable is one of macOS's own daemons and agents (apsd, trustd,
// nsurlsessiond, Dock), which nobody routes on their own.
func systemDaemon(path string) bool {
	return strings.HasPrefix(path, "/System/Library/") || strings.HasPrefix(path, "/Library/Apple/") || strings.HasPrefix(path, "/usr/libexec/")
}

// bundleOf is the .app bundle a path is inside, or "".
func bundleOf(path string) string {
	if i := strings.Index(path, ".app/"); i >= 0 {
		return path[:i+len(".app")]
	}
	return ""
}
