package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoginItemFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "login.plist")
	os.WriteFile(p, []byte(loginItemFile("/Applications/Connaio & Co.app/Contents/MacOS/connaio")), 0o644)
	if out, err := exec.Command("plutil", "-lint", p).CombinedOutput(); err != nil {
		t.Fatalf("invalid plist: %s", out)
	}
	out, _ := exec.Command("plutil", "-extract", "ProgramArguments.0", "raw", p).Output()
	if string(out) != "/Applications/Connaio & Co.app/Contents/MacOS/connaio\n" {
		t.Fatalf("path round trip: %q", out)
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

func TestAppIcon(t *testing.T) {
	for _, p := range []string{"/System/Applications/Calculator.app", "/bin/ls"} {
		if u := (&SettingsService{}).AppIcon(p); !strings.HasPrefix(u, "data:image/png;base64,") || len(u) < 500 {
			t.Fatalf("%s: %.60q", p, u)
		}
	}
}
