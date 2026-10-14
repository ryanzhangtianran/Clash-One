package core

import (
	"context"
	"github.com/ryanzhangtianran/Connaio/internal/run"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"github.com/Microsoft/go-winio"
)

const coreExe = "connaio-core.exe"

// controlAddr is the child's named pipe, one per user: the core takes -ext-ctl-pipe on Windows.
func controlAddr(string) string {
	name := "connaio"
	if u, err := user.Current(); err == nil {
		name += "-" + strings.ReplaceAll(strings.ToLower(u.Username), `\`, "-")
	}
	return `\\.\pipe\` + name
}

func controlArgs(addr string) []string { return []string{"-ext-ctl-pipe", addr} }

// dialControl reaches the core's named pipe.
func dialControl(ctx context.Context, addr string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, addr)
}

// stopStale ends the core recorded in pidFile if it is still running.
func stopStale(pidFile string) {
	pid, ok := stalePid(pidFile)
	if !ok {
		return
	}
	defer os.Remove(pidFile)
	// only kill it if the pid still belongs to a connaio-core (pids get reused)
	out, err := run.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").Output()
	if err != nil || !strings.Contains(strings.ToLower(string(out)), `"`+coreExe+`"`) {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

// stop kills the child outright: Windows has no SIGTERM, and the child never runs TUN.
func stop(cmd *exec.Cmd, exited <-chan struct{}) {
	_ = cmd.Process.Kill()
	<-exited
}
