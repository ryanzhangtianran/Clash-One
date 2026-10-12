package app

import (
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// routeOf is the default route ("-inet" or "-inet6") as key -> value, interface and gateway, as
// `route -n get` gives them on macOS; from /proc/net/route and /proc/net/ipv6_route.
func routeOf(family string) map[string]string {
	if family == "-inet6" {
		b, _ := os.ReadFile("/proc/net/ipv6_route")
		return parseRoute6(string(b))
	}
	b, _ := os.ReadFile("/proc/net/route")
	return parseRoute(string(b))
}

// parseRoute picks the lowest-metric default route of /proc/net/route, whose addresses are
// little-endian hex.
func parseRoute(table string) map[string]string {
	best, metric := map[string]string{}, -1
	for _, line := range strings.Split(table, "\n")[1:] {
		f := strings.Fields(line)
		if len(f) < 7 || f[1] != "00000000" {
			continue
		}
		m, _ := strconv.Atoi(f[6])
		if metric >= 0 && m >= metric {
			continue
		}
		gw, err := hex.DecodeString(f[2])
		if err != nil || len(gw) != 4 {
			continue
		}
		best, metric = map[string]string{"interface": f[0], "gateway": net.IPv4(gw[3], gw[2], gw[1], gw[0]).String()}, m
	}
	return best
}

// parseRoute6 picks the default route (::/0) of /proc/net/ipv6_route, skipping loopback.
func parseRoute6(table string) map[string]string {
	for _, line := range strings.Split(table, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || f[0] != strings.Repeat("0", 32) || f[1] != "00" || f[9] == "lo" {
			continue
		}
		m := map[string]string{"interface": f[9]}
		if gw, err := hex.DecodeString(f[4]); err == nil && len(gw) == 16 && !net.IP(gw).IsUnspecified() {
			m["gateway"] = net.IP(gw).String()
		}
		return m
	}
	return map[string]string{}
}

// dnsServers are the system resolver's upstreams: systemd-resolved's, since Ubuntu's
// /etc/resolv.conf names only its 127.0.0.53 stub; that file when resolvectl isn't there.
func dnsServers() []string {
	if out, err := exec.Command("resolvectl", "dns").Output(); err == nil {
		return parseResolvectl(string(out))
	}
	b, _ := os.ReadFile("/etc/resolv.conf")
	var servers []string
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "nameserver" && f[1] != "127.0.0.53" && !slices.Contains(servers, f[1]) {
			servers = append(servers, f[1])
		}
	}
	return servers
}

// parseResolvectl collects the addresses `resolvectl dns` lists per link ("Link 2 (eth0): 1.1.1.1").
func parseResolvectl(out string) []string {
	var servers []string
	for _, line := range strings.Split(out, "\n") {
		_, list, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		for _, s := range strings.Fields(list) {
			s, _, _ = strings.Cut(s, "#") // an optional "#server-name" for DNS over TLS
			if net.ParseIP(strings.Split(s, "%")[0]) != nil && !slices.Contains(servers, s) {
				servers = append(servers, s)
			}
		}
	}
	return servers
}

// pingArgs time one ICMP echo, with a 2s timeout (-W on Linux, where -t is the TTL).
var pingArgs = []string{"-n", "-c", "1", "-W", "2"}

// arpTable maps IPv4 addresses to MAC addresses from /proc/net/arp (Ubuntu has no `arp` by default).
func arpTable() map[string]string {
	b, _ := os.ReadFile("/proc/net/arp")
	return parseProcARP(string(b))
}

func parseProcARP(table string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(table, "\n")[1:] {
		f := strings.Fields(line)
		if len(f) >= 4 && f[2] != "0x0" && f[3] != "00:00:00:00:00:00" { // flags 0: incomplete
			m[f[0]] = f[3]
		}
	}
	return m
}

// interfaceLabel is the NetworkManager connection on iface, as Ubuntu's settings name it (e.g.
// 有线连接 1); else Wi-Fi or 有线连接 by the kind of interface.
func interfaceLabel(iface string) string {
	out, _ := exec.Command("nmcli", "-t", "-f", "NAME,DEVICE", "connection", "show", "--active").Output()
	if name := nmConnection(string(out), iface); name != "" {
		return name
	}
	if _, err := os.Stat("/sys/class/net/" + iface + "/wireless"); err == nil {
		return "Wi-Fi"
	}
	if _, err := os.Stat("/sys/class/net/" + iface + "/device"); err == nil { // a physical interface
		return "有线连接"
	}
	return ""
}

// nmConnection picks iface's connection from `nmcli -t -f NAME,DEVICE` lines, whose fields escape
// their colons with a backslash.
func nmConnection(out, iface string) string {
	for _, line := range strings.Split(out, "\n") {
		var fields []string
		var cur strings.Builder
		for i := 0; i < len(line); i++ {
			switch {
			case line[i] == '\\' && i+1 < len(line):
				i++
				cur.WriteByte(line[i])
			case line[i] == ':':
				fields = append(fields, cur.String())
				cur.Reset()
			default:
				cur.WriteByte(line[i])
			}
		}
		if fields = append(fields, cur.String()); len(fields) == 2 && fields[1] == iface {
			return fields[0]
		}
	}
	return ""
}
