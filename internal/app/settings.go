package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/ryanzhangtianran/Connaio/internal/profile"
)

// Settings are the GUI's own switches, kept in settings.json next to the config.
type Settings struct {
	SystemProxy bool `json:"systemProxy"`
	Enhanced    bool `json:"enhanced"`
	// HideTraySpeed hides the upload/download speed next to the menu bar icon.
	HideTraySpeed bool `json:"hideTraySpeed"`
	// VPN is the VPN page's last settings, kept while disconnected so 连接 brings them back.
	VPN *VPN `json:"vpn,omitempty"`
}

type settingsFile struct {
	path string
	mu   sync.Mutex
	s    Settings
}

func loadSettings(dir string) *settingsFile {
	f := &settingsFile{path: filepath.Join(dir, "settings.json")}
	if b, err := os.ReadFile(f.path); err == nil {
		_ = json.Unmarshal(b, &f.s)
	}
	return f
}

func (f *settingsFile) Get() Settings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.s
}

func (f *settingsFile) Update(change func(*Settings)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(&f.s)
	b, err := json.MarshalIndent(f.s, "", "  ")
	if err != nil {
		return err
	}
	return profile.WriteAtomic(f.path, b, 0o644)
}
