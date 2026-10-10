package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryanzhangtianran/Clash-One/internal/core"
	"github.com/ryanzhangtianran/Clash-One/internal/profile"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type Profiles struct {
	Current  string            `json:"current"`
	Profiles []profile.Profile `json:"profiles"`
}

func init() {
	application.RegisterEvent[Profiles]("profiles")
}

// ProfileService manages subscriptions and the override, and feeds the merged config to the core.
type ProfileService struct {
	store    *profile.Store
	core     *core.Core
	settings *settingsFile
	mu       sync.Mutex // serializes rebuild + reload
}

func (s *ProfileService) build() ([]byte, error) {
	enhanced := s.settings != nil && s.settings.Get().Enhanced && s.core.Daemon()
	return s.store.Build(enhanced)
}

// WriteConfig builds config.yaml before the core starts; nothing is running yet to reload.
func (s *ProfileService) WriteConfig() error {
	b, err := s.build()
	if err != nil {
		return err
	}
	return os.WriteFile(s.core.ConfigPath(), b, 0o600)
}

// apply rebuilds the config, has the core binary validate it, then swaps it in and reloads.
// A rejected config never replaces the running one.
func (s *ProfileService) apply() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := s.build()
	if err != nil {
		return err
	}
	next := filepath.Join(s.core.Dir, "config.next.yaml")
	if err := os.WriteFile(next, b, 0o600); err != nil {
		return err
	}
	if err := s.core.Validate(next); err != nil {
		os.Remove(next)
		return err
	}
	if err := os.Rename(next, s.core.ConfigPath()); err != nil {
		return err
	}
	return s.core.Reload(context.Background(), b)
}

func (s *ProfileService) changed() {
	emit("profiles", s.Profiles())
}

func (s *ProfileService) Profiles() Profiles {
	current, list := s.store.List()
	return Profiles{Current: current, Profiles: list}
}

// clients try the subscription directly first, then through the core's mixed-port (blocked hosts).
func (s *ProfileService) clients() []*http.Client {
	direct := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 30 * time.Second}
	var cfg struct {
		MixedPort int `json:"mixed-port"`
	}
	if s.core.Do(context.Background(), http.MethodGet, "/configs", nil, &cfg) != nil || cfg.MixedPort == 0 {
		return []*http.Client{direct}
	}
	proxy := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(cfg.MixedPort)}
	return []*http.Client{direct, {Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}, Timeout: 30 * time.Second}}
}

// Add downloads a subscription; it is applied right away when it is the first profile.
func (s *ProfileService) Add(name, link string) error {
	p, err := s.store.AddRemote(name, link, s.clients()...)
	if err != nil {
		return err
	}
	defer s.changed()
	if current, _ := s.store.List(); current == p.ID {
		return s.apply()
	}
	return nil
}

// AddLocal stores pasted Clash YAML as a profile.
func (s *ProfileService) AddLocal(name, content string) error {
	p, err := s.store.AddLocal(name, []byte(content))
	if err != nil {
		return err
	}
	defer s.changed()
	if current, _ := s.store.List(); current == p.ID {
		return s.apply()
	}
	return nil
}

func (s *ProfileService) Update(id string) error {
	if _, err := s.store.Update(id, s.clients()...); err != nil {
		return err
	}
	defer s.changed()
	if current, _ := s.store.List(); current == id {
		return s.apply()
	}
	return nil
}

func (s *ProfileService) Remove(id string) error {
	wasCurrent, _ := s.store.List()
	if err := s.store.Remove(id); err != nil {
		return err
	}
	defer s.changed()
	if wasCurrent == id {
		return s.apply()
	}
	return nil
}

// Select switches profiles; on a rejected config the previous one stays selected and running.
func (s *ProfileService) Select(id string) error {
	previous, _ := s.store.List()
	if err := s.store.Select(id); err != nil {
		return err
	}
	defer s.changed()
	if err := s.apply(); err != nil {
		if previous != "" {
			_ = s.store.Select(previous)
		}
		return err
	}
	return nil
}

