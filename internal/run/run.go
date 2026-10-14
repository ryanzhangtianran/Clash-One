// Package run starts subprocesses without a console window on Windows, where every child of a
// windowsgui program would otherwise pop one up; elsewhere it is exec.Command.
package run

import (
	"context"
	"os/exec"
)

func Command(name string, args ...string) *exec.Cmd {
	return hide(exec.Command(name, args...))
}

func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return hide(exec.CommandContext(ctx, name, args...))
}
