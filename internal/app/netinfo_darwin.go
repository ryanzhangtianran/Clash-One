package app

import (
	"os/exec"
	"strings"
)

// routeOf is `route -n get <family> default` as key -> value.
func routeOf(family string) map[string]string {
	out, _ := exec.Command("route", "-n", "get", family, "default").Output()
	m := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), ":"); ok {
			m[k] = strings.TrimSpace(v)
		}
	}
	return m
}

// dnsServers are the system resolver's nameservers, from `scutil --dns`.
func dnsServers() []string {
	out, _ := exec.Command("scutil", "--dns").Output()
	var servers []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), " : "); ok && strings.HasPrefix(k, "nameserver[") && !seen[v] {
			seen[v] = true
			servers = append(servers, v)
		}
	}
	return servers
}

// pingArgs time one ICMP echo, with a 2s timeout (-t on macOS).
var pingArgs = []string{"-n", "-c", "1", "-t", "2"}

// arpTable maps IPv4 addresses to MAC addresses from `arp -an`.
func arpTable() map[string]string {
	out, _ := exec.Command("arp", "-an").Output()
	return parseARP(string(out))
}

// parseARP reads lines like "? (192.168.1.23) at a4:83:e7:1:2:3 on en0 ifscope [ethernet]".
func parseARP(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		_, rest, ok := strings.Cut(line, "(")
		ip, rest, ok2 := strings.Cut(rest, ") at ")
		mac, _, _ := strings.Cut(rest, " ")
		if ok && ok2 && mac != "(incomplete)" {
			m[ip] = mac
		}
	}
	return m
}

// interfaceLabel is iface's hardware port as System Settings names it, e.g. Wi-Fi or USB 10/100/1000 LAN.
func interfaceLabel(iface string) string {
	out, _ := exec.Command("networksetup", "-listallhardwareports").Output()
	return hardwarePort(string(out), iface)
}

func hardwarePort(out, iface string) string {
	port := ""
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(line, "Hardware Port: "); ok {
			port = v
		} else if v, ok := strings.CutPrefix(line, "Device: "); ok && v == iface {
			return port
		}
	}
	return ""
}
