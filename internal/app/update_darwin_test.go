package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceBundle(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "new.app"), filepath.Join(dir, "Connaio.app")
	for p, v := range map[string]string{filepath.Join(src, "Contents/v"): "2", filepath.Join(dst, "Contents/v"): "1", filepath.Join(dst, "Contents/stale"): "x"} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(v), 0o644)
	}
	if err := replaceBundle(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "Contents/v")); string(b) != "2" {
		t.Fatalf("got version %q", b)
	}
	if _, err := os.Stat(filepath.Join(dst, "Contents/stale")); err == nil {
		t.Fatal("files of the old version must go")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 { // new.app and Connaio.app: no .new or .old left
		t.Fatalf("left behind: %v", entries)
	}
}
