package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ryanzhangtianran/Clash-One/internal/core"
	"github.com/ryanzhangtianran/Clash-One/internal/daemon"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type Traffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type Status struct {
	Running bool   `json:"running"`
	Version string `json:"version"`
	Error   string `json:"error"`
}

type Proxy struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Delay int    `json:"delay"` // ms of the last test, 0 if untested or failed
}

type Group struct {
	Name string  `json:"name"`
	Type string  `json:"type"`
	Now  string  `json:"now"`
	All  []Proxy `json:"all"`
}

func init() {
	application.RegisterEvent[Traffic]("traffic")
	application.RegisterEvent[Status]("status")
	application.RegisterEvent[string]("mode")
}

// emit sends an event to the frontend; a no-op without a running app (tests).
func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

// CoreService owns the clash-one-core child process and exposes its API to the frontend.
type CoreService struct {
	core      *core.Core
	onReady   func() error  // pushes the GUI's config once a core answers
	onTraffic func(Traffic) // menu bar speed
	onMode    func(string)  // menu bar mode radio

	mu     sync.Mutex
	status Status
}

func (s *CoreService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	go func() { // start in the background so the window shows right away
		var err error
		if daemon.Installed() {
			err = s.core.Attach(ctx, daemon.Socket)
		} else {
			err = s.core.Start(ctx)
		}
		if err == nil && s.onReady != nil {
			err = s.onReady() // a daemon runs its boot config until the GUI hands it the real one
		}
		if err != nil {
			s.setStatus(Status{Error: err.Error()})
			return
		}
		version, _ := s.core.Version(ctx)
		s.setStatus(Status{Running: true, Version: version})
		if mode, err := s.Mode(); err == nil {
			s.modeChanged(mode)
		}
		go s.streamLogs(ctx)
		s.streamTraffic(ctx)
	}()
	return nil
}

// ServiceShutdown stops the child core, or puts the daemon back on its boot config so it doesn't
// keep capturing traffic (TUN) while the GUI is gone.
func (s *CoreService) ServiceShutdown() error {
	if s.core.Daemon() {
		return s.core.Reload(context.Background(), []byte(daemon.BootConfig))
	}
	return s.core.Stop()
}

func (s *CoreService) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *CoreService) setStatus(st Status) {
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
	emit("status", st)
}

// Mode is the outbound mode: rule, global or direct.
func (s *CoreService) Mode() (string, error) {
	var cfg struct {
		Mode string `json:"mode"`
	}
	err := s.core.Do(context.Background(), http.MethodGet, "/configs", nil, &cfg)
	return cfg.Mode, err
}

func (s *CoreService) SetMode(mode string) error {
	if err := s.core.Do(context.Background(), http.MethodPatch, "/configs", map[string]string{"mode": mode}, nil); err != nil {
		return err
	}
	s.modeChanged(mode)
	return nil
}

func (s *CoreService) modeChanged(mode string) {
	if s.onMode != nil {
		s.onMode(mode)
	}
	emit("mode", mode)
}

// Groups lists proxy groups in config order (GLOBAL's member order), GLOBAL last.
func (s *CoreService) Groups() ([]Group, error) {
	var resp struct {
		Proxies map[string]struct {
			Type    string   `json:"type"`
			Now     string   `json:"now"`
			All     []string `json:"all"`
			History []struct {
				Delay int `json:"delay"`
			} `json:"history"`
		} `json:"proxies"`
	}
	if err := s.core.Do(context.Background(), http.MethodGet, "/proxies", nil, &resp); err != nil {
		return nil, err
	}
	toGroup := func(name string) (Group, bool) {
		p, ok := resp.Proxies[name]
		if !ok || len(p.All) == 0 {
			return Group{}, false
		}
		g := Group{Name: name, Type: p.Type, Now: p.Now}
		for _, member := range p.All {
			m := resp.Proxies[member]
			delay := 0
			if n := len(m.History); n > 0 {
				delay = m.History[n-1].Delay
			}
			g.All = append(g.All, Proxy{Name: member, Type: m.Type, Delay: delay})
		}
		return g, true
	}
	var groups []Group
	for _, name := range resp.Proxies["GLOBAL"].All {
		if g, ok := toGroup(name); ok {
			groups = append(groups, g)
		}
	}
	if g, ok := toGroup("GLOBAL"); ok {
		groups = append(groups, g)
	}
	return groups, nil
}

// ProxyNames lists every node and group a member list or dialer-proxy can name (GLOBAL excluded).
func (s *CoreService) ProxyNames() ([]string, error) {
	groups, err := s.Groups()
	if err != nil {
		return nil, err
	}
	var names []string
	seen := map[string]bool{"GLOBAL": true}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, g := range groups {
		add(g.Name)
		for _, p := range g.All {
			add(p.Name)
		}
	}
	return names, nil
}

// TestProxy measures one proxy and returns its delay in ms.
func (s *CoreService) TestProxy(name string) (int, error) {
	var d struct {
		Delay int `json:"delay"`
	}
	q := url.Values{"url": {"https://www.gstatic.com/generate_204"}, "timeout": {"5000"}}
	err := s.core.Do(context.Background(), http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay?"+q.Encode(), nil, &d)
	return d.Delay, err
}

func (s *CoreService) Select(group, proxy string) error {
	return s.core.Do(context.Background(), http.MethodPut, "/proxies/"+url.PathEscape(group), map[string]string{"name": proxy}, nil)
}

// TestGroup measures every member of group and returns name -> delay in ms (missing = failed).
func (s *CoreService) TestGroup(group string) (map[string]int, error) {
	q := url.Values{"url": {"https://www.gstatic.com/generate_204"}, "timeout": {"5000"}}
	delays := map[string]int{}
	err := s.core.Do(context.Background(), http.MethodGet, "/group/"+url.PathEscape(group)+"/delay?"+q.Encode(), nil, &delays)
	return delays, err
}

// streamTraffic forwards the core's per-second traffic to the frontend and menu bar, reconnecting on errors.
func (s *CoreService) streamTraffic(ctx context.Context) {
	for ctx.Err() == nil {
		err := s.core.Stream(ctx, "/traffic", func(line []byte) {
			var t Traffic
			if json.Unmarshal(line, &t) != nil {
				return
			}
			if s.onTraffic != nil {
				s.onTraffic(t)
			}
			emit("traffic", t)
		})
		if ctx.Err() == nil {
			log.Printf("traffic stream: %v, retrying", err)
			time.Sleep(time.Second)
		}
	}
}
