package profile

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

const sub = `mixed-port: 7890
external-controller: 127.0.0.1:9090
secret: abc
dns: {enable: true, nameserver: [223.5.5.5]}
proxies:
  - {name: hk, type: socks5, server: 1.2.3.4, port: 1080}
proxy-groups:
  - {name: 代理, type: select, proxies: [hk]}
rules:
  - MATCH,代理
`

func TestStore(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		if r.URL.Path == "/base64" {
			w.Write([]byte("dm1lc3M6Ly9leGFtcGxl"))
			return
		}
		w.Header().Set("subscription-userinfo", "upload=10; download=20; total=1073741824; expire=1893456000")
		w.Header().Set("profile-update-interval", "12")
		w.Write([]byte(sub))
	}))
	defer srv.Close()

	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddRemote("bad", srv.URL+"/base64"); err == nil {
		t.Fatal("a base64 link list must be rejected")
	}
	p, err := s.AddRemote("机场", srv.URL+"/sub")
	if err != nil {
		t.Fatal(err)
	}
	if gotUA != UserAgent || p.Usage == nil || p.Usage.Total != 1<<30 || p.Usage.Expire != 1893456000 || p.Interval != 12 {
		t.Fatalf("subscription metadata: ua=%q %+v usage=%+v", gotUA, p, p.Usage)
	}
	if cur, list := s.List(); cur != p.ID || len(list) != 1 {
		t.Fatalf("first profile should become current: %s %v", cur, list)
	}

	if err := s.SetOverride("not: [valid"); err == nil {
		t.Fatal("broken override accepted")
	}
	if err := s.SetOverride(`
dns: {nameserver: [https://1.1.1.1/dns-query]}
prepend-rules:
  - DOMAIN,corp.example,DIRECT
tun: {stack: system}
`); err != nil {
		t.Fatal(err)
	}
	b, err := s.Build(false)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["external-controller"] != nil || cfg["secret"] != nil {
		t.Fatal("control keys must be stripped")
	}
	if cfg["mixed-port"] != 7890 || cfg["find-process-mode"] != "always" {
		t.Fatalf("profile beats defaults, defaults fill the rest: %v %v", cfg["mixed-port"], cfg["find-process-mode"])
	}
	dns := cfg["dns"].(map[string]any)
	if dns["enable"] != true || dns["nameserver"].([]any)[0] != "https://1.1.1.1/dns-query" {
		t.Fatalf("maps merge deeply, lists are replaced: %v", dns)
	}
	if rules := cfg["rules"].([]any); len(rules) != 2 || rules[0] != "DOMAIN,corp.example,DIRECT" {
		t.Fatalf("prepend-rules: %v", rules)
	}
	if cfg["prepend-rules"] != nil {
		t.Fatal("prepend-rules must not reach the core")
	}

	if due := s.Due(time.Now().Add(13 * time.Hour)); len(due) != 1 {
		t.Fatalf("due after the interval: %v", due)
	}
	if _, err := s.Update(p.ID); err != nil {
		t.Fatal(err)
	}

	local, err := s.AddLocal("本地", []byte(sub))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(p.ID); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.List(); cur != local.ID {
		t.Fatalf("removing the current profile selects the next one, got %q", cur)
	}

	reopened, err := Open(s.dir[:len(s.dir)-len("/profiles")])
	if err != nil {
		t.Fatal(err)
	}
	if cur, list := reopened.List(); cur != local.ID || len(list) != 1 {
		t.Fatalf("index persists: %s %v", cur, list)
	}
}

func TestBuildWithoutProfile(t *testing.T) {
	s, _ := Open(t.TempDir())
	b, err := s.Build(false)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	_ = yaml.Unmarshal(b, &cfg)
	if cfg["mixed-port"] != 7899 || cfg["rules"].([]any)[0] != "MATCH,代理" {
		t.Fatalf("empty build: %v", cfg)
	}
}

