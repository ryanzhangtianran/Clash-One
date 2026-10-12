package app

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Needs a built core: CLASH_ONE_CORE=/path/to/clash-one-core go test -run TestActivity .
func TestActivity(t *testing.T) {
	s, _, port := startCore(t, "")

	// keep one connection open through the mixed-port so there is something to summarize
	ip, err := s.ExternalIP()
	t.Logf("external ip %q err=%v", ip, err)
	go http.Get(fmt.Sprintf("http://127.0.0.1:%d", port)) // the core answers on mixed-port; harmless if it errors
	time.Sleep(300 * time.Millisecond)

	a, err := s.Activity()
	if err != nil {
		t.Fatal(err)
	}
	if a.Mode != "rule" || a.Network == "" {
		t.Fatalf("activity header %+v", a)
	}
	t.Logf("activity: conns=%d procs=%d devices=%d byProcess=%v", a.Connections, a.Processes, a.Devices, a.ByProcess)

	l := s.Latency()
	t.Logf("latency %+v", l)
	if l.Group != "DIRECT" { // the empty profile's catch-all
		t.Fatalf("latency group %q", l.Group)
	}
}

func TestPortWarning(t *testing.T) {
	s, _, port := startCore(t, "")
	if w := s.portWarning(); w != "" {
		t.Fatalf("port %d is open, got warning %q", port, w)
	}
	// a second core told to use the first core's port can't open it, yet keeps running
	cs, _, _ := startCore(t, "")
	if err := cs.core.Reload(t.Context(), []byte(fmt.Sprintf("mixed-port: %d\nrules:\n  - MATCH,DIRECT\n", port))); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	w := cs.portWarning()
	if !strings.Contains(w, strconv.Itoa(port)) {
		t.Fatalf("expected a warning naming port %d, got %q", port, w)
	}
	t.Logf("warning: %s", w)
}

func TestLedger(t *testing.T) {
	l := openLedger(filepath.Join(t.TempDir(), "traffic.json"))
	day1 := time.Date(2026, 10, 9, 23, 0, 0, 0, time.Local)
	day2 := day1.Add(2 * time.Hour)
	l.add([]Conn{{ID: "old", Chain: "代理 → HK 01", Download: 500}}, day1) // baseline only
	l.add([]Conn{
		{ID: "old", Chain: "代理 → HK 01", Download: 800},
		{ID: "new", Chain: "DIRECT", Upload: 100},
	}, day1)
	l.add([]Conn{{ID: "new", Chain: "DIRECT", Upload: 150}}, day2)
	today, month := l.totals(day2)
	if today != (Split{Direct: 50}) || month != (Split{Direct: 150, Proxy: 300}) {
		t.Fatalf("today %+v month %+v", today, month)
	}
	if err := l.save(); err != nil {
		t.Fatal(err)
	}
	if _, month := openLedger(l.path).totals(day2); month != (Split{Direct: 150, Proxy: 300}) {
		t.Fatalf("reloaded month %+v", month)
	}
	hours, clients, _, policies := l.today(day1)
	if hours[23] != (Split{Direct: 100, Proxy: 300}) {
		t.Fatalf("hour 23 %+v", hours[23])
	}
	if len(clients) != 1 || clients[0].Name != "-" || len(policies) != 2 || policies[0].Name != "代理" {
		t.Fatalf("clients %+v policies %+v", clients, policies)
	}
}

func TestLedgerOldFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.json")
	if err := os.WriteFile(path, []byte(`{"2026-10-09":{"direct":7,"proxy":9}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if today, _ := openLedger(path).totals(time.Date(2026, 10, 9, 12, 0, 0, 0, time.Local)); today != (Split{Direct: 7, Proxy: 9}) {
		t.Fatalf("today %+v", today)
	}
}

func TestClientOf(t *testing.T) {
	if n, i := clientOf(Conn{Path: "/Applications/Google Chrome.app/Contents/Frameworks/x.app/Contents/MacOS/Helper"}); n != "Google Chrome" || i != "/Applications/Google Chrome.app" {
		t.Fatalf("%q %q", n, i)
	}
	if n, i := clientOf(Conn{Source: "192.168.1.9"}); n != "192.168.1.9" || i != "" {
		t.Fatalf("%q %q", n, i)
	}
	if n, _ := clientOf(Conn{Inbound: "Inner"}); n != "Clash One" {
		t.Fatalf("the core's own connection is %q", n)
	}
}

func TestPingMs(t *testing.T) {
	if ms := pingMs(t.Context(), "127.0.0.1"); ms < 1 {
		t.Fatalf("ping 127.0.0.1 = %d ms", ms)
	}
	if ms := pingMs(t.Context(), "invalid.invalid"); ms != 0 {
		t.Fatalf("ping of an unresolvable host = %d ms, want 0", ms)
	}
}

func TestDiagnoseNetwork(t *testing.T) {
	sec := diagNetwork(t.Context())
	if len(sec.Lines) != 6 {
		t.Fatalf("lines %v", sec.Lines)
	}
	t.Log(strings.Join(sec.Lines, "\n"), "\n", diagRouter(t.Context()).Lines, diagDNS(t.Context()).Lines, diagDirect(t.Context()).Lines)
}

func TestWhoResolves(t *testing.T) {
	r := whoResolves(&http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 10 * time.Second}, freshHost())
	if r.Err != "" || r.IP == "" {
		t.Skipf("edns.ip-api.com unreachable here: %+v", r)
	}
	t.Logf("%+v", r)
}

func TestAppIcon(t *testing.T) {
	for _, p := range []string{"/System/Applications/Calculator.app", "/bin/ls"} {
		if u := (&SettingsService{}).AppIcon(p); !strings.HasPrefix(u, "data:image/png;base64,") || len(u) < 500 {
			t.Fatalf("%s: %.60q", p, u)
		}
	}
}

func TestSymbol(t *testing.T) {
	if u := (&SettingsService{}).Symbol("gearshape.fill"); len(u) < 200 {
		t.Fatalf("%.60q", u)
	}
	if u := (&SettingsService{}).Symbol("no.such.symbol"); u != "" {
		t.Fatalf("unknown symbol gave %.40q", u)
	}
}

func TestLedgerChildren(t *testing.T) {
	l := openLedger(filepath.Join(t.TempDir(), "traffic.json"))
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.Local)
	app := "/Applications/Google Chrome.app/Contents/"
	conns := []Conn{{ID: "a", Path: app + "MacOS/Google Chrome"}, {ID: "b", Path: app + "Frameworks/x.app/Contents/MacOS/Google Chrome Helper"}}
	l.add(conns, now)
	conns[0].Download, conns[1].Download = 100, 300
	l.add(conns, now)
	_, clients, _, _ := l.today(now)
	if len(clients) != 1 || clients[0].Name != "Google Chrome" || len(clients[0].Children) != 2 || clients[0].Children[0].Name != "Google Chrome Helper" {
		t.Fatalf("%+v", clients)
	}
}

func TestFrpsScript(t *testing.T) {
	script := fmt.Sprintf(frpsScript, 7000, "abc")
	if strings.Contains(script, "%!") || !strings.Contains(script, `bindPort = 7000\nauth.method = "token"\nauth.token = "abc"`) {
		t.Fatalf("script:\n%s", script)
	}
	if !strings.Contains(script, "is-active --quiet frps || {") {
		t.Fatal("a frps that did not start must fail the deploy")
	}
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v %s", err, out)
	}
}

func TestNotifyCollapsesRepeats(t *testing.T) {
	eventLog.list = nil
	notify("error", "core down")
	notify("error", "core down") // a poll failing again
	notify("info", "core up")
	if len(eventLog.list) != 2 {
		t.Fatalf("events %+v", eventLog.list)
	}
}
