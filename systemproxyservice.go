package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ryanzhangtianran/Clash-One/internal/core"
	"github.com/ryanzhangtianran/Clash-One/internal/sysproxy"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() {
	application.RegisterEvent[bool]("sysproxy")
}

// SystemProxyService points the macOS system proxy at the core's mixed-port while enabled, and
// guards it: another app replacing the setting gets overridden within 5s.
type SystemProxyService struct {
	core     *core.Core
	settings *settingsFile
	onChange func(bool) // menu bar checkbox

	mu      sync.Mutex
	enabled bool // the user's choice, persisted
	port    int  // port currently set, 0 = not set
}

func (s *SystemProxyService) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

func (s *SystemProxyService) SetEnabled(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		port, err := s.mixedPort()
		if err != nil {
			return err
		}
		if err := sysproxy.Set(port); err != nil {
			return err
		}
		s.port = port
	} else if s.port != 0 {
		if err := sysproxy.Clear(); err != nil {
			return err
		}
		s.port = 0
	}
	s.enabled = on
	if err := s.settings.Update(func(st *Settings) { st.SystemProxy = on }); err != nil {
		return err
	}
	if s.onChange != nil {
		s.onChange(on)
	}
	emit("sysproxy", on)
	return nil
}

func (s *SystemProxyService) mixedPort() (int, error) {
	var cfg struct {
		MixedPort int `json:"mixed-port"`
	}
	if err := s.core.Do(context.Background(), http.MethodGet, "/configs", nil, &cfg); err != nil {
		return 0, err
	}
	if cfg.MixedPort == 0 {
		return 0, errors.New("配置中没有 mixed-port，无法设置系统代理")
	}
	return cfg.MixedPort, nil
}

func (s *SystemProxyService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.enabled = s.settings.Get().SystemProxy // applied by the guard once the core answers
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			s.guard()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return nil
}

// guard (re)applies the system proxy when enabled: at startup, after another app changed it,
// or when a new profile moved mixed-port.
func (s *SystemProxyService) guard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		return
	}
	port, err := s.mixedPort()
	if err != nil {
		return // core not up yet
	}
	if ok, err := sysproxy.IsSet(port); err == nil && ok {
		s.port = port
		return
	}
	if err := sysproxy.Set(port); err != nil {
		log.Printf("system proxy: %v", err)
		return
	}
	if s.port == port {
		log.Printf("system proxy was changed by another app, restored 127.0.0.1:%d", port)
	}
	s.port = port
	if s.onChange != nil {
		s.onChange(true)
	}
	emit("sysproxy", true)
}

// ServiceShutdown turns the proxy off so apps aren't left pointing at a closed port; the setting
// stays on and is re-applied next launch.
func (s *SystemProxyService) ServiceShutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.port == 0 {
		return nil
	}
	s.port = 0
	return sysproxy.Clear()
}
