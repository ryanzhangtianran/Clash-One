//go:build !darwin

package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var trayDim *bool // whether the tray icon was last set faded; nil before the first call

// setTrayIcon sets the cat, pink and faded while nothing is captured, only when that changes:
// each icon goes over D-Bus on Linux.
func setTrayIcon(tray *application.SystemTray, icon []byte, capturing bool) {
	if trayDim != nil && *trayDim != capturing {
		return
	}
	dim := !capturing
	trayDim = &dim
	a := 1.0
	if dim {
		a = 0.4
	}
	tray.SetIcon(trayPNG(icon, a))
}

// trayPNG redraws the macOS template cat (black, 50×48, off-centre) for a Linux or Windows tray: pink, which
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

// detailSep sits between a menu item's label and its detail, and is how a label is told from
// what decorate appended to it last time.
const detailSep = " · "

// decorate folds each detail into its item's label ("Proxy · 香港", "网络质量 · 86 ms"), since
// Linux and Windows menus show plain text: a pill keeps its text, the mode badge becomes the mode's
// name, and icons are skipped.
func decorate(details, _ map[*application.MenuItem]string) {
	for item, detail := range details {
		base, _, _ := strings.Cut(item.Label(), detailSep)
		if text := detailText(detail); text != "" {
			base += detailSep + text
		}
		if base != item.Label() {
			item.SetLabel(base)
		}
	}
}

func detailText(detail string) string {
	if len(detail) > 1 && detail[0] == 1 { // a pill: "\x01color|text"
		_, detail, _ = strings.Cut(detail[1:], "|")
	}
	for key, letter := range modeBadges {
		if letter == detail {
			return modeLabel(key)
		}
	}
	return detail
}
