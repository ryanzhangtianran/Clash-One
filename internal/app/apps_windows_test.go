package app

import "testing"

func TestWindowsProcs(t *testing.T) {
	out := "4 0 0 System\r\n100 4 1 WindowsTerminal.exe\r\n101 100 0 pwsh.exe\r\n102 101 0 claude.exe\r\n200 4 0 svchost.exe\r\n"
	table := windowsProcs(out)
	if p, _ := parentIn(table, "claude.exe"); p != "WindowsTerminal.exe" {
		t.Fatalf("claude's parent %q", p)
	}
	if p, _ := parentIn(table, "svchost.exe"); p != "" {
		t.Fatalf("svchost's parent %q", p)
	}
}
