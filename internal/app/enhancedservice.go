package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ryanzhangtianran/Clash-One/internal/core"
	"github.com/ryanzhangtianran/Clash-One/internal/daemon"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[bool]("enhanced")
}

// EnhancedService switches enhanced mode (TUN). It needs the root daemon, which Grant installs first.
type EnhancedService struct {
	core     *core.Core
	profiles *ProfileService
	settings *settingsFile
	onChange func(bool) // menu bar checkbox
}

func (s *EnhancedService) Enabled() bool { return s.settings.Get().Enhanced }

// Mode is how TUN is used: "" (off), "tun" (enhanced mode, all traffic) or "process-tun" (partial
// enhancement: only the selected apps and their child processes).
func (s *EnhancedService) Mode() string {
	if !s.Enabled() {
		return ""
	}
	if v, _ := s.profiles.Value("capture.mode"); v == "process-tun" {
		return "process-tun"
	}
	return "tun"
}

// SetMode switches between the two, or turns TUN off with "".
func (s *EnhancedService) SetMode(mode string) error {
	switch mode {
	case "":
		return s.SetEnabled(false)
	case "tun", "process-tun":
		if err := s.profiles.Set("capture.mode", mode); err != nil {
			return err
		}
		return s.SetEnabled(true)
	}
	return fmt.Errorf("unknown enhanced mode %q", mode)
}
func (s *EnhancedService) DaemonInstalled() bool { return daemon.Installed() }

// Grant installs the root daemon (one administrator prompt) and moves the GUI's core onto it;
// enhanced mode can be turned on only after this.
func (s *EnhancedService) Grant() error {
	bin, err := core.Binary()
	if err != nil {
		return err
	}
	if err := daemon.Install(bin); err != nil {
		return err
	}
	if err := s.useDaemon(); err != nil {
		return err
	}
	if err := s.profiles.apply(); err != nil {
		return err
	}
	s.changed(s.Enabled())
	return nil
}

func (s *EnhancedService) SetEnabled(on bool) error {
	if on && !s.core.Daemon() {
		if !daemon.Installed() {
			return errors.New("请先在「通用」中授予 root 权限")
		}
		if err := s.useDaemon(); err != nil {
			return err
		}
	}
	if err := s.settings.Update(func(st *Settings) { st.Enhanced = on }); err != nil {
		return err
	}
	if err := s.profiles.apply(); err != nil {
		_ = s.settings.Update(func(st *Settings) { st.Enhanced = !on })
		return err
	}
	s.changed(on)
	return nil
}

// useDaemon moves the GUI from its child core to the installed daemon.
func (s *EnhancedService) useDaemon() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = s.core.Stop() // frees the ports the daemon is about to take over
	if err := s.core.Attach(ctx, daemon.Socket); err != nil {
		return errors.Join(err, s.core.Start(ctx)) // keep a working core either way
	}
	return nil
}

// Uninstall turns enhanced mode off, removes the daemon and goes back to a child core.
func (s *EnhancedService) Uninstall() error {
	if s.Enabled() {
		if err := s.SetEnabled(false); err != nil {
			return err
		}
	}
	if err := daemon.Uninstall(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.core.Start(ctx); err != nil {
		return err
	}
	if err := s.profiles.apply(); err != nil {
		return err
	}
	s.changed(false) // the window and menu bar disable enhancing again
	return nil
}

func (s *EnhancedService) changed(on bool) {
	if s.onChange != nil {
		s.onChange(on)
	}
	emit("enhanced", on)
}
