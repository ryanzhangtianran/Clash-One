package main

import (
	"context"
	"net"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// Device is another machine on the LAN using this Mac's proxy port (allow-lan).
type Device struct {
	IP          string `json:"ip"`
	Name        string `json:"name"`
	MAC         string `json:"mac"`
	Connections int    `json:"connections"`
	Upload      int64  `json:"upload"`
	Download    int64  `json:"download"`
	Policy      string `json:"policy"`    // set by a SRC-IP-CIDR rule, "" = follows the rules
	Temporary   bool   `json:"temporary"` // policy comes from a temporary rule
	RuleID      int    `json:"ruleId"`    // temporary rule id
	Rule        string `json:"rule"`      // permanent rule line
}

type LanInfo struct {
	AllowLan  bool     `json:"allowLan"`
	Addresses []string `json:"addresses"` // this Mac's LAN IPv4 addresses
	Port      int      `json:"port"`      // mixed-port: HTTP and SOCKS5
}

// DeviceService lists LAN devices and pins them to policies.
type DeviceService struct {
	core     *CoreService
	profiles *ProfileService

	mu    sync.Mutex
	names map[string]string // reverse DNS cache
}

func (s *DeviceService) Lan() (LanInfo, error) {
	var cfg struct {
		AllowLan  bool `json:"allow-lan"`
		MixedPort int  `json:"mixed-port"`
	}
	if err := s.core.core.Do(context.Background(), http.MethodGet, "/configs", nil, &cfg); err != nil {
		return LanInfo{}, err
	}
	return LanInfo{AllowLan: cfg.AllowLan, Addresses: lanAddresses(), Port: cfg.MixedPort}, nil
}

// SetAllowLan lets other devices use the proxy port; kept in the override.
func (s *DeviceService) SetAllowLan(on bool) error {
	return s.profiles.Set("allow-lan", on)
}

func lanAddresses() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && n.IP.IsPrivate() {
			out = append(out, n.IP.String())
		}
	}
	return out
}

// Devices groups live connections by source IP, leaving out this Mac's own processes and addresses.
func (s *DeviceService) Devices() ([]Device, error) {
	conns, err := s.core.Connections()
	if err != nil {
		return nil, err
	}
	own := lanAddresses()
	byIP := map[string]*Device{}
	for _, c := range conns {
		ip := net.ParseIP(c.Source)
		if c.Process != "" || ip == nil || ip.IsLoopback() || slices.Contains(own, c.Source) {
			continue
		}
		d := byIP[c.Source]
		if d == nil {
			d = &Device{IP: c.Source}
			byIP[c.Source] = d
		}
		d.Connections++
		d.Upload += c.Upload
		d.Download += c.Download
	}
	if err := s.attachPolicies(byIP); err != nil {
		return nil, err
	}
	macs := arpTable()
	devices := make([]Device, 0, len(byIP))
	for _, d := range byIP {
		d.MAC = macs[d.IP]
		d.Name = s.name(d.IP)
		devices = append(devices, *d)
	}
	slices.SortFunc(devices, func(a, b Device) int { return int((b.Upload + b.Download) - (a.Upload + a.Download)) })
	return devices, nil
}

// attachPolicies fills each device's policy from SRC-IP-CIDR <ip>/32 rules; a pinned device with
// no live connection is still listed so its policy can be removed.
func (s *DeviceService) attachPolicies(byIP map[string]*Device) error {
	device := func(ip string) *Device {
		if byIP[ip] == nil {
			byIP[ip] = &Device{IP: ip}
		}
		return byIP[ip]
	}
	items, err := s.profiles.UserItems()
	if err != nil {
		return err
	}
	for _, rule := range items.Rules {
		parts := strings.Split(rule, ",")
		if len(parts) >= 3 && parts[0] == "SRC-IP-CIDR" && strings.HasSuffix(parts[1], "/32") {
			d := device(strings.TrimSuffix(parts[1], "/32"))
			d.Policy, d.Rule = parts[2], rule
		}
	}
	temp, err := s.core.TempRules()
	if err != nil {
		return err
	}
	for _, r := range temp { // temporary rules match first, so they win
		if r.Type == "SrcIPCIDR" && strings.HasSuffix(r.Payload, "/32") {
			d := device(strings.TrimSuffix(r.Payload, "/32"))
			d.Policy, d.Temporary, d.RuleID = r.Proxy, true, r.ID
		}
	}
	return nil
}

// SetPolicy pins ip to policy with a temporary or permanent SRC-IP-CIDR rule, replacing an earlier pin.
func (s *DeviceService) SetPolicy(ip, policy string, permanent bool) error {
	if net.ParseIP(ip) == nil {
		return &net.ParseError{Type: "IP address", Text: ip}
	}
	if err := s.ClearPolicy(ip); err != nil {
		return err
	}
	rule := "SRC-IP-CIDR," + ip + "/32," + policy
	if permanent {
		return s.profiles.AddRule(rule)
	}
	_, err := s.core.AddTempRule(rule)
	if err == nil {
		_ = s.core.CloseSource(ip)
	}
	return err
}

// ClearPolicy removes ip's pins, so it follows the rules again.
func (s *DeviceService) ClearPolicy(ip string) error {
	byIP := map[string]*Device{}
	if err := s.attachPolicies(byIP); err != nil {
		return err
	}
	d := byIP[ip]
	if d == nil {
		return nil
	}
	if d.Temporary {
		if err := s.core.RemoveTempRule(d.RuleID); err != nil {
			return err
		}
	}
	if d.Rule != "" {
		return s.profiles.RemoveRule(d.Rule)
	}
	return nil
}

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

// name is the device's reverse DNS name (often set by the router from DHCP), cached.
func (s *DeviceService) name(ip string) string {
	s.mu.Lock()
	if s.names == nil {
		s.names = map[string]string{}
	}
	name, ok := s.names[ip]
	s.mu.Unlock()
	if ok {
		return name
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if names, err := net.DefaultResolver.LookupAddr(ctx, ip); err == nil && len(names) > 0 {
		name = strings.TrimSuffix(names[0], ".")
	}
	s.mu.Lock()
	s.names[ip] = name
	s.mu.Unlock()
	return name
}
