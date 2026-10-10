package core

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Needs CLASH_ONE_CORE.
func TestStaleCoreAndExitReason(t *testing.T) {
	if os.Getenv("CLASH_ONE_CORE") == "" {
		t.Skip("CLASH_ONE_CORE not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()
	cfg := []byte("mixed-port: 0\nrules:\n  - MATCH,DIRECT\n")

	first := New(dir)
	os.WriteFile(first.ConfigPath(), cfg, 0o644)
	if err := first.Start(ctx); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dir + "/core.pid")
	oldPid, _ := strconv.Atoi(string(b))

	// a new GUI (fresh Core, same data dir) after the first one crashed without stopping its core
	second := New(dir)
	if err := second.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer second.Stop()
	if syscall.Kill(oldPid, 0) == nil {
		t.Fatalf("stale core %d still running", oldPid)
	}
	if _, err := os.Stat(dir + "/core.prev.log"); err != nil {
		t.Fatal("previous log not kept")
	}
	second.Stop()

	// a core that exits at once reports why
	os.WriteFile(second.ConfigPath(), []byte("rules:\n  - NOT-A-RULE,x,DIRECT\n"), 0o644)
	err := New(dir).Start(ctx)
	if err == nil || !strings.Contains(err.Error(), "NOT-A-RULE") {
		t.Fatalf("exit reason not reported: %v", err)
	}
	t.Logf("startup error: %v", err)
}

// mixed-port taken by another program: the core keeps running without it.
func TestPortTaken(t *testing.T) {
	if os.Getenv("CLASH_ONE_CORE") == "" {
		t.Skip("CLASH_ONE_CORE not set")
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	c := New(dir)
	os.WriteFile(c.ConfigPath(), []byte("mixed-port: "+strconv.Itoa(port)+"\nrules:\n  - MATCH,DIRECT\n"), 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("the core should still start: %v", err)
	}
	defer c.Stop()
	log, _ := os.ReadFile(dir + "/core.log")
	if !strings.Contains(string(log), "address already in use") {
		t.Fatalf("expected a bind error in the log:\n%s", log)
	}
}
