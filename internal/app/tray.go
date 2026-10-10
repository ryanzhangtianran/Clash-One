package app

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var modes = []struct{ key, label string }{
	{"rule", "规则判定"},
	{"global", "全局代理"},
	{"direct", "直接连接"},
}

var scopes = []struct{ key, label string }{{"tun", "全部流量"}, {"process-tun", "仅指定进程"}}

const trayProcesses = 5

// trayMenu is the menu bar item and its menu, laid out after Surge's. The menu is rebuilt only when
// its layout (selectable groups, profiles) changes; checks, badges and process rows are updated in
// place, so an open menu stays live.
type trayMenu struct {
	app      *application.App
	tray     *application.SystemTray
	icon     []byte
	show     func()
	svc      *CoreService
	profiles *ProfileService
	sysProxy *SystemProxyService
	enhanced *EnhancedService
	settings *settingsFile
	menu     *application.Menu

	mu           sync.Mutex
	started      bool // the native menu exists; decorating before would block on the main thread
	traffic      Traffic
	mode         string
	proxy, enhOn bool
	quality      string
	layout       string // groups and profiles the menu was built for
	rows         []procRow
	modeItem     *application.MenuItem
	qualityItem  *application.MenuItem
	proxyItem    *application.MenuItem
	enhancedItem *application.MenuItem
	modeItems    map[string]*application.MenuItem
	scopeItems   map[string]*application.MenuItem
	procItems    []*application.MenuItem
	badges       map[*application.MenuItem]string // capsule on the right (mode, network quality), drawn natively
	details      map[*application.MenuItem]string // plain grey text on the right (selected node, speed)
	icons        map[*application.MenuItem]string // file whose Finder icon the item shows
	groupItems   map[string]*application.MenuItem
	memberItems  map[string]map[string]*application.MenuItem
	profileItems map[string]*application.MenuItem
}

type procRow struct {
	name, icon  string
	rate, total int64
}

func newTrayMenu(t *trayMenu) {
	t.mode, t.proxy, t.enhOn = "rule", t.sysProxy.Enabled(), t.enhanced.Enabled()
	t.menu = application.NewMenu()
	t.build(nil, Profiles{})
	t.tray.SetMenu(t.menu)
	t.svc.onMode = t.setMode
	t.svc.onConns = func(conns []Conn, deltas map[string]int64) {
		t.locked(func() { t.rows = processRows(conns, deltas); t.showRows() })
	}
	t.svc.onTraffic = func(tr Traffic) { t.locked(func() { t.traffic = tr }) }
	t.sysProxy.onChange = func(on bool) { t.locked(func() { t.proxy = on; t.proxyItem.SetChecked(on) }) }
	t.enhanced.onChange = func(on bool) { t.locked(func() { t.enhOn = on; t.enhancedItem.SetChecked(on) }) }
	t.app.Event.On("setting", func(e *application.CustomEvent) {
		if e.Data == "capture.mode" {
			t.locked(t.checkScope)
		}
	})
	t.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go func() { // off the main thread: decorate waits on it
			t.locked(func() { t.started = true })
			t.loop()
		}()
	})
}

// locked runs change under the lock, then redraws the menu bar item and the native decorations.
func (t *trayMenu) locked(change func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	change()
	t.badges[t.modeItem] = modeBadges[t.mode]
	showTray(t.tray, t.icon, t.traffic, t.mode, t.proxy || t.enhOn, !t.settings.Get().HideTraySpeed)
	if t.started {
		decorate(t.badges, t.details, t.icons)
	}
}

func (t *trayMenu) setMode(mode string) {
	t.locked(func() {
		t.mode = mode
		for key, item := range t.modeItems {
			item.SetChecked(key == mode)
		}
	})
}

// checkScope shows the enhanced-mode scope; capture.* reads as if enhanced, so it shows while off too.
func (t *trayMenu) checkScope() {
	v, _ := t.profiles.Value("capture.mode")
	for key, item := range t.scopeItems {
		item.SetChecked(key == v)
	}
}

