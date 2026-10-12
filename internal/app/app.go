package app

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/ryanzhangtianran/Connaio/internal/core"
	"github.com/ryanzhangtianran/Connaio/internal/profile"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Run wires the services, window and menu bar item, and blocks until the app quits.
func Run(assets fs.FS, trayIcon []byte) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dataDir := filepath.Join(configDir, "Connaio")
	store, err := profile.Open(dataDir)
	if err != nil {
		return err
	}
	settings := loadSettings(dataDir)
	svc := &CoreService{core: core.New(dataDir), ledger: openLedger(filepath.Join(dataDir, "traffic.json"))}
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
		// the window shows no errors: whatever a call from it fails with is listed under the events button
		MarshalError: func(err error) []byte { notify("error", "%v", err); return nil },
		// a second copy would fight the first over the core's ports; bring the first forward instead
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.ryanzhangtianran.connaio",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if window != nil {
					window.Show()
					window.Focus()
				}
			},
		},
		Name:        "Connaio",
		Description: "Rule-based proxy with reverse tunnels",
		Services:    []application.Service{application.NewService(svc), application.NewService(profiles), application.NewService(sysProxy), application.NewService(enhanced), application.NewService(devices), application.NewService(settingsSvc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false, // lives on in the menu bar
		},
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "Connaio",
		Width:          960,
		Height:         700,
		MinWidth:       960, // the activity and dashboard layouts fit completely from here
		MinHeight:      700,
		BackgroundType: application.BackgroundTypeTranslucent,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 52,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/",
	})
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { makeVibrant(window) })
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	newTrayMenu(&trayMenu{
		app: app, tray: tray, icon: trayIcon, show: func() { window.Show(); window.Focus() },
		svc: svc, profiles: profiles, sysProxy: sysProxy, enhanced: enhanced, settings: settings,
	})

	return app.Run()
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