func (s *ProfileService) Override() (string, error) {
	return s.store.Override()
}

// SetOverride saves and applies the override, restoring the old one if the core rejects the result.
func (s *ProfileService) SetOverride(content string) error {
	old, err := s.store.Override()
	if err != nil {
		return err
	}
	if err := s.store.SetOverride(content); err != nil {
		return err
	}
	if err := s.apply(); err != nil {
		return errors.Join(err, s.store.SetOverride(old))
	}
	return nil
}

// edit runs a change to the override and applies it; if the core rejects the result the override
// is restored, so a bad form entry never breaks the running config.
func (s *ProfileService) edit(change func() error) error {
	old, err := s.store.Override()
	if err != nil {
		return err
	}
	if err := change(); err != nil {
		return err
	}
	if err := s.apply(); err != nil {
		return errors.Join(err, s.store.SetOverride(old))
	}
	return nil
}

// UserItems are the proxies, groups, rules and tunnels added from the GUI (kept in the override).
func (s *ProfileService) UserItems() (profile.UserItems, error) { return s.store.UserItems() }

// AddRule makes rule permanent by prepending it in the override.
func (s *ProfileService) AddRule(rule string) error {
	return s.edit(func() error { return s.store.PrependRule(rule) })
}

func (s *ProfileService) RemoveRule(rule string) error {
	return s.edit(func() error { return s.store.RemoveRule(rule) })
}

// AddProxy adds a node, also placing it at the front of the given groups. extra is YAML merged
// over the form's fields, for settings the form doesn't cover.
func (s *ProfileService) AddProxy(proxy map[string]any, extra string, groups []string) error {
	if err := profile.MergeYAML(proxy, extra); err != nil {
		return err
	}
	return s.edit(func() error { return s.store.AddProxy(proxy, groups) })
}

// AddTunnelYAML adds a reverse tunnel written as YAML (one reverse-tunnels entry).
func (s *ProfileService) AddTunnelYAML(text string) error {
	tunnel := map[string]any{}
	if err := profile.MergeYAML(tunnel, text); err != nil {
		return err
	}
	return s.AddTunnel(tunnel)
}

func (s *ProfileService) RemoveProxy(name string) error {
	return s.edit(func() error { return s.store.RemoveProxy(name) })
}

func (s *ProfileService) AddGroup(group map[string]any) error {
	return s.edit(func() error { return s.store.AddGroup(group) })
}

func (s *ProfileService) RemoveGroup(name string) error {
	return s.edit(func() error { return s.store.RemoveGroup(name) })
}

func (s *ProfileService) AddTunnel(tunnel map[string]any) error {
	return s.edit(func() error { return s.store.AddTunnel(tunnel) })
}

func (s *ProfileService) RemoveTunnel(name string) error {
	return s.edit(func() error { return s.store.RemoveTunnel(name) })
}

// SetKey sets a top-level override key (e.g. allow-lan) and applies it.
func (s *ProfileService) SetKey(key string, value any) error {
	return s.edit(func() error { return s.store.SetKey(key, value) })
}

// Set writes a dotted setting such as "dns.enhanced-mode" into the override and applies it.
func (s *ProfileService) Set(path string, value any) error {
	return s.edit(func() error { return s.store.SetPath(strings.Split(path, "."), value) })
}

// Value reads a dotted setting from the effective config (defaults, profile and override merged).
func (s *ProfileService) Value(path string) (any, error) {
	enhanced := s.settings != nil && s.settings.Get().Enhanced && s.core.Daemon()
	return s.store.Value(enhanced, strings.Split(path, "."))
}

func (s *ProfileService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				for _, id := range s.store.Due(now) {
					if err := s.Update(id); err != nil {
						log.Printf("auto-update profile %s: %v", id, err)
					}
				}
			}
		}
	}()
	return nil
}
