package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLoginItemPlist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "login.plist")
	os.WriteFile(p, []byte(loginItemPlist("/Applications/Clash One & Co.app/Contents/MacOS/clash-one")), 0o644)
	if out, err := exec.Command("plutil", "-lint", p).CombinedOutput(); err != nil {
		t.Fatalf("invalid plist: %s", out)
	}
	out, _ := exec.Command("plutil", "-extract", "ProgramArguments.0", "raw", p).Output()
	if string(out) != "/Applications/Clash One & Co.app/Contents/MacOS/clash-one\n" {
		t.Fatalf("path round trip: %q", out)
	}
}

func TestSettingsApplied(t *testing.T) {
	cs, ps, _ := startCore(t, "")
	for path, value := range map[string]any{
		"log-level":                "warning",
		"dns.enable":               true,
		"dns.enhanced-mode":        "redir-host",
		"dns.nameserver":           []string{"223.5.5.5"},
		"dns.use-system-resolvers": true,
		"capture.processes":        []string{"ghostty"},
	} {
		if err := ps.Set(path, value); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if v, _ := ps.Value("dns.enhanced-mode"); v != "redir-host" {
		t.Fatalf("value %v", v)
	}
	if err := ps.Set("dns.enhanced-mode", "nonsense"); err == nil {
		t.Fatal("the core must reject an unknown enhanced-mode")
	}
	if v, _ := ps.Value("dns.enhanced-mode"); v != "redir-host" {
		t.Fatalf("rejected setting must roll back, got %v", v)
	}
	var cfg struct {
		LogLevel string `json:"log-level"`
	}
	if err := cs.core.Do(t.Context(), "GET", "/configs", nil, &cfg); err != nil || cfg.LogLevel != "warning" {
		t.Fatalf("log-level applied: %q %v", cfg.LogLevel, err)
	}
}

func TestParseApps(t *testing.T) {
	apps := parseApps(`/Applications/Ghostty.app/Contents/MacOS/ghostty
/Applications/Lark.app/Contents/MacOS/Feishu
/Applications/Lark.app/Contents/Frameworks/Lark Framework.framework/Versions/1/Helpers/Lark Helper.app/Contents/MacOS/Lark Helper
/Applications/Surge.app/Contents/Applications/Surge Dashboard.app/Contents/MacOS/Surge Dashboard
/Applications/Ghostty.app/Contents/MacOS/ghostty
/usr/sbin/mDNSResponder
`)
	if len(apps) != 2 || apps[0].Name != "Ghostty" || apps[0].Exec != "ghostty" || apps[1].Name != "Lark" || apps[1].Exec != "Feishu" {
		t.Fatalf("apps %+v", apps)
	}
}

func TestAppExecutable(t *testing.T) {
	c, err := appExecutable("/System/Applications/Calculator.app")
	if err != nil || c.Exec != "Calculator" || c.Name != "Calculator" {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := appExecutable(t.TempDir()); err == nil {
		t.Fatal("a folder that isn't an app must be rejected")
	}
}

func TestRate(t *testing.T) {
	for b, want := range map[int64]string{0: "0 KB/s", 999_999: "999 KB/s", 1_000_000: "1.0 MB/s", 99_949_999: "99.9 MB/s", 99_950_000: "100 MB/s"} {
		if got := rate(b); got != want {
			t.Errorf("rate(%d) = %q, want %q", b, got, want)
		}
	}
}
