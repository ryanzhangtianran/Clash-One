package app

import (
	"cmp"
	"fmt"
	"maps"
	"runtime"
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

// trayRecent is how long a process stays listed after its last byte, so rows don't flicker.
const trayRecent = 10 * time.Second

// modeBadges are the letters for the outbound mode, on the menu bar icon and the mode item
var modeBadges = map[string]string{"rule": "R", "global": "G", "direct": "D"}

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
	started      bool        // the native menu exists; decorating before would block on the main thread
	updates      chan func() // one pending update from the data path (traffic, connections); a newer one replaces it
	drawMu       sync.Mutex  // one native redraw at a time, outside mu
	drawn        drawing     // what the native side shows, to skip redraws that change nothing
	traffic      Traffic
	mode         string
	proxy        bool
	enh          string // EnhancedService.Mode: "", "tun" or "process-tun"
	layout       string // groups and profiles the menu was built for
	rows         []procRow
	lastActive   map[string]time.Time // process -> when it last moved bytes
	modeItem     *application.MenuItem
	vpnItem      *application.MenuItem            // the VPN submenu: the connected ones as its detail
	vpnItems     map[string]*application.MenuItem // the VPN page's entries: state shown, click connects or disconnects
	proxyItem    *application.MenuItem
	enhancedItem *application.MenuItem
	modeItems    map[string]*application.MenuItem
	partialItem  *application.MenuItem
	partialMenu  *application.MenuItem // the 部分增强 submenu, which says when it is on
	qualityItem  *application.MenuItem
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

// drawing is everything the native side is told: the menu bar item's inputs and the decorations.
type drawing struct {
	traffic        Traffic
	mode           string
	state          string // what captures traffic: "", "proxy", "partial" (partial enhancement) or "tun" (enhanced mode)
	speed          bool
	details, icons map[*application.MenuItem]string
}

func newTrayMenu(t *trayMenu) {
	t.mode, t.proxy, t.enh = "rule", t.sysProxy.Enabled(), t.enhanced.Mode()
	t.lastActive = map[string]time.Time{}
	t.updates = make(chan func(), 1)
	t.menu = application.NewMenu()
	t.build(nil, Profiles{}, nil)
	t.tray.SetMenu(t.menu)
	t.svc.onMode = t.setMode
	// the data path (ledger count, traffic stream) must never wait on the main thread, which the
	// administrator prompt can hold: it drops an update off here and goes on
	t.svc.onConns = func(conns []Conn, deltas map[string]int64) {
		t.offer(func() { t.rows = processRows(conns, deltas, t.lastActive, time.Now()); t.showRows() })
	}
	t.svc.onTraffic = func(tr Traffic) { t.offer(func() { t.traffic = tr }) }
	go func() {
		for fn := range t.updates {
			t.locked(fn)
		}
	}()
	t.sysProxy.onChange = func(on bool) { t.locked(func() { t.proxy = on; t.setOn(t.proxyItem, on) }) }
	t.enhanced.onChange = func(bool) { t.locked(func() { t.enh = t.enhanced.Mode(); t.checkEnhanced() }) }
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

// offer queues an update for the menu bar, replacing one still waiting: only the latest matters.
func (t *trayMenu) offer(fn func()) {
	select {
	case <-t.updates:
	default:
	}
	select {
	case t.updates <- fn:
	default:
	}
}

// locked runs change under the lock, then redraws the menu bar item and the native decorations
// when something they show has changed; the native calls wait on the main thread, outside the lock.
func (t *trayMenu) locked(change func()) {
	t.mu.Lock()
	change()
	t.details[t.modeItem] = ""
	if letter := modeBadges[t.mode]; letter != "" {
		t.details[t.modeItem] = pill("accent", letter)
	}
	state := ""
	switch {
	case t.enh == "tun":
		state = "tun"
	case t.enh == "process-tun":
		state = "partial"
	case t.proxy:
		state = "proxy"
	}
	next := drawing{t.traffic, t.mode, state, !t.settings.Get().HideTraySpeed, maps.Clone(t.details), maps.Clone(t.icons)}
	started := t.started
	t.mu.Unlock()

	t.drawMu.Lock()
	defer t.drawMu.Unlock()
	if next.traffic != t.drawn.traffic || next.mode != t.drawn.mode || next.state != t.drawn.state || next.speed != t.drawn.speed {
		showTray(t.tray, t.icon, next.traffic, next.mode, next.state, next.speed)
	}
	if started && (!maps.Equal(next.details, t.drawn.details) || !maps.Equal(next.icons, t.drawn.icons)) {
		decorate(next.details, next.icons)
	}
	t.drawn = next
}

func (t *trayMenu) setMode(mode string) {
	t.locked(func() {
		t.mode = mode
		for key, item := range t.modeItems {
			check(item, key == mode)
		}
	})
}

// checkEnhanced ticks enhanced mode and partial enhancement as chosen; both need root.
func (t *trayMenu) checkEnhanced() {
	full, partial, granted := t.enhanced.Full(), t.enhanced.Partial(), t.enhanced.DaemonInstalled()
	t.setOn(t.enhancedItem, full)
	t.setOn(t.partialItem, partial)
	t.enhancedItem.SetEnabled(granted)
	t.partialItem.SetEnabled(granted)
	t.details[t.partialMenu] = ""
	switch {
	case partial && full:
		t.details[t.partialMenu] = "增强模式下暂停"
	case partial:
		t.details[t.partialMenu] = "已开启"
	}
}

// toggleEnhanced flips enhanced mode (full) or partial enhancement.
func (t *trayMenu) toggleEnhanced(full bool) {
	var err error
	if full {
		err = t.enhanced.SetFull(!t.enhanced.Full())
	} else {
		err = t.enhanced.SetPartial(!t.enhanced.Partial())
	}
	if err != nil {
		notify("error", "切换增强模式失败：%v", err)
	}
	t.locked(t.checkEnhanced) // also puts the checkbox back after a failure
}

// loop refreshes the groups and profiles every 5s (the process rows and speed come from the data
// path), and the network quality every minute.
// ponytail: polls /proxies even while the menu is closed; hook menuWillOpen if it shows up in a profile.
func (t *trayMenu) loop() {
	for i := 0; ; i += 5 {
		if i%60 == 10 { // the first one 10s in, once the startup apply has rebuilt the TUN under it
			go t.measure()
		}
		t.tick()
		time.Sleep(5 * time.Second)
	}
}

// measure shows the network quality. Its "…" goes up only the first time: on Linux every label
// change has the panel fetch the whole menu again, closing any submenu that is open.
func (t *trayMenu) measure() {
	t.locked(func() {
		if t.details[t.qualityItem] == "" {
			t.details[t.qualityItem] = dot("gray", "…")
		}
	})
	q := qualityBadge(t.svc.Latency().Internet)
	t.locked(func() { t.details[t.qualityItem] = q })
}

// qualityBadge colours a latency like the proxies page does: green, orange, then red.
func qualityBadge(ms int) string {
	switch {
	case ms <= 0:
		return dot("bad", "超时")
	case ms < 300:
		return dot("good", fmt.Sprintf("%d ms", ms))
	case ms < 800:
		return dot("warn", fmt.Sprintf("%d ms", ms))
	}
	return dot("bad", fmt.Sprintf("%d ms", ms))
}

// On Linux a list of choices starts every label with a tick or a blank as wide (tickOn, tickOff):
// the panel draws a radio's own mark before the label only once it is chosen, leaving no space for
// it on the rest, so the chosen label sat out of line. The blank is U+2800, which draws nothing,
// within half a pixel of ✓; a thin space follows either.
const (
	tickOn  = "✓\u2009"
	tickOff = "\u2800\u2009"
)

// addChoice adds one of a list of choices, ticked when on; on Linux a plain item (check).
func addChoice(m *application.Menu, label string, on bool) *application.MenuItem {
	if runtime.GOOS != "linux" {
		return m.AddRadio(label, on)
	}
	item := m.Add(tickOff + label)
	check(item, on)
	return item
}

// addSwitch adds an on/off item. On Linux its state is a detail after the label (setOn), so the
// main menu needs no column of ticks.
func (t *trayMenu) addSwitch(m *application.Menu, label string, on bool) *application.MenuItem {
	if runtime.GOOS != "linux" {
		return m.AddCheckbox(label, on)
	}
	item := m.Add(label)
	t.setOn(item, on)
	return item
}

// setOn shows a switch (addSwitch) on or off; called with the lock held.
func (t *trayMenu) setOn(item *application.MenuItem, on bool) {
	if runtime.GOOS != "linux" {
		item.SetChecked(on)
		return
	}
	t.details[item] = ""
	if on {
		t.details[item] = "已开启"
	}
}

// check ticks a choice (addChoice) or not; on Linux only a change touches the label, as each one has
// the panel fetch the whole menu again.
func check(item *application.MenuItem, on bool) {
	if runtime.GOOS != "linux" {
		item.SetChecked(on)
		return
	}
	label := tickOff + strings.TrimPrefix(strings.TrimPrefix(item.Label(), tickOn), tickOff)
	if on {
		label = tickOn + strings.TrimPrefix(label, tickOff)
	}
	if label != item.Label() {
		item.SetLabel(label)
	}
}

// modeLabel is the name of an outbound mode key ("rule" → 规则判定), or "".
func modeLabel(key string) string {
	for _, m := range modes {
		if m.key == key {
			return m.label
		}
	}
	return ""
}

// pill is a coloured pill for the right of a menu item: color is accent, good, warn, bad or gray.
func pill(color, text string) string { return "\x01" + color + "|" + text }

// dot is a detail drawn as a coloured dot before grey text: a state.
func dot(color, text string) string { return "\x02" + color + "|" + text }

// toggleVPN connects the named VPN's draft, or disconnects it when active.
func (t *trayMenu) toggleVPN(name string) {
	list, err := t.profiles.VPNs()
	if err != nil {
		return
	}
	i := slices.IndexFunc(list, func(v VPN) bool { return v.name() == name })
	if i < 0 {
		return
	}
	if list[i].Active {
		err = t.profiles.RemoveVPN(name)
	} else {
		err = t.profiles.SetVPN(list[i], name)
	}
	if err != nil {
		notify("error", "VPN：%v", err)
	}
	t.tick()
}

var vpnPills = map[string]string{"up": dot("good", "已连接"), "connecting": dot("warn", "连接中"), "down": dot("gray", "未连接")}

// vpnStates reads every VPN's state off the lock: name -> 未连接 / 连接中 / 已连接 as a pill.
func (t *trayMenu) vpnStates() map[string]string {
	list, err := t.profiles.VPNs()
	if err != nil {
		return nil
	}
	states := map[string]string{}
	for _, v := range list {
		states[v.name()] = vpnPills[v.State]
	}
	return states
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
	vpns := t.vpnStates()
	rebuilt := false
	t.locked(func() {
		if layout := menuLayout(groups, list, vpns); layout != t.layout {
			t.layout = layout
			t.build(groups, list, vpns)
			rebuilt = true
		}
		for _, g := range groups {
			t.details[t.groupItems[g.Name]] = g.Now
			for name, item := range t.memberItems[g.Name] {
				check(item, name == g.Now)
			}
		}
		for id, item := range t.profileItems {
			check(item, id == list.Current)
		}
		var up []string
		for name, item := range t.vpnItems {
			t.details[item] = vpns[name]
			if vpns[name] == vpnPills["up"] {
				up = append(up, name)
			}
		}
		slices.Sort(up)
		t.details[t.vpnItem] = cmp.Or(strings.Join(up, "、"), "未连接")
		t.showRows()
	})
	if rebuilt && runtime.GOOS == "linux" {
		// the tray hands its menu to the panel over D-Bus only when one is set: Update redraws GTK's
		// menus, not that copy, which kept the first layout, made before the core had any groups.
		// Outside the lock, as it waits on the main thread.
		t.tray.SetMenu(t.menu)
	}
}

func menuLayout(groups []Group, list Profiles, vpns map[string]string) string {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(vpns)) {
		b.WriteString("vpn:" + name + "\x00")
	}
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
func processRows(conns []Conn, deltas map[string]int64, lastActive map[string]time.Time, now time.Time) []procRow {
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
	for name, r := range byName {
		if r.rate > 0 {
			lastActive[name] = now
		}
		if now.Sub(lastActive[name]) <= trayRecent { // idle connections (kept alive, or opened before the proxy was turned off) stay out
			rows = append(rows, *r)
		}
	}
	for name, at := range lastActive { // forget processes gone quiet, or the map holds every one ever seen
		if now.Sub(at) > trayRecent {
			delete(lastActive, name)
		}
	}
	slices.SortFunc(rows, func(a, b procRow) int {
		return cmp.Or(cmp.Compare(b.rate, a.rate), cmp.Compare(b.total, a.total), strings.Compare(a.name, b.name))
	})
	return rows[:min(len(rows), trayProcesses)]
}

func (t *trayMenu) showRows() {
	for i, item := range t.procItems {
		switch {
		case i < len(t.rows):
			r := t.rows[i]
			item.SetLabel(r.name).SetEnabled(true).SetHidden(false)
			t.details[item], t.icons[item] = rate(max(0, r.rate)), r.icon
		case i == 0:
			item.SetLabel("暂无流量").SetEnabled(false).SetHidden(false)
			t.details[item], t.icons[item] = "", ""
		default:
			item.SetHidden(true)
		}
	}
}

// build lays the menu out afresh; called with the lock held, or before the app runs.
func (t *trayMenu) build(groups []Group, list Profiles, vpns map[string]string) {
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
	var headers []*application.MenuItem
	header := func(title string) {
		if runtime.GOOS != "darwin" { // elsewhere a title is a plain disabled item, read as a greyed-out option
			m.AddSeparator()
			return
		}
		headers = append(headers, m.Add(title).SetEnabled(false))
	}
	header("路由")
	sub, item := submenu("出站模式")
	t.modeItem, t.modeItems = item, map[string]*application.MenuItem{}
	for _, md := range modes {
		key := md.key
		t.modeItems[key] = addChoice(sub, md.label, key == t.mode).OnClick(func(*application.Context) {
			if err := t.svc.SetMode(key); err != nil {
				notify("error", "切换出站模式失败：%v", err)
				t.setMode(t.mode) // put the radio back
			}
		})
	}

	t.groupItems, t.memberItems = map[string]*application.MenuItem{}, map[string]map[string]*application.MenuItem{}
	for _, g := range groups {
		sub, item := submenu(g.Name)
		t.groupItems[g.Name], t.memberItems[g.Name] = item, map[string]*application.MenuItem{}
		for _, p := range g.All {
			group, name := g.Name, p.Name
			t.memberItems[group][name] = addChoice(sub, name, name == g.Now).OnClick(func(*application.Context) {
				if err := t.svc.Select(group, name); err != nil {
					notify("error", "选择 %s / %s 失败：%v", group, name, err)
				}
				t.tick()
			}).SetEnabled(g.Type == "Selector") // a subnet group follows the network on its own
		}
	}

	// the VPNs: a submenu of every one with its state (click connects or disconnects), and the way to add more
	sub, item = submenu("VPN")
	t.vpnItem, t.vpnItems = item, map[string]*application.MenuItem{}
	for _, name := range slices.Sorted(maps.Keys(vpns)) {
		t.vpnItems[name] = sub.Add(name).OnClick(func(*application.Context) { t.toggleVPN(name) })
	}
	if len(vpns) > 0 {
		sub.AddSeparator()
	}
	sub.Add("添加 VPN…").OnClick(func(*application.Context) {
		t.show()
		emit("navigate", "vpn")
	})
	header("网络")
	t.qualityItem = m.Add("网络质量").OnClick(func(*application.Context) { go t.measure() })
	t.details[t.qualityItem] = quality
	t.procItems = nil
	if runtime.GOOS == "darwin" {
		// each process's speed, live. Not on Linux, where every label change has the panel fetch the
		// whole menu again and close the submenu being read, nor on Windows, which shows plain text
		header("流量")
		for range trayProcesses {
			t.procItems = append(t.procItems, m.Add("").SetHidden(true).OnClick(func(*application.Context) { t.show() }))
		}
	}

	header("接管")
	t.proxyItem = t.addSwitch(m, "系统代理", t.proxy).SetAccelerator("CmdOrCtrl+S").OnClick(func(*application.Context) {
		if err := t.sysProxy.SetEnabled(!t.sysProxy.Enabled()); err != nil {
			notify("error", "设置系统代理失败：%v", err)
			t.sysProxy.onChange(t.sysProxy.Enabled()) // put the checkbox back
		}
	})
	sub, t.partialMenu = submenu("部分增强")
	t.partialItem = t.addSwitch(sub, "开启", false).OnClick(func(*application.Context) { go t.toggleEnhanced(false) })
	sub.Add("配置…").OnClick(func(*application.Context) {
		t.show()
		emit("navigate", "processes") // the window opens the 增强 page, where the apps are picked
	})
	t.enhancedItem = t.addSwitch(m, "增强模式", false).SetAccelerator("CmdOrCtrl+E").OnClick(func(*application.Context) { go t.toggleEnhanced(true) })
	t.checkEnhanced()

	t.profileItems = map[string]*application.MenuItem{}
	if len(list.Profiles) > 0 {
		m.AddSeparator()
		sub, _ := submenu("配置")
		for _, p := range list.Profiles {
			id := p.ID
			t.profileItems[id] = addChoice(sub, p.Name, id == list.Current).OnClick(func(*application.Context) {
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
	sectionHeaders(headers)
	t.showRows()
}
