package run

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func hide(c *exec.Cmd) *exec.Cmd {
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return c
}
