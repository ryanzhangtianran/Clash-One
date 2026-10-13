//go:build unix

package core

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const coreExe = "connaio-core"

// controlAddr is the child's Unix socket; kept short, macOS caps socket paths at 104 bytes.
func controlAddr(dir string) string {
	socket := filepath.Join(dir, "core.sock")
	_ = os.Remove(socket) // a stale socket from a crash would make the core fail to listen
	return socket
}

func controlArgs(addr string) []string { return []string{"-ext-ctl-unix", addr} }

func dialControl(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", addr)
}

// stopStale ends the core recorded in pidFile if it is still running, waiting up to 5s.
func stopStale(pidFile string) {
	pid, ok := stalePid(pidFile)
	if !ok {
		return
	}
	// only signal it if the pid still belongs to a connaio-core (pids get reused)
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil || filepath.Base(strings.TrimSpace(string(out))) != coreExe {
		_ = os.Remove(pidFile)
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	_ = os.Remove(pidFile)
}

// stop ends the child gently (it restores routes and listeners on SIGTERM), killing it after 5s.
func stop(cmd *exec.Cmd, exited <-chan struct{}) {
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
	}
}
