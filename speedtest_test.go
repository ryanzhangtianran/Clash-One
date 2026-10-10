package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSpeedTest(t *testing.T) {
	s, _, _ := startCore(t, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 5<<20))
	}))
	defer srv.Close()
	old := speedURL
	speedURL = srv.URL + "/down"
	defer func() { speedURL = old }()

	sp, err := s.SpeedTest("DIRECT")
	if err != nil {
		t.Fatal(err)
	}
	if sp.Bytes != 5<<20 || sp.Mbps <= 0 {
		t.Fatalf("speed %+v", sp)
	}
	t.Logf("local transfer: %.0f Mbps", sp.Mbps)
	if rules, _ := s.TempRules(); len(rules) != 0 {
		t.Fatalf("temporary rule left behind: %+v", rules)
	}
	if _, err := s.SpeedTest("REJECT"); err == nil {
		t.Fatal("a rejected path must fail")
	}
	if err := s.SetMode("global"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpeedTest("DIRECT"); err == nil || !strings.Contains(err.Error(), "规则判定") {
		t.Fatalf("global mode: %v", err)
	}
}
