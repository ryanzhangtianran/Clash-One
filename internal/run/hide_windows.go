package run

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func hide(c *exec.Cmd) *exec.Cmd {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return c
}
