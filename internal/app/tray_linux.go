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

// showTray sets the cat, pink and faded while nothing is captured, only when that changes, as each
// icon goes over D-Bus.
// The speed goes in the item's title. ponytail: no mode badge as on macOS; the menu shows the mode.
func showTray(tray *application.SystemTray, icon []byte, t Traffic, _ string, capturing, showSpeed bool) {
	if trayDim == nil || *trayDim == capturing {
		dim := !capturing
		trayDim = &dim
		tray.SetIcon(trayPNG(icon, map[bool]float64{false: 1, true: 0.4}[dim]))
	}
	if showSpeed {
		tray.SetLabel(fmt.Sprintf("↑%s ↓%s", rate(t.Up), rate(t.Down)))
	}
}

// trayPNG redraws the macOS template cat (black, 50×48, off-centre) for a Linux tray: pink, which
// shows on Ubuntu's dark top bar where black would vanish, at opacity a, centred on a square canvas,
// since trays fit icons to a square.
func trayPNG(data []byte, a float64) []byte {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	var box image.Rectangle // the drawn part
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, alpha := img.At(x, y).RGBA(); alpha > 0 {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	side := max(box.Dx(), box.Dy()) + 4 // a 2px margin
	out := image.NewNRGBA(image.Rect(0, 0, side, side))
	dx, dy := (side-box.Dx())/2-box.Min.X, (side-box.Dy())/2-box.Min.Y
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			_, _, _, alpha := img.At(x, y).RGBA()
			out.SetNRGBA(x+dx, y+dy, color.NRGBA{0xff, 0x6e, 0xb4, uint8(float64(alpha>>8) * a)})
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
