package app

import "testing"

func TestParseDesktopLinux(t *testing.T) {
	a := parseDesktop("[Desktop Entry]\nType=Application\nName=Firefox\nExec=env MOZ_X=1 /usr/lib/firefox/firefox %u\nIcon=firefox\n\n[Desktop Action new-window]\nName=New Window\nExec=firefox --new-window\n")
	if a != (desktopApp{name: "Firefox", exec: "firefox", icon: "firefox"}) {
		t.Fatalf("%+v", a)
	}
	if a := parseDesktop("[Desktop Entry]\nType=Application\nName=Hidden\nExec=x\nNoDisplay=true\n"); a.exec != "" {
		t.Fatalf("hidden entries are not apps: %+v", a)
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
