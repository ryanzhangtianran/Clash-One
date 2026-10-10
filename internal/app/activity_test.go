package app

import (
	"fmt"
	"net/http"
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
	if a.Mode != "rule" || a.Network == "" || a.Profile == "" {
		t.Fatalf("activity header %+v", a)
	}
	t.Logf("activity: conns=%d procs=%d devices=%d byProcess=%v", a.Connections, a.Processes, a.Devices, a.ByProcess)

	l := s.Latency()
	t.Logf("latency %+v", l)
	if l.Group != "代理" {
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
}

func TestPingMs(t *testing.T) {
	if ms := pingMs(t.Context(), "127.0.0.1"); ms < 1 {
		t.Fatalf("ping 127.0.0.1 = %d ms", ms)
	}
	if ms := pingMs(t.Context(), "invalid.invalid"); ms != 0 {
		t.Fatalf("ping of an unresolvable host = %d ms, want 0", ms)
	}
}
