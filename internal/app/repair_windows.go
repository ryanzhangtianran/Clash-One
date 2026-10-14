package app

import (
	"encoding/csv"
	"os"
	"strconv"
	"strings"

	"github.com/ryanzhangtianran/Connaio/internal/run"
)

// orphanCores lists connaio-core.exe processes other than ours, from tasklist's CSV.
func orphanCores(own int) []int {
	out, _ := run.Command("tasklist", "/FI", "IMAGENAME eq connaio-core.exe", "/FI", "USERNAME eq "+os.Getenv("USERNAME"), "/FO", "CSV", "/NH").Output() // this user's only: the service is not an orphan
	rows, _ := csv.NewReader(strings.NewReader(string(out))).ReadAll()
	var pids []int
	for _, r := range rows {
		if len(r) > 1 {
			if pid, err := strconv.Atoi(r[1]); err == nil && pid != own {
				pids = append(pids, pid)
			}
		}
	}
	return pids
}

func flushDNS() (string, error) {
	if out, err := run.Command("ipconfig", "/flushdns").CombinedOutput(); err != nil {
		return "", errorsText("ipconfig /flushdns", out, err)
	}
	return "DNS 缓存已刷新", nil
}

// platformChecks: the wintun adapter a dead core left, and the resets Windows users reach for.
func platformChecks() []RepairItem {
	tun := RepairItem{ID: "tun", Title: "残留 TUN", Detail: "无", Fix: "移除"}
	if out, err := powershell("Get-NetAdapter -Name Meta -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Name"); err == nil && strings.TrimSpace(out) == "Meta" && len(orphanCores(-1)) == 0 {
		tun.Found, tun.Detail = true, "适配器 Meta 无人使用"
	}
	return []RepairItem{
		tun,
		{ID: "winsock", Title: "Winsock", Detail: "重置 Winsock", Fix: "重置", Reboot: true},
		{ID: "ipstack", Title: "TCP/IP", Detail: "重置 TCP/IP", Fix: "重置", Reboot: true},
	}
}

func platformRepair(id string) (string, error) {
	switch id {
	case "tun":
		return runAdmin("netsh interface set interface \"Meta\" admin=disable\r\npnputil /remove-device /deviceid \"Meta\" >nul 2>&1\r\nexit /b 0\r\n", "", "TUN 适配器已移除")
	case "winsock":
		return runAdmin("netsh winsock reset\r\n", "", "Winsock 已重置，重启后生效")
	case "ipstack":
		return runAdmin("netsh int ip reset\r\nnetsh int ipv6 reset\r\n", "", "TCP/IP 已重置，重启后生效")
	}
	return "", errUnknownRepair
}
