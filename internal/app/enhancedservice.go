package app

import (
	"context"
	"errors"
	"time"

	"github.com/ryanzhangtianran/Clash-One/internal/core"
	"github.com/ryanzhangtianran/Clash-One/internal/daemon"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[bool]("enhanced")
}

// EnhancedService switches enhanced mode (TUN). It needs the root daemon, installed on first use.
type EnhancedService struct {
	core     *core.Core
	profiles *ProfileService
	settings *settingsFile
	onChange func(bool) // menu bar checkbox
}

func (s *EnhancedService) Enabled() bool         { return s.settings.Get().Enhanced }
func (s *EnhancedService) DaemonInstalled() bool { return daemon.Installed() }

func (s *EnhancedService) SetEnabled(on bool) error {
	if on && !s.core.Daemon() {
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

// useDaemon installs the daemon if needed (one admin prompt) and moves the GUI from its child
// core to the daemon.
func (s *EnhancedService) useDaemon() error {
	if !daemon.Installed() {
		bin, err := core.Binary()
		if err != nil {
			return err
		}
		if err := daemon.Install(bin); err != nil {
			return err
		}
	}
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
	return s.profiles.apply()
}

func (s *EnhancedService) changed(on bool) {
	if s.onChange != nil {
		s.onChange(on)
	}
	emit("enhanced", on)
}
