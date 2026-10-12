//go:build unix

package app

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// relaunch quits and runs cmd once this process is gone, so the single-instance check doesn't
// hand the new one back to the quitting one.
func relaunch(cmd []string) error {
	wait := `while kill -0 "$0" 2>/dev/null; do sleep 0.2; done; exec "$@"`
	c := exec.Command("sh", append([]string{"-c", wait, strconv.Itoa(os.Getpid())}, cmd...)...)
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // outlives this process
	if err := c.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond) // the window hears the call returned first
		application.Get().Quit()
	}()
	return nil
}
