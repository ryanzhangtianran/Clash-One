package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/ryanzhangtianran/Clash-One/internal/core"
	"github.com/ryanzhangtianran/Clash-One/internal/profile"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/tray.png
var trayIcon []byte

var modes = []struct{ key, label string }{
	{"rule", "规则判定"},
	{"global", "全局代理"},
	{"direct", "直接连接"},
}

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	dataDir := filepath.Join(configDir, "Clash One")
	store, err := profile.Open(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	settings := loadSettings(dataDir)
	svc := &CoreService{core: core.New(dataDir)}
	profiles := &ProfileService{store: store, core: svc.core, settings: settings}
	svc.onReady = profiles.apply
	sysProxy := &SystemProxyService{core: svc.core, settings: settings}
	enhanced := &EnhancedService{core: svc.core, profiles: profiles, settings: settings}
	devices := &DeviceService{core: svc, profiles: profiles}
	settingsSvc := &SettingsService{core: svc, settings: settings}
	if err := profiles.WriteConfig(); err != nil {
		log.Printf("build config: %v (keeping the previous config.yaml)", err)
	}

	var window *application.WebviewWindow
	app := application.New(application.Options{
		// a second copy would fight the first over the core's ports; bring the first forward instead
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.ryanzhangtianran.clashone",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if window != nil {
					window.Show()
					window.Focus()
				}
			},
		},
		Name:        "Clash One",
		Description: "Rule-based proxy with reverse tunnels",
		Services:    []application.Service{application.NewService(svc), application.NewService(profiles), application.NewService(sysProxy), application.NewService(enhanced), application.NewService(devices), application.NewService(settingsSvc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false, // lives on in the menu bar
		},
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Clash One",
		Width:          960,
		Height:         640,
		MinWidth:       720,
		MinHeight:      480,
		BackgroundType: application.BackgroundTypeTranslucent,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 52,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/",
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	var shown struct { // what the menu bar item displays; traffic and mode arrive on different goroutines
		sync.Mutex
		t               Traffic
		mode            string
		proxy, enhanced bool
	}
	shown.proxy, shown.enhanced = sysProxy.Enabled(), enhanced.Enabled()
	redraw := func(change func()) {
		shown.Lock()
		defer shown.Unlock()
		change()
		showTray(tray, shown.t, shown.mode, shown.proxy || shown.enhanced, !settings.Get().HideTraySpeed)
	}
	menu := application.NewMenu()
	items := map[string]*application.MenuItem{}
	for _, m := range modes {
		key := m.key
		items[key] = menu.AddRadio(m.label, key == "rule").OnClick(func(*application.Context) {
			if err := svc.SetMode(key); err != nil {
				log.Printf("set mode %s: %v", key, err)
			}
		})
	}
	menu.AddSeparator()
	proxyItem := menu.AddCheckbox("系统代理", false).OnClick(func(ctx *application.Context) {
		if err := sysProxy.SetEnabled(ctx.ClickedMenuItem().Checked()); err != nil {
			log.Printf("system proxy: %v", err)
		}
	})
	sysProxy.onChange = func(on bool) {
		proxyItem.SetChecked(on)
		menu.Update()
		redraw(func() { shown.proxy = on })
	}
	enhancedItem := menu.AddCheckbox("增强模式", settings.Get().Enhanced).OnClick(func(ctx *application.Context) {
		if err := enhanced.SetEnabled(ctx.ClickedMenuItem().Checked()); err != nil {
			log.Printf("enhanced mode: %v", err)
			enhanced.changed(enhanced.Enabled()) // put the checkbox back
		}
	})
	enhanced.onChange = func(on bool) {
		enhancedItem.SetChecked(on)
		menu.Update()
		redraw(func() { shown.enhanced = on })
	}
	menu.AddSeparator()
	menu.Add("打开 Clash One").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	menu.Add("退出").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	svc.onMode = func(mode string) {
		for key, item := range items {
			item.SetChecked(key == mode)
		}
		menu.Update()
		redraw(func() { shown.mode = mode })
	}
	svc.onTraffic = func(t Traffic) { redraw(func() { shown.t = t }) }

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// rate formats bytes per second for the menu bar, at most 9 characters: 0 KB/s, 512 KB/s, 3.4 MB/s.
func rate(b int64) string {
	switch mb := float64(b) / 1e6; {
	case b < 1e6:
		return fmt.Sprintf("%d KB/s", b/1000)
	case mb < 99.95:
		return fmt.Sprintf("%.1f MB/s", mb)
	default:
		return fmt.Sprintf("%.0f MB/s", mb)
	}
}
