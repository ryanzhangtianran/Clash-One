package app

import (
	"os"
	"strconv"
	"strings"

	"github.com/ryanzhangtianran/Connaio/internal/run"
)

// orphanCores lists connaio-core processes other than ours; the root daemon's is listed too, and
// Kill then fails on it with a pointer to 通用.
func orphanCores(own int) []int {
	out, _ := run.Command("pgrep", "-x", "-u", strconv.Itoa(os.Getuid()), "connaio-core").Output() // this user's only: the root daemon is not an orphan
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil && pid != own {
			pids = append(pids, pid)
		}
	}
	return pids
}

func flushDNS() (string, error) {
	return runAdmin("dscacheutil -flushcache; killall -HUP mDNSResponder", "Connaio 要刷新 DNS 缓存。", "DNS 缓存已刷新")
}

// platformChecks: a TUN left by a dead core vanishes with it on macOS, routes included, so there
// is nothing beyond the common checks.
func platformChecks() []RepairItem { return nil }

func platformRepair(string) (string, error) { return "", errUnknownRepair }
