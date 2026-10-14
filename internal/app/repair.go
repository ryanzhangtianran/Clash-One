package app

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/ryanzhangtianran/Connaio/internal/daemon"
	"github.com/ryanzhangtianran/Connaio/internal/sysproxy"
)

// RepairItem is one network check with its fix: what a proxy app leaves behind when it dies
// (a system proxy pointing at nothing, a stray core, a TUN adapter), plus the usual resets.
type RepairItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`    // the button; "" for a check with nothing to run
	Found  bool   `json:"found"`  // a problem was found, as opposed to an optional reset
	Admin  bool   `json:"admin"`  // the fix needs the administrator prompt
	Reboot bool   `json:"reboot"` // the fix takes effect after a restart
}

// RepairChecks runs every check and reports what it found, problems first.
func (s *ToolService) RepairChecks() []RepairItem {
	items := []RepairItem{s.checkProxy(), s.checkOrphans()}
	items = append(items, platformChecks()...)
	items = append(items, RepairItem{ID: "dns", Title: "DNS 缓存", Detail: "刷新系统的 DNS 缓存，解决改过代理或 DNS 后域名解析仍旧的问题", Fix: "刷新", Admin: runtime.GOOS == "darwin"})
	slices.SortStableFunc(items, func(a, b RepairItem) int {
		if a.Found != b.Found {
			if a.Found {
				return -1
			}
			return 1
		}
		return 0
	})
	return items
}

// Repair runs one item's fix and says what it did.
func (s *ToolService) Repair(id string) (string, error) {
	switch id {
	case "proxy":
		if err := sysproxy.Clear(); err != nil {
			return "", err
		}
		return "系统代理已清除", nil
	case "orphan":
		var killed, failed []string
		for _, pid := range orphanCores(s.ownCorePid()) {
			p, err := os.FindProcess(pid)
			if err == nil {
				err = p.Kill()
			}
			if err != nil {
				failed = append(failed, strconv.Itoa(pid))
			} else {
				killed = append(killed, strconv.Itoa(pid))
			}
		}
		if len(failed) > 0 {
			return "", fmt.Errorf("结束了 %d 个，另外 %s 没能结束", len(killed), strings.Join(failed, ", "))
		}
		return fmt.Sprintf("结束了 %d 个残留内核", len(killed)), nil
	case "dns":
		return flushDNS()
	}
	return platformRepair(id)
}

// checkProxy finds a system proxy that leads nowhere: not ours, or ours while the proxy is off.
func (s *ToolService) checkProxy() RepairItem {
	item := RepairItem{ID: "proxy", Title: "系统代理", Fix: "清除", Detail: "系统代理未设置"}
	addr, on := sysproxy.Current()
	if !on {
		return item
	}
	ours := ""
	if port, err := mixedPort(s.cs.core); err == nil {
		ours = "127.0.0.1:" + strconv.Itoa(port)
	}
	switch {
	case addr == ours && s.cs.Status().Running:
		item.Detail = "系统代理指向 Connaio（" + addr + "），正常"
	case addr == ours:
		item.Found, item.Detail = true, "系统代理指向 Connaio（"+addr+"），但内核没有运行"
	default:
		item.Found, item.Detail = true, "系统代理指向 "+addr+"，不是 Connaio 设置的；若那个程序已退出，网络会不通"
	}
	return item
}

// checkOrphans finds this user's connaio-core processes other than this app's child.
func (s *ToolService) checkOrphans() RepairItem {
	item := RepairItem{ID: "orphan", Title: "残留的内核进程", Fix: "结束", Detail: "没有残留"}
	if pids := orphanCores(s.ownCorePid()); len(pids) > 0 {
		var list []string
		for _, p := range pids {
			list = append(list, strconv.Itoa(p))
		}
		item.Found, item.Detail = true, fmt.Sprintf("%d 个不属于本应用的 connaio-core 在运行（PID %s），可能占着端口或 TUN", len(pids), strings.Join(list, ", "))
	}
	return item
}

func (s *ToolService) ownCorePid() int { return s.cs.core.Pid() }

// runAdmin runs a repair script after the administrator prompt and reports it done.
func runAdmin(script, prompt, done string) (string, error) {
	if err := daemon.RunAsAdmin(script, prompt); err != nil {
		return "", err
	}
	return done, nil
}

var errUnknownRepair = errors.New("没有这个修复项")

// errorsText is a failed command's output as the error, falling back to err.
func errorsText(cmd string, out []byte, err error) error {
	if text := strings.TrimSpace(string(out)); text != "" {
		return fmt.Errorf("%s：%s", cmd, text)
	}
	return fmt.Errorf("%s：%w", cmd, err)
}
