package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Usage is one row of the activity traffic ranking.
type Usage struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

type Activity struct {
	Network       string  `json:"network"`
	Profile       string  `json:"profile"`
	Mode          string  `json:"mode"`
	Tun           bool    `json:"tun"` // enhanced mode
	Connections   int     `json:"connections"`
	Processes     int     `json:"processes"`
	Devices       int     `json:"devices"`
	UploadTotal   int64   `json:"uploadTotal"`
	DownloadTotal int64   `json:"downloadTotal"`
	ByProcess     []Usage `json:"byProcess"`
	ByHost        []Usage `json:"byHost"`
	ByPolicy      []Usage `json:"byPolicy"`
}

type Latency struct {
	Internet int    `json:"internet"` // direct HTTP round trip, ms; 0 = failed
	DNS      int    `json:"dns"`      // system resolver lookup, ms
	Proxy    int    `json:"proxy"`    // current node of ProxyGroup, ms
	Group    string `json:"group"`
}

const testURL = "https://www.gstatic.com/generate_204"

// Activity summarizes the live connections for the activity page; polled by the frontend.
func (s *CoreService) Activity() (Activity, error) {
	ctx := context.Background()
	var cfg struct {
		Mode string `json:"mode"`
		Tun  struct {
			Enable bool `json:"enable"`
		} `json:"tun"`
	}
	if err := s.core.Do(ctx, http.MethodGet, "/configs", nil, &cfg); err != nil {
		return Activity{}, err
	}
	var snap struct {
		UploadTotal   int64 `json:"uploadTotal"`
		DownloadTotal int64 `json:"downloadTotal"`
		Connections   []struct {
			Upload   int64    `json:"upload"`
			Download int64    `json:"download"`
			Chains   []string `json:"chains"`
			Metadata struct {
				SourceIP    string `json:"sourceIP"`
				Host        string `json:"host"`
				DestIP      string `json:"destinationIP"`
				Process     string `json:"process"`
				ProcessPath string `json:"processPath"`
			} `json:"metadata"`
		} `json:"connections"`
	}
	if err := s.core.Do(ctx, http.MethodGet, "/connections", nil, &snap); err != nil {
		return Activity{}, err
	}

	a := Activity{
		Network:       networkName(),
		Profile:       "Default",
		Mode:          cfg.Mode,
		Tun:           cfg.Tun.Enable,
		Connections:   len(snap.Connections),
		UploadTotal:   snap.UploadTotal,
		DownloadTotal: snap.DownloadTotal,
	}
	processes, hosts, policies := map[string]int64{}, map[string]int64{}, map[string]int64{}
	devices := map[string]bool{}
	for _, c := range snap.Connections {
		bytes := c.Upload + c.Download
		m := c.Metadata
		name := m.Process
		if name == "" && m.ProcessPath != "" {
			name = filepath.Base(m.ProcessPath)
		}
		if name == "" { // no local process: a LAN device using us as a proxy or gateway
			name = m.SourceIP
			if ip := net.ParseIP(m.SourceIP); ip != nil && !ip.IsLoopback() {
				devices[m.SourceIP] = true
			}
		}
		processes[name] += bytes
		host := m.Host
		if host == "" {
			host = m.DestIP
		}
		hosts[host] += bytes
		if len(c.Chains) > 0 {
			policies[c.Chains[len(c.Chains)-1]] += bytes // outermost group the rule chose
		}
	}
	a.Processes, a.Devices = len(processes), len(devices)
	a.ByProcess, a.ByHost, a.ByPolicy = top(processes), top(hosts), top(policies)
	return a, nil
}

// top returns the 8 largest entries, biggest first.
func top(m map[string]int64) []Usage {
	out := make([]Usage, 0, len(m))
	for name, bytes := range m {
		out = append(out, Usage{name, bytes})
	}
	slices.SortFunc(out, func(a, b Usage) int { return int(b.Bytes - a.Bytes) })
	return out[:min(len(out), 8)]
}

// networkName is the Wi-Fi SSID, or the default route's interface when it isn't Wi-Fi
// (macOS hides the SSID from apps without location permission).
func networkName() string {
	out, _ := exec.Command("route", "-n", "get", "default").Output()
	iface := ""
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && k == "interface" {
			iface = strings.TrimSpace(v)
		}
	}
	if iface == "" {
		return "未连接"
	}
	if out, err := exec.Command("networksetup", "-getairportnetwork", iface).Output(); err == nil {
		if _, ssid, ok := strings.Cut(strings.TrimSpace(string(out)), "Current Wi-Fi Network: "); ok {
			return ssid
		}
	}
	return iface
}

// Latency measures the direct internet path, the system DNS and the selected node of ProxyGroup.
func (s *CoreService) Latency() Latency {
	l := Latency{Group: "代理"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	if _, err := net.DefaultResolver.LookupHost(ctx, "www.apple.com"); err == nil {
		l.DNS = ms(start)
	}
	direct := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	start = time.Now()
	if resp, err := direct.Head(testURL); err == nil {
		resp.Body.Close()
		l.Internet = ms(start)
	}
	var now struct {
		Now string `json:"now"`
	}
	if s.core.Do(ctx, http.MethodGet, "/proxies/"+url.PathEscape(l.Group), nil, &now) == nil && now.Now != "" {
		var d struct {
			Delay int `json:"delay"`
		}
		q := url.Values{"url": {testURL}, "timeout": {"5000"}}
		if s.core.Do(ctx, http.MethodGet, "/proxies/"+url.PathEscape(now.Now)+"/delay?"+q.Encode(), nil, &d) == nil {
			l.Proxy = d.Delay
		}
	}
	return l
}

// ExternalIP is the address the internet sees for traffic through the core's mixed-port.
func (s *CoreService) ExternalIP() (string, error) {
	var cfg struct {
		MixedPort int `json:"mixed-port"`
	}
	if err := s.core.Do(context.Background(), http.MethodGet, "/configs", nil, &cfg); err != nil {
		return "", err
	}
	proxy := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(cfg.MixedPort)}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}, Timeout: 8 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	return strings.TrimSpace(string(b)), err
}

func ms(start time.Time) int { return max(1, int(time.Since(start).Milliseconds())) }
