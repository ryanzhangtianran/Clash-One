package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestParseDesktopLinux(t *testing.T) {
	a := parseDesktop("[Desktop Entry]\nType=Application\nName=Firefox\nExec=env MOZ_X=1 /usr/lib/firefox/firefox %u\nIcon=firefox\n\n[Desktop Action new-window]\nName=New Window\nExec=firefox --new-window\n")
	if a != (desktopApp{name: "Firefox", exec: "firefox", icon: "firefox", cmd: "/usr/lib/firefox/firefox"}) {
		t.Fatalf("%+v", a)
	}
	if a := parseDesktop("[Desktop Entry]\nType=Application\nName=Hidden\nExec=x\nNoDisplay=true\n"); a.exec != "" {
		t.Fatalf("hidden entries are not apps: %+v", a)
	}
}

func TestAppOfLinux(t *testing.T) {
	apps := desktopApps()
	defer func() { desktopApps = func() []desktopApp { return apps } }()
	desktopApps = func() []desktopApp {
		return []desktopApp{{name: "Feishu", exec: "bytedance-feishu-stable", dir: "/opt/bytedance/feishu"}, {name: "Files", exec: "nautilus"}}
	}
	for exe, want := range map[string]string{"/opt/bytedance/feishu/feishu": "Feishu", "/usr/bin/nautilus": "Files", "nautilus": "Files", "/usr/bin/curl": ""} {
		if a, _ := appOf(exe); a.name != want {
			t.Errorf("appOf(%q) = %q, want %q", exe, a.name, want)
		}
	}
}

func TestLinuxProc(t *testing.T) {
	stat := "123 (my (app)) S 100 123 123 0 -1"
	if p := linuxProc(stat, "/opt/app/my (app)"); p.ppid != 100 || p.comm != "/opt/app/my (app)" {
		t.Fatalf("proc %+v", p)
	}
	if p := linuxProc(stat, ""); p.ppid != 100 || p.comm != "my (app)" {
		t.Fatalf("proc without exe %+v", p)
	}
}

func TestTrimPNGLinux(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 30; y < 60; y++ { // a 20×30 mark with a wide margin
		for x := 40; x < 60; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	out, err := png.Decode(bytes.NewReader(trimPNG(buf.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	if b := out.Bounds(); b.Dx() != 30 || b.Dy() != 30 {
		t.Fatalf("trimmed to %v, want 30×30", b)
	}
	if _, _, _, a := out.At(15, 15).RGBA(); a == 0 {
		t.Fatal("the mark isn't centred")
	}
}
