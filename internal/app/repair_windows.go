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
	tun := RepairItem{ID: "tun", Title: "残留的 TUN 适配器", Detail: "没有残留", Fix: "移除", Admin: true}
	if out, err := powershell("Get-NetAdapter -Name Meta -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Name"); err == nil && strings.TrimSpace(out) == "Meta" && len(orphanCores(-1)) == 0 {
		tun.Found, tun.Detail = true, "适配器 Meta 还在，但没有内核在用它；它的路由会把流量引向黑洞"
	}
	return []RepairItem{
		tun,
		{ID: "winsock", Title: "Winsock", Detail: "重置 Winsock 目录，解决被其他代理或安全软件改坏的网络栈", Fix: "重置", Admin: true, Reboot: true},
		{ID: "ipstack", Title: "TCP/IP", Detail: "重置 TCP/IP 协议栈到默认状态", Fix: "重置", Admin: true, Reboot: true},
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
