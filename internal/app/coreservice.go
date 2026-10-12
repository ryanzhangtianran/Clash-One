package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ryanzhangtianran/Connaio/internal/core"
	"github.com/ryanzhangtianran/Connaio/internal/daemon"
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
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Now     string            `json:"now"`
	All     []Proxy           `json:"all"`
	Default string            `json:"default,omitempty"` // Subnet: the member when no rule matches
	Rules   map[string]string `json:"rules,omitempty"`   // Subnet: "SSID:…", "ROUTER:…", CIDR, "TYPE:…" -> member
}

func init() {
	application.RegisterEvent[Traffic]("traffic")
	application.RegisterEvent[Status]("status")
	application.RegisterEvent[string]("mode")
}

// emit sends an event to the frontend; a no-op without a running app (tests).
func init() {
	application.RegisterEvent[Event]("event")
	application.RegisterEvent[string]("navigate") // page key the window should show, e.g. from the menu bar
}

// Event is something worth telling the user, listed under the topbar's events button.
type Event struct {
	Time  string `json:"time"`  // 2026/10/8 19:27:19
	Level string `json:"level"` // info, warning or error
	Text  string `json:"text"`
}

var eventLog struct {
	sync.Mutex
	list []Event // newest last, at most 50
}

// notify logs an event and adds it to the window's event list.
func notify(level, format string, args ...any) {
	e := Event{Time: time.Now().Format("2006/1/2 15:04:05"), Level: level, Text: fmt.Sprintf(format, args...)}
	eventLog.Lock()
	if n := len(eventLog.list); n > 0 && eventLog.list[n-1].Level == level && eventLog.list[n-1].Text == e.Text {
		eventLog.Unlock()
		return // the same thing again, e.g. a poll failing every few seconds while the core is down
	}
	log.Printf("%s: %s", level, e.Text)
	eventLog.list = append(eventLog.list[max(0, len(eventLog.list)-49):], e)
	eventLog.Unlock()
	emit("event", e)
}

// Notify lists something the window has to tell under the events button.
func (s *CoreService) Notify(level, text string) { notify(level, "%s", text) }

func (s *CoreService) Events() []Event {
	eventLog.Lock()
	defer eventLog.Unlock()
	return slices.Clone(eventLog.list)
}

func (s *CoreService) ClearEvents() {
	eventLog.Lock()
	eventLog.list = nil
	eventLog.Unlock()
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

// CoreService owns the connaio-core child process and exposes its API to the frontend.
type CoreService struct {
	core      *core.Core
	ledger    *ledger                                     // today's and this month's traffic; nil in tests
	onReady   func() error                                // pushes the GUI's config once a core answers
	onTraffic func(Traffic)                               // menu bar speed
	onMode    func(string)                                // menu bar mode radio
	onConns   func(conns []Conn, deltas map[string]int64) // menu bar process rows, from the ledger's count

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
			notify("error", "内核启动失败：%v", err)
			return
		}
		info, _ := s.core.Version(ctx)
		s.setStatus(Status{Running: true, Version: info.Version})
		notify("info", "内核已启动（%s）", info.Version)
		if info.API < core.API {
			notify("warning", "内核接口版本为 %d，App 需要 %d：部分功能不可用，请用新版 connaio-core 重新打包", info.API, core.API)
		}
		if w := s.portWarning(); w != "" {
			notify("warning", "%s", w)
		}
		if mode, err := s.Mode(); err == nil {
			s.modeChanged(mode)
		}
		go s.streamLogs(ctx)
		if s.ledger != nil {
			go s.countTraffic(ctx)
		}
		go s.updateDaemon()
		s.streamTraffic(ctx)
	}()
	return nil
}

// ServiceShutdown stops the child core, or puts the daemon back on its boot config so it doesn't
// keep capturing traffic (TUN) while the GUI is gone.
// portWarning reports listeners (e.g. mixed-port) the core could not open because another program,
// such as another proxy app, holds the port.
func (s *CoreService) portWarning() string {
	errs := s.core.BindErrors()
	if len(errs) == 0 {
		return ""
	}
	var ports []string
	for _, e := range errs {
		if m := bindPort.FindStringSubmatch(e); m != nil {
			ports = append(ports, m[1])
		}
	}
	if len(ports) == 0 {
		return "代理端口已被其他程序占用"
	}
	return "端口 " + strings.Join(ports, "、") + " 已被其他程序占用，未能监听"
}

var bindPort = regexp.MustCompile(`:(\d+): bind`)

func (s *CoreService) ServiceShutdown() error {
	if s.ledger != nil {
		if err := s.ledger.save(); err != nil {
			log.Printf("save traffic: %v", err)
		}
	}
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
			Type    string            `json:"type"`
			Now     string            `json:"now"`
			All     []string          `json:"all"`
			Default string            `json:"default"`
			Rules   map[string]string `json:"rules"`
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
		g := Group{Name: name, Type: p.Type, Now: p.Now, Default: p.Default, Rules: p.Rules}
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
// updateDaemon reinstalls the root daemon when an app update brought a new core: it keeps running
// the old copy until then. It runs once the window is up and the old core attached, since the
// administrator prompt holds the main thread; the heartbeat below re-pushes the config to the new one.
func (s *CoreService) updateDaemon() {
	bin, err := core.Binary()
	if err != nil || !s.core.Daemon() || !daemon.Stale(bin) {
		return
	}
	if err := daemon.Install(bin); err != nil {
		notify("warning", "root 内核未更新，仍在运行旧版：%v", err)
	}
}

// The stream doubles as the core's heartbeat: when it breaks the core went away (a crash, launchd
// restarting the root daemon, an update), and a core that comes back runs its boot config, so the
// GUI's config is pushed again once the stream is back.
func (s *CoreService) streamTraffic(ctx context.Context) {
	broken := false
	for ctx.Err() == nil {
		err := s.core.Stream(ctx, "/traffic", func(line []byte) {
			if broken {
				broken = false
				go func() {
					if s.onReady != nil {
						if err := s.onReady(); err != nil {
							notify("error", "内核已重新连接，但载入配置失败：%v", err)
							return
						}
					}
					notify("info", "内核已重新连接")
				}()
			}
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
			if !broken {
				broken = true
				notify("warning", "与内核的连接中断，正在重连")
			}
			log.Printf("traffic stream: %v, retrying", err)
			time.Sleep(time.Second)
		}
	}
}

// ReportError records a frontend error (render crash, unhandled rejection) in gui-errors.log.
func (s *CoreService) ReportError(message string) {
	f, err := os.OpenFile(filepath.Join(s.core.Dir, "gui-errors.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n\n", time.Now().Format(time.RFC3339), message)
}
