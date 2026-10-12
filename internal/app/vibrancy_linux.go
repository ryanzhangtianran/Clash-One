package app

import "github.com/wailsapp/wails/v3/pkg/application"

// The window's macOS touches (translucency, traffic lights, leaving the Dock) have no Linux
// counterpart: the window keeps the system's decorations.
func makeVibrant(*application.WebviewWindow) {}

func setDockVisible(bool) {}

// AppIcon and Symbol come from AppKit; on Linux the window falls back to its generic glyphs.
// ponytail: app icons could come from .desktop files and the icon theme.
func (s *SettingsService) AppIcon(string) string { return "" }

func (s *SettingsService) Symbol(string) string { return "" }
