package app

import (
	"bytes"
	"image/png"
	"os"
	"testing"
)

func TestTrayPNGLinux(t *testing.T) {
	data, err := os.ReadFile("../../build/tray.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(trayPNG(data, 1)))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != b.Dy() {
		t.Fatalf("not square: %v", b)
	}
	// the cat is centred: its drawn columns leave equal margins (within a pixel)
	left, right := b.Dx(), 0
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				left, right = min(left, x), max(right, x)
			}
		}
	}
	if d := left - (b.Dx() - 1 - right); d < -1 || d > 1 {
		t.Fatalf("off-centre: margins %d and %d", left, b.Dx()-1-right)
	}
}
