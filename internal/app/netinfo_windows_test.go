package app

import "testing"

func TestWindowsParsers(t *testing.T) {
	route := parseRouteCSV("\"InterfaceAlias\",\"NextHop\"\r\n\"以太网\",\"192.168.1.1\"\r\n")
	if route["interface"] != "以太网" || route["gateway"] != "192.168.1.1" {
		t.Errorf("route %v", route)
	}
	if len(parseRouteCSV("")) != 0 {
		t.Error("empty route")
	}
	arp := parseWindowsARP("Interface: 192.168.1.5 --- 0x8\r\n  Internet Address      Physical Address      Type\r\n  192.168.1.1           aa-bb-cc-dd-ee-ff     dynamic\r\n  224.0.0.22            01-00-5e-00-00-16     static\r\n")
	if arp["192.168.1.1"] != "aa:bb:cc:dd:ee:ff" || len(arp) != 1 {
		t.Errorf("arp %v", arp)
	}
	apps := parseApps("chrome\r\nCode\r\n")
	if len(apps) != 2 || apps[0].Exec != "chrome.exe" || apps[1].Name != "Code" {
		t.Errorf("apps %v", apps)
	}
}
