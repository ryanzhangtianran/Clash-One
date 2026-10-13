package app

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[DiagSection]("diagnose")
}

// ToolService runs diagnostics and tests through the core (network diagnosis, DNS leak, speed,
// UDP and NAT checks) and deploys frps; it keeps no core state of its own.
type ToolService struct {
	cs       *CoreService
	mu       sync.Mutex
	stopDiag context.CancelFunc // cancels a running Diagnose
}

// DiagSection is one step of the network diagnosis, sent as it finishes.
type DiagSection struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
	OK    bool     `json:"ok"`
}

const directTestURL = "http://www.apple.com/library/test/success.html"

// Diagnose checks, step by step as Surge does, the config, this Mac's network, the router, DNS,
// a direct connection and the proxies in use, emitting each section as it is done. CancelDiagnose
// stops it.
func (s *ToolService) Diagnose() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	s.mu.Lock()
	if s.stopDiag != nil {
		s.stopDiag()
	}
	s.stopDiag = cancel
	s.mu.Unlock()
	defer cancel()

	steps := []func(context.Context) DiagSection{s.diagConfig, diagNetwork, diagRouter, diagDNS, diagDirect, s.diagProxies}
	for _, step := range steps {
		if ctx.Err() != nil {
			return nil
		}
		emit("diagnose", step(ctx))
	}
	return nil
}

func (s *ToolService) CancelDiagnose() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopDiag != nil {
		s.stopDiag()
	}
}

func (s *ToolService) diagConfig(ctx context.Context) DiagSection {
	if err := s.cs.core.Validate(ctx, s.cs.core.ConfigPath()); err != nil {
		return DiagSection{Title: "检查配置", Lines: []string{err.Error()}}
	}
	return DiagSection{Title: "检查配置", Lines: []string{"配置检查成功"}, OK: true}
}

func diagNetwork(context.Context) DiagSection {
	route := defaultRoute()
	v6 := routeOf("-inet6")
	lines := []string{
		"首选网络接口（IPv4）：" + cmp.Or(route["interface"], "无"),
		"首选 IP 地址（IPv4）：" + cmp.Or(ifaceAddr(route["interface"], false), "无"),
		"默认路由（IPv4）：" + cmp.Or(route["gateway"], "无"),
		"首选网络接口（IPv6）：" + cmp.Or(v6["interface"], "不适用"),
		"首选 IP 地址（IPv6）：" + cmp.Or(ifaceAddr(v6["interface"], true), "不适用"),
		"DNS 服务器：" + cmp.Or(strings.Join(dnsServers(), "、"), "无"),
	}
	return DiagSection{Title: "网络配置", Lines: lines, OK: route["interface"] != ""}
}

func diagRouter(ctx context.Context) DiagSection {
	gw := defaultRoute()["gateway"]
	if gw == "" {
		return DiagSection{Title: "路由器", Lines: []string{"没有默认路由"}}
	}
	if ms := pingMs(ctx, gw); ms > 0 {
		return DiagSection{Title: "路由器", Lines: []string{fmt.Sprintf("Ping %s：%d ms", gw, ms)}, OK: true}
	}
	return DiagSection{Title: "路由器", Lines: []string{fmt.Sprintf("Ping %s：无响应", gw)}}
}

func diagDNS(ctx context.Context) DiagSection {
	servers := dnsServers()
	if len(servers) == 0 {
		return DiagSection{Title: "测试 DNS", Lines: []string{"没有 DNS 服务器"}}
	}
	addr := net.JoinHostPort(servers[0], "53")
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := r.LookupHost(ctx, "www.apple.com"); err != nil {
		return DiagSection{Title: "测试 DNS", Lines: []string{fmt.Sprintf("%s 没有应答：%v", addr, err)}}
	}
	return DiagSection{Title: "测试 DNS", Lines: []string{fmt.Sprintf("收到 %s 的应答：%d ms", addr, ms(start))}, OK: true}
}

