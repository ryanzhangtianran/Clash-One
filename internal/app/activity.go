package app

import (
	"cmp"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Usage is a client, host or policy's traffic today; Icon is a file whose Finder icon stands for it.
type Usage struct {
	Name     string  `json:"name"`
	Icon     string  `json:"icon,omitempty"`
	Children []Usage `json:"children,omitempty"` // a client's own processes (an app's helpers), biggest first
	Split
}

type Activity struct {
	Today       Split     `json:"today"` // traffic counted by the ledger
	Month       Split     `json:"month"`
	Network     string    `json:"network"`
	Mode        string    `json:"mode"`
	Tun         bool      `json:"tun"` // enhanced mode
	Connections int       `json:"connections"`
	Processes   int       `json:"processes"`
	Devices     int       `json:"devices"`
	Hosts       int       `json:"hosts"`     // distinct domains (or IPs) with live connections
	Hours       [24]Split `json:"hours"`     // today, by hour of day
	ByProcess   []Usage   `json:"byProcess"` // today's top clients, hosts and policies
	ByHost      []Usage   `json:"byHost"`
	ByPolicy    []Usage   `json:"byPolicy"`
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
	conns, err := s.Connections()
	if err != nil {
		return Activity{}, err
	}
	a := Activity{Network: s.networkName(ctx), Mode: cfg.Mode, Tun: cfg.Tun.Enable, Connections: len(conns)}
	processes, hosts, devices := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range conns {
		name, _ := clientOf(c)
		processes[name], hosts[c.Host] = true, true
		if c.Process == "" && c.Path == "" { // no local process: a LAN device using us as a proxy or gateway
			if ip := net.ParseIP(c.Source); ip != nil && !ip.IsLoopback() {
				devices[c.Source] = true
			}
		}
	}
	a.Processes, a.Devices, a.Hosts = len(processes), len(devices), len(hosts)
	if s.ledger != nil {
		a.Today, a.Month = s.ledger.totals(time.Now())
		a.Hours, a.ByProcess, a.ByHost, a.ByPolicy = s.ledger.today(time.Now())
	}
	return a, nil
}

// networkName is the Wi-Fi SSID, or the default route's interface when it is unknown. macOS redacts
// the SSID from users, so only the core running as the root daemon can tell.
func (s *CoreService) networkName(ctx context.Context) string {
	var n struct {
		SSID string `json:"ssid"`
	}
	if s.core.Do(ctx, http.MethodGet, "/network", nil, &n) == nil && n.SSID != "" {
		return n.SSID
	}
	return cmp.Or(defaultRoute()["interface"], "未连接")
}

// defaultRoute is the IPv4 default route as key -> value, e.g. interface, gateway.
func defaultRoute() map[string]string { return routeOf("-inet") }

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

// Latency measures the router, the direct internet path, the system DNS and the policy of the
// final MATCH rule (where unmatched traffic goes); a DIRECT one is the internet path already.
func (s *CoreService) Latency() Latency {
	l := Latency{Group: cmp.Or(s.matchPolicy(), "代理")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if gw := defaultRoute()["gateway"]; gw != "" {
		l.Router = pingMs(ctx, gw)
	}
	start := time.Now()
	if _, err := net.DefaultResolver.LookupHost(ctx, "www.apple.com"); err == nil {
		l.DNS = ms(start)
	}
	direct := oneShotClient(0, 5*time.Second)
	start = time.Now()
	if resp, err := direct.Head(testURL); err == nil {
		resp.Body.Close()
		l.Internet = ms(start)
	}
	if node := s.exitOf(l.Group); node != "DIRECT" && node != "REJECT" {
		l.Proxy, _ = s.TestProxy(node)
	}
	return l
}

// matchPolicy is the policy of the final MATCH rule, where unmatched traffic goes; "" without one.
func (s *CoreService) matchPolicy() string {
	rules, _ := s.Rules()
	if n := len(rules); n > 0 && rules[n-1].Type == "Match" {
		return rules[n-1].Proxy
	}
	return ""
}

// exitOf follows policy through the groups' current choices to the node it uses now.
func (s *CoreService) exitOf(policy string) string {
	groups, _ := s.Groups()
	for range 10 { // ponytail: depth cap instead of cycle detection; groups nest a few levels at most
		i := slices.IndexFunc(groups, func(g Group) bool { return g.Name == policy })
		if i < 0 {
			break
		}
		policy = groups[i].Now
	}
	return policy
}

// ExternalIP is the address the internet sees for traffic through the core's mixed-port.
func (s *CoreService) ExternalIP() (string, error) {
	port, err := mixedPort(s.core)
	if err != nil {
		return "", err
	}
	resp, err := oneShotClient(port, 8*time.Second).Get("https://api.ipify.org")
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

// clientOf names a connection's client, an app folded into its bundle, the LAN device's address,
// or Connaio for the core's own connections (rule sets, health checks), with the file whose
// Finder icon stands for it (none for a device).
func clientOf(c Conn) (name, icon string) {
	if c.Inbound == "Inner" {
		return "Connaio", ownBundle
	}
	name, icon = c.Process, c.Path
	if i := strings.Index(c.Path, ".app/"); i >= 0 {
		icon = c.Path[:i+len(".app")]
		name = strings.TrimSuffix(filepath.Base(icon), ".app")
	}
	return cmp.Or(name, c.Source, "-"), icon
}

// ownBundle is this app's bundle, for its icon; empty when not run from one.
var ownBundle = func() string {
	exe, _ := os.Executable()
	if i := strings.Index(exe, ".app/"); i >= 0 {
		return exe[:i+len(".app")]
	}
	return ""
}()

func (s *Split) add(direct bool, n int64) {
	if direct {
		s.Direct += n
	} else {
		s.Proxy += n
	}
}

// Day is one day of traffic: its total (the old file held only that), by hour, and by client, host
// and policy.
type Day struct {
	Split
	Hours  [24]Split                   `json:"hours"`
	Client map[string]Split            `json:"client,omitempty"`
	Procs  map[string]map[string]Split `json:"procs,omitempty"` // client -> its processes
	Host   map[string]Split            `json:"host,omitempty"`
	Policy map[string]Split            `json:"policy,omitempty"`
}

// ledger keeps per-day traffic in traffic.json.
type ledger struct {
	path  string
	mu    sync.Mutex
	days  map[string]*Day   // "2006-01-02" -> traffic
	icons map[string]string // client -> icon file, as last seen
	seen  map[string]int64  // bytes per live connection at the last count; nil before the first
	dirty bool
}

func openLedger(path string) *ledger {
	l := &ledger{path: path, days: map[string]*Day{}, icons: map[string]string{"Connaio": ownBundle}}
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
	key := now.Format(time.DateOnly)
	d := l.days[key]
	if d == nil {
		d = &Day{}
		l.days[key] = d
	}
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
		direct := c.Chain[strings.LastIndex(c.Chain, " ")+1:] == "DIRECT"
		name, icon := clientOf(c)
		policy, _, _ := strings.Cut(c.Chain, " → ") // the outermost group the rule chose
		d.Split.add(direct, delta)
		d.Hours[now.Hour()].add(direct, delta)
		bump(&d.Client, name, direct, delta)
		bump(&d.Host, c.Host, direct, delta)
		bump(&d.Policy, policy, direct, delta)
		if proc := processName(c); proc != "" {
			if d.Procs == nil {
				d.Procs = map[string]map[string]Split{}
			}
			procs := d.Procs[name]
			bump(&procs, proc, direct, delta)
			d.Procs[name] = procs
		}
		if icon != "" {
			l.icons[name] = icon
		}
		l.dirty = true
	}
	l.seen = seen
	return deltas
}

// bump adds n to m[k], making m if it is nil.
func bump(m *map[string]Split, k string, direct bool, n int64) {
	if *m == nil {
		*m = map[string]Split{}
	}
	v := (*m)[k]
	v.add(direct, n)
	(*m)[k] = v
}

func (l *ledger) totals(now time.Time) (today, month Split) {
	l.mu.Lock()
	defer l.mu.Unlock()
	prefix := now.Format("2006-01-")
	for day, d := range l.days {
		if strings.HasPrefix(day, prefix) {
			month.Direct += d.Direct
			month.Proxy += d.Proxy
		}
	}
	if d := l.days[now.Format(time.DateOnly)]; d != nil {
		today = d.Split
	}
	return today, month
}

// today is today's traffic by hour, and its top clients, hosts and policies.
func (l *ledger) today(now time.Time) (hours [24]Split, clients, hosts, policies []Usage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	d := l.days[now.Format(time.DateOnly)]
	if d == nil {
		return
	}
	clients = topUsage(d.Client, l.icons, 20)
	for i := range clients {
		if procs := d.Procs[clients[i].Name]; len(procs) > 1 || (len(procs) == 1 && procs[clients[i].Name] == (Split{})) {
			clients[i].Children = topUsage(procs, nil, 20)
		}
	}
	return d.Hours, clients, topUsage(d.Host, nil, 20), topUsage(d.Policy, nil, 20)
}

// processName is the executable behind a connection, as opposed to the app it belongs to.
func processName(c Conn) string {
	if c.Path != "" {
		return filepath.Base(c.Path)
	}
	return c.Process
}

// topUsage is the n largest entries, biggest first (the page can rank by proxied traffic alone).
func topUsage(m map[string]Split, icons map[string]string, n int) []Usage {
	out := make([]Usage, 0, len(m))
	for name, s := range m {
		out = append(out, Usage{Name: name, Icon: icons[name], Split: s})
	}
	slices.SortFunc(out, func(a, b Usage) int {
		return cmp.Or(cmp.Compare(b.Direct+b.Proxy, a.Direct+a.Proxy), strings.Compare(a.Name, b.Name))
	})
	return out[:min(len(out), n)]
}

func (l *ledger) save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.dirty {
		return nil
	}
	for _, d := range l.days { // ponytail: hosts grow without bound in a day; keep the 500 busiest
		if len(d.Host) > 500 {
			top := topUsage(d.Host, nil, 500)
			d.Host = make(map[string]Split, len(top))
			for _, u := range top {
				d.Host[u.Name] = u.Split
			}
		}
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
