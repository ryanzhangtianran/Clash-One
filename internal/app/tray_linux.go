package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var trayDim *bool // whether the tray icon was last set faded; nil before the first call

// showTray paints the cat white, for Ubuntu's dark top bar where the macOS template (black) would
// vanish, faded while nothing is captured; only when that changes, as each icon goes over D-Bus.
// The speed goes in the item's title. ponytail: no mode badge as on macOS; the menu shows the mode.
func showTray(tray *application.SystemTray, icon []byte, t Traffic, _ string, capturing, showSpeed bool) {
	if trayDim == nil || *trayDim == capturing {
		dim := !capturing
		trayDim = &dim
		tray.SetIcon(whiteIcon(icon, map[bool]float64{false: 1, true: 0.4}[dim]))
	}
	if showSpeed {
		tray.SetLabel(fmt.Sprintf("↑%s ↓%s", rate(t.Up), rate(t.Down)))
	}
}

// whiteIcon repaints a template PNG white at opacity a, keeping its shape.
func whiteIcon(data []byte, a float64) []byte {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	b := img.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			out.SetNRGBA(x, y, color.NRGBA{255, 255, 255, uint8(float64(alpha>>8) * a)})
		}
	}
	var buf bytes.Buffer
	if png.Encode(&buf, out) != nil {
		return data
	}
	return buf.Bytes()
}

// decorate draws details and icons beside macOS menu items; Linux menus show plain items.
func decorate(_, _ map[*application.MenuItem]string) {}
