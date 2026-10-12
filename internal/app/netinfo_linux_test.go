package app

import (
	"slices"
	"testing"
)

func TestParseRouteLinux(t *testing.T) {
	table := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"wlp2s0\t00000000\t0101A8C0\t0003\t0\t0\t600\t00000000\t0\t0\t0\n" +
		"enp3s0\t00000000\t0100000A\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"enp3s0\t0000000A\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	if r := parseRoute(table); r["interface"] != "enp3s0" || r["gateway"] != "10.0.0.1" {
		t.Fatalf("lowest-metric default route: %v", r)
	}
	v6 := "00000000000000000000000000000000 00 00000000000000000000000000000000 00 fe800000000000000000000000000001 00000400 00000001 00000000 00000003 wlp2s0\n"
	if r := parseRoute6(v6); r["interface"] != "wlp2s0" || r["gateway"] != "fe80::1" {
		t.Fatalf("v6 default route: %v", r)
	}
}

func TestParseResolvectlLinux(t *testing.T) {
	out := "Global:\nLink 2 (enp3s0): 10.0.0.1 fe80::1%2\nLink 3 (wlp2s0): 1.1.1.1#cloudflare-dns.com 10.0.0.1\n"
	if got := parseResolvectl(out); !slices.Equal(got, []string{"10.0.0.1", "fe80::1%2", "1.1.1.1"}) {
		t.Fatalf("%v", got)
	}
}

func TestParseProcARPLinux(t *testing.T) {
	table := "IP address       HW type     Flags       HW address            Mask     Device\n" +
		"192.168.1.1      0x1         0x2         9c:a6:15:aa:bb:cc     *        wlp2s0\n" +
		"192.168.1.9      0x1         0x0         00:00:00:00:00:00     *        wlp2s0\n"
	if m := parseProcARP(table); len(m) != 1 || m["192.168.1.1"] != "9c:a6:15:aa:bb:cc" {
		t.Fatalf("%v", m)
	}
}

func TestNMConnectionLinux(t *testing.T) {
	out := "有线连接 1:enp3s0\nlo:lo\nOffice\\: 5G:wlp2s0\n"
	if got := nmConnection(out, "enp3s0"); got != "有线连接 1" {
		t.Fatalf("%q", got)
	}
	if got := nmConnection(out, "wlp2s0"); got != "Office: 5G" {
		t.Fatalf("%q", got)
	}
}