func diagDirect(ctx context.Context) DiagSection {
	client := oneShotClient(0, 5*time.Second)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, directTestURL, nil)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return DiagSection{Title: "测试直连策略", Lines: []string{"直接连接失败：" + err.Error()}}
	}
	resp.Body.Close()
	return DiagSection{Title: "测试直连策略", Lines: []string{fmt.Sprintf("直接连接到 %s：%d ms", directTestURL, ms(start))}, OK: true}
}

// diagProxies tests the node each selectable group currently uses, once per node.
func (s *ToolService) diagProxies(ctx context.Context) DiagSection {
	groups, err := s.cs.Groups()
	if err != nil {
		return DiagSection{Title: "测试代理策略", Lines: []string{err.Error()}}
	}
	sec := DiagSection{Title: "测试代理策略", OK: true}
	seen := map[string]bool{"DIRECT": true, "REJECT": true, "": true}
	for _, g := range groups {
		if g.Name == "GLOBAL" || seen[g.Now] {
			continue
		}
		seen[g.Now] = true
		delay, err := s.cs.testProxy(ctx, g.Now)
		if err != nil {
			sec.Lines = append(sec.Lines, fmt.Sprintf("通过 %s 连接失败", g.Now))
			sec.OK = false
			continue
		}
		sec.Lines = append(sec.Lines, fmt.Sprintf("通过 %s 连接到 %s：%d ms", g.Now, testURL, delay))
	}
	if len(sec.Lines) == 0 {
		sec.Lines = []string{"没有在用的代理"}
	}
	return sec
}

// ifaceAddr is the interface's first IPv4 (or global IPv6) address.
func ifaceAddr(name string, v6 bool) string {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, _ := ifc.Addrs()
	for _, a := range addrs {
		ip, _, err := net.ParseCIDR(a.String())
		if err == nil && (ip.To4() == nil) == v6 && ip.IsGlobalUnicast() {
			return ip.String()
		}
	}
	return ""
}

// Resolver is which DNS server looked a test domain up, as the test service saw it.
type Resolver struct {
	IP  string `json:"ip"`
	Geo string `json:"geo"`
	Err string `json:"err,omitempty"`
}

// DNSLeak looks up a fresh random domain through the proxy unmatched traffic uses and directly.
// The test service names the resolver that asked for it: through the proxy that should be the
// proxy's, so the same resolver both times means proxied domains are resolved locally, a leak.
type DNSLeak struct {
	Proxy  Resolver `json:"proxy"`
	Direct Resolver `json:"direct"`
	Leak   bool     `json:"leak"`
}

func (s *ToolService) DNSLeakTest() (DNSLeak, error) {
	policy := s.cs.matchPolicy()
	if exit := s.cs.exitOf(policy); exit == "" || exit == "DIRECT" || exit == "REJECT" {
		return DNSLeak{}, fmt.Errorf("未匹配的流量经 %s 直连，没有代理可检测", cmp.Or(policy, "DIRECT"))
	}
	host := freshHost()
	proxied, done, err := s.viaProxy(policy, "http://"+host) // a temporary rule sends it to policy
	if err != nil {
		return DNSLeak{}, err
	}
	defer done()
	proxied.Timeout = 10 * time.Second
	r := DNSLeak{Proxy: whoResolves(proxied, host)}
	r.Direct = whoResolves(oneShotClient(0, 10*time.Second), freshHost())
	r.Leak = r.Proxy.IP != "" && r.Proxy.IP == r.Direct.IP
	return r, nil
}

// freshHost is a random subdomain of edns.ip-api.com (it wants exactly 32 characters), so no cache
// answers for it.
func freshHost() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + ".edns.ip-api.com"
}

// whoResolves asks edns.ip-api.com, at host from freshHost, which resolver queried it.
// ponytail: one third-party service; dnsleaktest-style multi-probe runs if it's flaky.
func whoResolves(c *http.Client, host string) Resolver {
	resp, err := c.Get("http://" + host + "/json")
	if err != nil {
		return Resolver{Err: err.Error()}
	}
	defer resp.Body.Close()
	var v struct {
		DNS struct {
			IP  string `json:"ip"`
			Geo string `json:"geo"`
		} `json:"dns"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return Resolver{Err: err.Error()}
	}
	return Resolver{IP: v.DNS.IP, Geo: v.DNS.Geo}
}
