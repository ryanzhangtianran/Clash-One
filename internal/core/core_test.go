package core

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

// Needs a built core: CLASH_ONE_CORE=/path/to/clash-one-core go test ./internal/core
func TestCore(t *testing.T) {
	if os.Getenv("CLASH_ONE_CORE") == "" {
		t.Skip("CLASH_ONE_CORE not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := New(t.TempDir())
	if err := os.WriteFile(c.ConfigPath(), []byte("mixed-port: 0\nrules:\n  - MATCH,DIRECT\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(c.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()

	if v, err := c.Version(ctx); err != nil || v == "" {
		t.Fatalf("version %q, %v", v, err)
	}
	if err := c.Do(ctx, http.MethodPatch, "/configs", map[string]string{"mode": "global"}, nil); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Mode string `json:"mode"`
	}
	if err := c.Do(ctx, http.MethodGet, "/configs", nil, &cfg); err != nil || cfg.Mode != "global" {
		t.Fatalf("mode %q, %v", cfg.Mode, err)
	}
	if err := c.Do(ctx, http.MethodGet, "/no-such-endpoint", nil, nil); err == nil {
		t.Fatal("expected an error for a 404")
	}

	got := make(chan []byte, 1)
	streamCtx, stop := context.WithCancel(ctx)
	go c.Stream(streamCtx, "/traffic", func(line []byte) {
		select {
		case got <- append([]byte(nil), line...):
		default:
		}
	})
	select {
	case line := <-got:
		t.Logf("traffic: %s", line)
	case <-time.After(5 * time.Second):
		t.Fatal("no traffic line")
	}
	stop()

	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Version(ctx); err == nil {
		t.Fatal("core still answering after Stop")
	}
}