// loop refreshes the menu every second, and the network quality every minute.
// ponytail: polls /proxies and /connections even while the menu is closed; hook menuWillOpen if it shows up in a profile.
func (t *trayMenu) loop() {
	for i := 0; ; i++ {
		if i%60 == 0 {
			go t.measure()
		}
		t.tick()
		time.Sleep(time.Second)
	}
}

func (t *trayMenu) measure() {
	t.locked(func() { t.quality = "…"; t.badges[t.qualityItem] = t.quality })
	l := t.svc.Latency()
	q := "超时"
	if l.Internet > 0 {
		q = fmt.Sprintf("%d ms", l.Internet)
	}
	t.locked(func() { t.quality = q; t.badges[t.qualityItem] = q })
}

func (t *trayMenu) tick() {
	all, _ := t.svc.Groups()
	var groups []Group
	for _, g := range all {
		if g.Type == "Selector" && g.Name != "GLOBAL" {
			groups = append(groups, g)
		}
	}
	list := t.profiles.Profiles()
	t.locked(func() {
		if layout := menuLayout(groups, list); layout != t.layout {
			t.layout = layout
			t.build(groups, list)
		}
		for _, g := range groups {
			t.details[t.groupItems[g.Name]] = g.Now
			for name, item := range t.memberItems[g.Name] {
				item.SetChecked(name == g.Now)
			}
		}
		for id, item := range t.profileItems {
			item.SetChecked(id == list.Current)
		}
		t.showRows()
	})
}

func menuLayout(groups []Group, list Profiles) string {
	var b strings.Builder
	for _, g := range groups {
		b.WriteString(g.Name + "\x00")
		for _, p := range g.All {
			b.WriteString(p.Name + "\x01")
		}
	}
	for _, p := range list.Profiles {
		b.WriteString(p.ID + "\x02" + p.Name + "\x00")
	}
	return b.String()
}

// processRows ranks the processes (apps folded into their bundle) by the bytes they moved since the
// last count.
func processRows(conns []Conn, deltas map[string]int64) []procRow {
	byName := map[string]*procRow{}
	for _, c := range conns {
		name, icon := c.Process, c.Path
		if i := strings.Index(c.Path, ".app/"); i >= 0 {
			icon = c.Path[:i+len(".app")]
			name = strings.TrimSuffix(filepath.Base(icon), ".app")
		}
		if name == "" {
			name = cmp.Or(c.Source, "-")
		}
		r := byName[name]
		if r == nil {
			r = &procRow{name: name, icon: icon}
			byName[name] = r
		}
		r.total += c.Upload + c.Download
		r.rate += deltas[c.ID]
	}
	rows := make([]procRow, 0, len(byName))
	for _, r := range byName {
		rows = append(rows, *r)
	}
	slices.SortFunc(rows, func(a, b procRow) int {
		return cmp.Or(cmp.Compare(b.rate, a.rate), cmp.Compare(b.total, a.total), strings.Compare(a.name, b.name))
	})
	return rows[:min(len(rows), trayProcesses)]
}

func (t *trayMenu) showRows() {
	for i, item := range t.procItems {
		if i >= len(t.rows) {
			item.SetHidden(true)
			continue
		}
		r := t.rows[i]
		item.SetLabel(r.name).SetHidden(false)
		t.details[item], t.icons[item] = rate(max(0, r.rate)), r.icon
	}
}

