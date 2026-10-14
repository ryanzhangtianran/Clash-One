//go:build !windows

package run

import "os/exec"

func hide(c *exec.Cmd) *exec.Cmd { return c }
