package app

import (
	"cmp"
	"fmt"
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

const trayProcesses = 5

// trayMenu is the menu bar item and its menu, laid out after Surge's. The menu is rebuilt only when
// its layout (selectable groups, profiles) changes; checks, details and process rows are updated in
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
	layout       string // groups and profiles the menu was built for
	rows         []procRow
	modeItem     *application.MenuItem
	qualityItem  *application.MenuItem
	proxyItem    *application.MenuItem
	enhancedItem *application.MenuItem
	modeItems    map[string]*application.MenuItem
	partialItem  *application.MenuItem
	procItems    []*application.MenuItem
	details      map[*application.MenuItem]string // right of the label: grey text, or a pill (see pill)
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
	t.enhanced.onChange = func(on bool) { t.locked(func() { t.enhOn = on; t.checkEnhanced() }) }
	t.app.Event.On("setting", func(e *application.CustomEvent) {
		if e.Data == "capture.mode" {
			t.locked(t.checkEnhanced)
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
	t.details[t.modeItem] = ""
	if letter := modeBadges[t.mode]; letter != "" {
		t.details[t.modeItem] = pill("accent", letter)
	}
	showTray(t.tray, t.icon, t.traffic, t.mode, t.proxy || t.enhOn, !t.settings.Get().HideTraySpeed)
	if t.started {
		decorate(t.details, t.icons)
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

// checkEnhanced ticks enhanced mode or partial enhancement, whichever is on; both need root.
func (t *trayMenu) checkEnhanced() {
	mode, granted := t.enhanced.Mode(), t.enhanced.DaemonInstalled()
	t.enhancedItem.SetChecked(mode == "tun").SetEnabled(granted)
	t.partialItem.SetChecked(mode == "process-tun").SetEnabled(granted)
}

// toggleEnhanced turns mode on or, when it is the current one, TUN off.
func (t *trayMenu) toggleEnhanced(mode string) {
	if t.enhanced.Mode() == mode {
		mode = ""
	}
	if err := t.enhanced.SetMode(mode); err != nil {
		notify("error", "切换增强模式失败：%v", err)
	}
	t.locked(t.checkEnhanced) // also puts the checkbox back after a failure
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
	t.locked(func() { t.details[t.qualityItem] = pill("gray", "…") })
	q := qualityBadge(t.svc.Latency().Internet)
	t.locked(func() { t.details[t.qualityItem] = q })
}

// pill is a coloured pill for the right of a menu item: color is accent, good, warn, bad or gray.
func pill(color, text string) string { return "\x01" + color + "|" + text }

// qualityBadge colours a latency like the proxies page does: green, orange, then red.
func qualityBadge(ms int) string {
	switch {
	case ms <= 0:
		return pill("bad", "超时")
	case ms < 300:
		return pill("good", fmt.Sprintf("%d ms", ms))
	case ms < 800:
		return pill("warn", fmt.Sprintf("%d ms", ms))
	}
	return pill("bad", fmt.Sprintf("%d ms", ms))
}

func (t *trayMenu) tick() {
	all, _ := t.svc.Groups()
	var groups []Group
	for _, g := range all {
		if (g.Type == "Selector" || g.Type == "Subnet") && g.Name != "GLOBAL" {
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
		name, icon := clientOf(c)
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
	quality := t.details[t.qualityItem] // kept across the rebuild
	t.details, t.icons = map[*application.MenuItem]string{}, map[*application.MenuItem]string{}
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
			}).SetEnabled(g.Type == "Selector") // a subnet group follows the network on its own
		}
	}

	m.AddSeparator()
	t.qualityItem = m.Add("网络质量").OnClick(func(*application.Context) { t.measure() })
	t.details[t.qualityItem] = quality
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
	sub, _ = submenu("部分增强")
	t.partialItem = sub.AddCheckbox("开启", false).OnClick(func(*application.Context) { go t.toggleEnhanced("process-tun") })
	sub.Add("配置…").OnClick(func(*application.Context) {
		t.show()
		emit("navigate", "processes") // the window opens the 增强 page, where the apps are picked
	})
	t.enhancedItem = m.AddCheckbox("增强模式", false).SetAccelerator("CmdOrCtrl+E").OnClick(func(*application.Context) { go t.toggleEnhanced("tun") })
	t.checkEnhanced()
	m.Add("复制终端代理命令").SetAccelerator("CmdOrCtrl+C").OnClick(func(*application.Context) {
		port, err := mixedPort(t.sysProxy.core)
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
