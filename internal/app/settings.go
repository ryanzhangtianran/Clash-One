package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings are the GUI's own switches, kept in settings.json next to the config.
type Settings struct {
	SystemProxy bool `json:"systemProxy"`
	Enhanced    bool `json:"enhanced"`
	// HideTraySpeed hides the upload/download speed next to the menu bar icon.
	HideTraySpeed bool `json:"hideTraySpeed"`
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
	return os.WriteFile(f.path, b, 0o644)
}
