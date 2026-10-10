package app

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Usage is one row of the activity traffic ranking.
type Usage struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

type Activity struct {
	Today       Split   `json:"today"` // traffic counted by the ledger
	Month       Split   `json:"month"`
	Network     string  `json:"network"`
	Profile     string  `json:"profile"`
	Mode        string  `json:"mode"`
	Tun         bool    `json:"tun"` // enhanced mode
	Connections int     `json:"connections"`
	Processes   int     `json:"processes"`
	Devices     int     `json:"devices"`
	Hosts       int     `json:"hosts"` // distinct domains (or IPs) with live connections
	ByProcess   []Usage `json:"byProcess"`
	ByHost      []Usage `json:"byHost"`
	ByPolicy    []Usage `json:"byPolicy"`
}

type Latency struct {
	Internet int    `json:"internet"` // direct HTTP round trip, ms; 0 = failed
	Router   int    `json:"router"`   // ping to the default gateway, ms
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
		Connections []struct {
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
		Network:     networkName(),
		Profile:     "Default",
		Mode:        cfg.Mode,
		Tun:         cfg.Tun.Enable,
		Connections: len(snap.Connections),
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
	a.Processes, a.Devices, a.Hosts = len(processes), len(devices), len(hosts)
	if s.ledger != nil {
		a.Today, a.Month = s.ledger.totals(time.Now())
	}
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
	iface := defaultRoute()["interface"]
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

// defaultRoute is `route -n get default` as key -> value, e.g. interface, gateway.
func defaultRoute() map[string]string {
	out, _ := exec.Command("route", "-n", "get", "default").Output()
	m := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), ":"); ok {
			m[k] = strings.TrimSpace(v)
		}
	}
	return m
}

// pingMs is one ICMP round trip to host via /sbin/ping (setuid, no root needed), 0 if it fails.
func pingMs(ctx context.Context, host string) int {
	out, err := exec.CommandContext(ctx, "ping", "-n", "-c", "1", "-t", "2", host).Output()
	if err != nil {
		return 0
	}
	_, rest, ok := strings.Cut(string(out), "time=")
	ms, _, _ := strings.Cut(rest, " ")
	f, err := strconv.ParseFloat(ms, 64)
	if !ok || err != nil {
		return 0
	}
	return max(1, int(f+0.5))
}

// Latency measures the router, the direct internet path, the system DNS and the selected node of ProxyGroup.
func (s *CoreService) Latency() Latency {
	l := Latency{Group: "代理"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if gw := defaultRoute()["gateway"]; gw != "" {
		l.Router = pingMs(ctx, gw)
	}
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

// Split is traffic in bytes, by whether it left through DIRECT or a proxy.
type Split struct {
	Direct int64 `json:"direct"`
	Proxy  int64 `json:"proxy"`
}

// ledger keeps per-day traffic totals in traffic.json.
type ledger struct {
	path  string
	mu    sync.Mutex
	days  map[string]Split // "2006-01-02" -> bytes
	seen  map[string]int64 // bytes per live connection at the last count; nil before the first
	dirty bool
}

func openLedger(path string) *ledger {
	l := &ledger{path: path, days: map[string]Split{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &l.days)
	}
	return l
}

// add counts what each connection moved since the last call, and returns those deltas by connection
// id; one new since then counts in full, while the first call only takes a baseline (nil: traffic
// from before the GUI started is not ours to count).
// ponytail: bytes moved in a connection's last second before it closes are lost; a stream of closed
// connections from the core would catch them.
func (l *ledger) add(conns []Conn, now time.Time) map[string]int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := now.Format(time.DateOnly)
	d := l.days[day]
	seen := make(map[string]int64, len(conns))
	var deltas map[string]int64
	if l.seen != nil {
		deltas = make(map[string]int64, len(conns))
	}
	for _, c := range conns {
		bytes := c.Upload + c.Download
		seen[c.ID] = bytes
		delta := bytes - l.seen[c.ID]
		if l.seen == nil || delta <= 0 {
			continue
		}
		deltas[c.ID] = delta
		if exit := c.Chain[strings.LastIndex(c.Chain, " ")+1:]; exit == "DIRECT" {
			d.Direct += delta
		} else {
			d.Proxy += delta
		}
		l.dirty = true
	}
	l.days[day], l.seen = d, seen
	return deltas
}

func (l *ledger) totals(now time.Time) (today, month Split) {
	l.mu.Lock()
	defer l.mu.Unlock()
	prefix := now.Format("2006-01-")
	for day, s := range l.days {
		if strings.HasPrefix(day, prefix) {
			month.Direct += s.Direct
			month.Proxy += s.Proxy
		}
	}
	return l.days[now.Format(time.DateOnly)], month
}

func (l *ledger) save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.dirty {
		return nil
	}
	b, err := json.Marshal(l.days)
	if err != nil {
		return err
	}
	l.dirty = false
	return os.WriteFile(l.path, b, 0o644)
}

// countTraffic feeds the ledger every second (and the menu bar's process rows), saving it every minute.
func (s *CoreService) countTraffic(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for i := 1; ; i++ {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if conns, err := s.Connections(); err == nil {
				deltas := s.ledger.add(conns, now)
				if s.onConns != nil {
					s.onConns(conns, deltas)
				}
			}
			if i%60 == 0 {
				if err := s.ledger.save(); err != nil {
					log.Printf("save traffic: %v", err)
				}
			}
		}
	}
}