// build lays the menu out afresh; called with the lock held, or before the app runs.
func (t *trayMenu) build(groups []Group, list Profiles) {
	m := t.menu
	m.Clear()
	t.badges, t.details, t.icons = map[*application.MenuItem]string{}, map[*application.MenuItem]string{}, map[*application.MenuItem]string{}
	submenu := func(label string) (*application.Menu, *application.MenuItem) {
		sub := application.NewMenu()
		item := application.NewSubmenu(label, sub)
		m.Append(application.NewMenuFromItems(item))
		return sub, item
	}

	m.Add("显示主窗口").SetAccelerator("CmdOrCtrl+M").OnClick(func(*application.Context) { t.show() })
	m.AddSeparator()
	sub, item := submenu("出站模式")
	t.modeItem, t.modeItems = item, map[string]*application.MenuItem{}
	for _, md := range modes {
		key := md.key
		t.modeItems[key] = sub.AddRadio(md.label, key == t.mode).OnClick(func(*application.Context) {
			if err := t.svc.SetMode(key); err != nil {
				notify("error", "切换出站模式失败：%v", err)
				t.setMode(t.mode) // put the radio back
			}
		})
	}

	t.groupItems, t.memberItems = map[string]*application.MenuItem{}, map[string]map[string]*application.MenuItem{}
	if len(groups) > 0 {
		m.AddSeparator()
	}
	for _, g := range groups {
		sub, item := submenu(g.Name)
		t.groupItems[g.Name], t.memberItems[g.Name] = item, map[string]*application.MenuItem{}
		for _, p := range g.All {
			group, name := g.Name, p.Name
			t.memberItems[group][name] = sub.AddRadio(name, name == g.Now).OnClick(func(*application.Context) {
				if err := t.svc.Select(group, name); err != nil {
					notify("error", "选择 %s / %s 失败：%v", group, name, err)
				}
				t.tick()
			})
		}
	}

	m.AddSeparator()
	t.qualityItem = m.Add("网络质量").OnClick(func(*application.Context) { t.measure() })
	t.badges[t.qualityItem] = t.quality
	m.AddSeparator()
	m.Add("进程与客户端").SetEnabled(false)
	t.procItems = nil
	for range trayProcesses {
		t.procItems = append(t.procItems, m.Add("").SetHidden(true).OnClick(func(*application.Context) { t.show() }))
	}

	m.AddSeparator()
	t.proxyItem = m.AddCheckbox("设置为系统代理", t.proxy).SetAccelerator("CmdOrCtrl+S").OnClick(func(ctx *application.Context) {
		if err := t.sysProxy.SetEnabled(ctx.ClickedMenuItem().Checked()); err != nil {
			notify("error", "设置系统代理失败：%v", err)
			t.sysProxy.onChange(t.sysProxy.Enabled()) // put the checkbox back
		}
	})
	t.enhancedItem = m.AddCheckbox("增强模式", t.enhOn).SetAccelerator("CmdOrCtrl+E").OnClick(func(ctx *application.Context) {
		if err := t.enhanced.SetEnabled(ctx.ClickedMenuItem().Checked()); err != nil {
			notify("error", "切换增强模式失败：%v", err)
			t.enhanced.changed(t.enhanced.Enabled()) // put the checkbox back
		}
	})
	sub, _ = submenu("增强模式范围")
	t.scopeItems = map[string]*application.MenuItem{}
	for _, s := range scopes {
		key := s.key
		t.scopeItems[key] = sub.AddRadio(s.label, false).OnClick(func(*application.Context) {
			if err := t.profiles.Set("capture.mode", key); err != nil {
				notify("error", "切换增强模式范围失败：%v", err)
				t.locked(t.checkScope) // put the radio back
			}
		})
	}
	t.checkScope()
	m.Add("复制终端代理命令").SetAccelerator("CmdOrCtrl+C").OnClick(func(*application.Context) {
		port, err := t.sysProxy.mixedPort()
		if err != nil {
			notify("error", "复制终端代理命令失败：%v", err)
			return
		}
		t.app.Clipboard.SetText(fmt.Sprintf("export https_proxy=http://127.0.0.1:%[1]d http_proxy=http://127.0.0.1:%[1]d all_proxy=socks5://127.0.0.1:%[1]d", port))
	})

	t.profileItems = map[string]*application.MenuItem{}
	if len(list.Profiles) > 0 {
		m.AddSeparator()
		sub, _ := submenu("配置")
		for _, p := range list.Profiles {
			id := p.ID
			t.profileItems[id] = sub.AddRadio(p.Name, id == list.Current).OnClick(func(*application.Context) {
				if err := t.profiles.Select(id); err != nil {
					notify("error", "切换配置失败：%v", err)
				}
				t.tick()
			})
		}
	}
	m.AddSeparator()
	m.Add("退出").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { t.app.Quit() })
	m.Update()
	t.showRows()
}
