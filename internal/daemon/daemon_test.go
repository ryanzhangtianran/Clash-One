package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d.plist")
	if err := os.WriteFile(p, []byte(plist(501)), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("plutil", "-lint", p).CombinedOutput(); err != nil {
		t.Fatalf("plist invalid: %s", out)
	}
	out, err := exec.Command("plutil", "-extract", "EnvironmentVariables.CONNAIO_SOCKET_UID", "raw", p).Output()
	if err != nil || strings.TrimSpace(string(out)) != "501" {
		t.Fatalf("socket uid: %q %v", out, err)
	}
}

// TestVerify runs the script's checksum line against a real file, as root would.
func TestVerify(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "helper")
	os.WriteFile(f, []byte("good"), 0o644)
	line := func(want string) string {
		for _, l := range strings.Split(installScript("/x", want, dir, want), "\n") {
			if strings.Contains(l, "shasum") {
				return strings.ReplaceAll(l, shellQuote(Helper), shellQuote(f)) // point the check at the test file
			}
		}
		t.Fatal("no checksum line")
		return ""
	}
	good := sum([]byte("good"))
	if out, err := exec.Command("sh", "-c", strings.ReplaceAll(line(good), Helper, f)).CombinedOutput(); err != nil {
		t.Fatalf("matching file rejected: %s", out)
	}
	if out, err := exec.Command("sh", "-c", strings.ReplaceAll(line(sum([]byte("other"))), Helper, f)).CombinedOutput(); err == nil {
		t.Fatalf("swapped file accepted: %s", out)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("a mismatching file must be removed")
	}
}

func TestScripts(t *testing.T) {
	// a path with spaces and a quote must survive shell and AppleScript quoting
	core := "/Applications/Connaio's.app/Contents/MacOS/connaio-core"
	script := installScript(core, "abc", "/tmp/stage", "def")
	if !strings.Contains(script, `'/Applications/Connaio'\''s.app/Contents/MacOS/connaio-core'`) {
		t.Fatalf("core path quoting:\n%s", script)
	}
	if out, err := exec.Command("sh", "-n", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("install script syntax: %s", out)
	}
	if out, err := exec.Command("sh", "-n", "-c", uninstallScript()).CombinedOutput(); err != nil {
		t.Fatalf("uninstall script syntax: %s", out)
	}
	// AppleScript string literal round trip, without the admin prompt
	out, err := exec.Command("osascript", "-e", "return "+appleQuote(script)).Output()
	if err != nil || strings.TrimSpace(string(out)) != strings.TrimSpace(script) {
		t.Fatalf("apple quoting changed the script: %v\n%s", err, out)
	}
}
