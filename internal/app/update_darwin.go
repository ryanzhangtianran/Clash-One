package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// updateAsset ends the name of this platform's release package.
const updateAsset = "-macos-" + runtime.GOARCH + ".dmg"

// installPackage copies the app out of the release's disk image over this app's bundle.
func installPackage(dmg string) error {
	if ownBundle == "" {
		return errors.New("只能更新从 .app 运行的 Connaio")
	}
	mount, err := os.MkdirTemp("", "connaio-dmg-")
	if err != nil {
		return err
	}
	defer os.Remove(mount)
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mount, dmg).CombinedOutput(); err != nil {
		return fmt.Errorf("打开安装包失败：%s", strings.TrimSpace(string(out)))
	}
	defer exec.Command("hdiutil", "detach", "-quiet", mount).Run()
	return replaceBundle(filepath.Join(mount, "Connaio.app"), ownBundle)
}

// replaceBundle copies src over the app bundle dst, swapping it in only once fully copied and
// putting the old one back if that fails.
func replaceBundle(src, dst string) error {
	staged, old := dst+".new", dst+".old"
	os.RemoveAll(staged)
	os.RemoveAll(old)
	if out, err := exec.Command("ditto", src, staged).CombinedOutput(); err != nil {
		os.RemoveAll(staged)
		return fmt.Errorf("复制新版本失败：%s", strings.TrimSpace(string(out)))
	}
	if err := os.Rename(dst, old); err != nil {
		os.RemoveAll(staged)
		return fmt.Errorf("无法替换 %s：%w", dst, err)
	}
	if err := os.Rename(staged, dst); err != nil {
		os.Rename(old, dst)
		return err
	}
	return os.RemoveAll(old)
}

func restartCommand() []string { return []string{"open", ownBundle} }
