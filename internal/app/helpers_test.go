package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/ryanzhangtianran/Clash-One/internal/core"
	"github.com/ryanzhangtianran/Clash-One/internal/profile"
)

// startCore runs a core from CLASH_ONE_CORE on a free mixed-port (a running Clash One may hold
// 7899), with override on top of the default config. Skips without CLASH_ONE_CORE.
func startCore(t *testing.T, override string) (*CoreService, *ProfileService, int) {
	t.Helper()
	if os.Getenv("CLASH_ONE_CORE") == "" {
		t.Skip("CLASH_ONE_CORE not set")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	dir := t.TempDir()
	store, _ := profile.Open(dir)
	if err := store.SetOverride(fmt.Sprintf("mixed-port: %d\n%s", port, override)); err != nil {
		t.Fatal(err)
	}
	cs := &CoreService{core: core.New(dir)}
	ps := &ProfileService{store: store, core: cs.core}
	if err := ps.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := cs.core.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.core.Stop() })
	return cs, ps, port
}
