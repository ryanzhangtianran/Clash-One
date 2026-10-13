package app

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// updateAsset ends the name of this platform's release package: the NSIS installer.
const updateAsset = "-windows-" + runtime.GOARCH + ".exe"

// installPackage runs the installer silently; it raises the UAC prompt itself.
func installPackage(installer string) error {
	out, err := exec.Command(installer, "/S").CombinedOutput()
	if err != nil {
		return fmt.Errorf("安装更新失败：%s", cmp.Or(strings.TrimSpace(string(out)), err.Error()))
	}
	return nil
}

func restartCommand() []string {
	exe, _ := os.Executable() // the install dir's connaio.exe, now the new version
	return []string{exe}
}

// relaunch quits and runs cmd once this process is gone, so the single-instance check doesn't
// hand the new one back to the quitting one.
func relaunch(cmd []string) error {
	c := exec.Command("cmd", "/c", "timeout /t 1 /nobreak >nul & start \"\" \""+cmd[0]+"\"")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if err := c.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond) // the window hears the call returned first
		application.Get().Quit()
	}()
	return nil
}
