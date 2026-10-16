package app

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestTrayPNGLinux(t *testing.T) {
	data, err := os.ReadFile("../../build/tray.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(trayPNG(data, trayColors[""])))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != b.Dy() {
		t.Fatalf("not square: %v", b)
	}
	// the tile is solid even with nothing captured, so the icon never fades into the panel
	if _, _, _, a := img.At(b.Dx()/2, 0).RGBA(); a != 0xffff {
		t.Fatalf("tile edge alpha %#x, want opaque", a)
	}
	// the white cat is centred: its drawn columns leave equal margins (within a pixel)
	left, right := b.Dx(), 0
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r>>8 > 0xe0 {
				left, right = min(left, x), max(right, x)
			}
		}
	}
	if d := left - (b.Dx() - 1 - right); d < -1 || d > 1 {
		t.Fatalf("off-centre: margins %d and %d", left, b.Dx()-1-right)
	}
}

func TestCheckLinux(t *testing.T) {
	item := application.NewMenuItem(tickOff + "GIA")
	check(item, true)
	if item.Label() != "✓\u2009GIA" {
		t.Fatalf("ticked: %q", item.Label())
	}
	check(item, false)
	if item.Label() != "\u2800\u2009GIA" {
		t.Fatalf("unticked: %q", item.Label())
	}
}