func TestPrependRule(t *testing.T) {
	s, _ := Open(t.TempDir())
	if err := s.PrependRule("DOMAIN,a.com,DIRECT"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOverride("# mine\nmode: rule # keep\nprepend-rules:\n  - DOMAIN,a.com,DIRECT\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.PrependRule("DOMAIN-SUFFIX,b.com,代理"); err != nil {
		t.Fatal(err)
	}
	text, _ := s.Override()
	var m map[string]any
	_ = yaml.Unmarshal([]byte(text), &m)
	rules := m["prepend-rules"].([]any)
	if len(rules) != 2 || rules[0] != "DOMAIN-SUFFIX,b.com,代理" || m["mode"] != "rule" {
		t.Fatalf("prepend: %v", m)
	}
	if !strings.Contains(text, "# mine") || !strings.Contains(text, "# keep") {
		t.Fatalf("comments lost:\n%s", text)
	}
}

func TestCapture(t *testing.T) {
	cfg := map[string]any{"capture": map[string]any{"mode": "system-proxy"}}
	Capture(cfg, true)
	if cfg["capture"].(map[string]any)["mode"] != "tun" || cfg["dns"].(map[string]any)["enable"] != true {
		t.Fatalf("enhanced: %v", cfg)
	}
	cfg = map[string]any{"capture": map[string]any{"mode": "process-tun"}, "dns": map[string]any{"enable": true, "nameserver": []any{"1.1.1.1"}}}
	Capture(cfg, true)
	if cfg["capture"].(map[string]any)["mode"] != "process-tun" || cfg["dns"].(map[string]any)["nameserver"].([]any)[0] != "1.1.1.1" {
		t.Fatalf("explicit process-tun and the profile's DNS are kept: %v", cfg)
	}
	Capture(cfg, false)
	if cfg["capture"].(map[string]any)["mode"] != "system-proxy" {
		t.Fatalf("no daemon, no TUN: %v", cfg)
	}
}

func TestUserItems(t *testing.T) {
	s, _ := Open(t.TempDir())
	if _, err := s.AddLocal("sub", []byte(sub)); err != nil {
		t.Fatal(err)
	}
	_ = s.SetOverride("# my notes\nmode: rule\n")

	node := map[string]any{"name": "my-ss", "type": "ss", "server": "5.6.7.8", "port": 8388, "cipher": "aes-128-gcm", "password": "x"}
	if err := s.AddProxy(node, []string{"代理"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddProxy(node, nil); err == nil {
		t.Fatal("duplicate proxy name accepted")
	}
	if err := s.AddGroup(map[string]any{"name": "自动", "type": "url-test", "proxies": []any{"my-ss", "hk"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTunnel(map[string]any{"name": "vps", "type": "ssh", "server": "h", "port": 22}); err != nil {
		t.Fatal(err)
	}
	if err := s.PrependRule("DOMAIN,a.com,自动"); err != nil {
		t.Fatal(err)
	}

	b, _ := s.Build(false)
	var cfg map[string]any
	_ = yaml.Unmarshal(b, &cfg)
	proxies := cfg["proxies"].([]any)
	groups := cfg["proxy-groups"].([]any)
	if proxies[0].(map[string]any)["name"] != "my-ss" || len(proxies) != 2 {
		t.Fatalf("proxies: %v", proxies)
	}
	if groups[0].(map[string]any)["name"] != "自动" {
		t.Fatalf("user group first: %v", groups)
	}
	if members := groups[1].(map[string]any)["proxies"].([]any); members[0] != "my-ss" || members[1] != "hk" {
		t.Fatalf("group-members prepends into the subscription group: %v", members)
	}
	if cfg["group-members"] != nil || cfg["reverse-tunnels"].([]any)[0].(map[string]any)["name"] != "vps" {
		t.Fatalf("group-members must not reach the core; tunnels kept: %v", cfg)
	}

	items, _ := s.UserItems()
	if len(items.Proxies) != 1 || len(items.Groups) != 1 || len(items.Tunnels) != 1 || len(items.Rules) != 1 || len(items.Members["代理"]) != 1 {
		t.Fatalf("user items: %+v", items)
	}

	_ = s.RemoveProxy("my-ss")
	_ = s.RemoveGroup("自动")
	_ = s.RemoveTunnel("vps")
	_ = s.RemoveRule("DOMAIN,a.com,自动")
	items, _ = s.UserItems()
	if len(items.Proxies)+len(items.Groups)+len(items.Tunnels)+len(items.Rules)+len(items.Members["代理"]) != 0 {
		t.Fatalf("after removal: %+v", items)
	}
	if text, _ := s.Override(); !strings.Contains(text, "# my notes") {
		t.Fatalf("comments lost:\n%s", text)
	}
}

func TestBuildDoesNotLeak(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.AddProxy(map[string]any{"name": "n", "type": "socks5", "server": "1.1.1.1", "port": 1}, []string{"代理"})
	_, _ = s.Build(false)
	_ = s.RemoveProxy("n")
	b, _ := s.Build(false)
	if strings.Contains(string(b), "- n\n") {
		t.Fatalf("a removed proxy leaked into the next build:\n%s", b)
	}
}
