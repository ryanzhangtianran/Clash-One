package main

import (
	"fmt"
	"net/http"
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
