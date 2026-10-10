package main

import (
	"net"
	"strconv"
	"testing"
	"time"
)

func TestParseARP(t *testing.T) {
	m := parseARP(`? (192.168.1.1) at 9c:a6:15:aa:bb:cc on en0 ifscope [ethernet]
? (192.168.1.23) at a4:83:e7:1:2:3 on en0 ifscope [ethernet]
? (192.168.1.99) at (incomplete) on en0 ifscope [ethernet]
`)
	if m["192.168.1.23"] != "a4:83:e7:1:2:3" || m["192.168.1.1"] == "" || m["192.168.1.99"] != "" {
		t.Fatalf("arp: %v", m)
	}
}

func TestDeviceService(t *testing.T) {
	cs, ps, _ := startCore(t, "")
	d := &DeviceService{core: cs, profiles: ps}

	if err := d.SetAllowLan(true); err != nil {
		t.Fatal(err)
	}
	lan, err := d.Lan()
	if err != nil || !lan.AllowLan || lan.Port == 0 {
		t.Fatalf("lan %+v %v", lan, err)
	}
	if len(lan.Addresses) > 0 { // reachable on the LAN address only while allow-lan is on
		addr := net.JoinHostPort(lan.Addresses[0], strconv.Itoa(lan.Port))
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatalf("allow-lan on, %s unreachable: %v", addr, err)
		}
		c.Close()
		if err := d.SetAllowLan(false); err != nil {
			t.Fatal(err)
		}
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			c.Close()
			t.Fatalf("allow-lan off, %s still reachable", addr)
		}
		t.Logf("LAN access toggles on %s", addr)
	}

	pinned := func(ip string) Device {
		list, err := d.Devices()
		if err != nil {
			t.Fatal(err)
		}
		for _, dev := range list {
			if dev.IP == ip {
				return dev
			}
		}
		return Device{}
	}
	const ip = "192.168.77.23"
	if err := d.SetPolicy(ip, "REJECT", false); err != nil {
		t.Fatal(err)
	}
	if dev := pinned(ip); dev.Policy != "REJECT" || !dev.Temporary {
		t.Fatalf("temporary pin: %+v", dev)
	}
	if err := d.SetPolicy(ip, "DIRECT", true); err != nil { // replaces the temporary pin
		t.Fatal(err)
	}
	if dev := pinned(ip); dev.Policy != "DIRECT" || dev.Temporary || dev.Rule != "SRC-IP-CIDR,"+ip+"/32,DIRECT" {
		t.Fatalf("permanent pin: %+v", dev)
	}
	if rules, _ := cs.TempRules(); len(rules) != 0 {
		t.Fatalf("old temporary pin left: %+v", rules)
	}
	if err := d.ClearPolicy(ip); err != nil {
		t.Fatal(err)
	}
	if dev := pinned(ip); dev.IP != "" {
		t.Fatalf("cleared device still listed: %+v", dev)
	}
}
