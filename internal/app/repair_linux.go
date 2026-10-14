package app

import (
	"os"
	"strconv"
	"strings"

	"github.com/ryanzhangtianran/Connaio/internal/run"
)

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
	if out, err := run.Command("resolvectl", "flush-caches").CombinedOutput(); err != nil {
		return "", errorsText("resolvectl flush-caches", out, err)
	}
	return "DNS 缓存已刷新", nil
}

// platformChecks looks for the Meta TUN interface a dead core left behind.
func platformChecks() []RepairItem {
	item := RepairItem{ID: "tun", Title: "残留 TUN", Detail: "无", Fix: "移除"}
	if _, err := os.Stat("/sys/class/net/Meta"); err == nil && len(orphanCores(-1)) == 0 {
		item.Found, item.Detail = true, "接口 Meta 无人使用"
	}
	return []RepairItem{item}
}

func platformRepair(id string) (string, error) {
	if id == "tun" {
		return runAdmin("ip link delete Meta", "Connaio 要移除残留的 TUN 接口。", "TUN 接口已移除")
	}
	return "", errUnknownRepair
}
