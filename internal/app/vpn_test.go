package app

import (
	"reflect"
	"testing"
)

// The VPN page's state goes into the override as a node, rules and a subnet group, and comes back
// the same; saving again replaces all three, and removing leaves nothing behind.
func TestVPNRoundTrip(t *testing.T) {
	cs, s, _ := startCore(t, "prepend-rules:\n  - DOMAIN,mine.com,DIRECT\n")
	v := VPN{
		Proxy: map[string]any{"name": "VPN", "type": "easyconnect", "server": "vpn.example.edu.cn", "port": 443, "username": "alice", "password": "secret", "udp": true},
		Rules: []string{"EASYCONNECT,VPN,VPN 路由", "IP-CIDR,10.0.0.0/8,VPN 路由,no-resolve", "DOMAIN-SUFFIX,example.edu.cn,VPN 路由"},
		Group: map[string]any{"type": "subnet", "default": "VPN", "rules": map[string]any{"SSID:Campus": "DIRECT"}},
	}
	if err := s.SetVPN(v, ""); err != nil {
		t.Fatal(err)
	}
	list, err := s.VPNs()
	if err != nil || len(list) != 1 || !list[0].Active {
		t.Fatal(err, list)
	}
	got := &list[0]
	if !reflect.DeepEqual(got.Rules, v.Rules) || got.Group["name"] != "VPN 路由" || got.Proxy["server"] != "vpn.example.edu.cn" {
		t.Errorf("read back %+v", got)
	}
	items, _ := s.UserItems()
	if len(items.Rules) != 4 || items.Rules[3] != "DOMAIN,mine.com,DIRECT" {
		t.Errorf("rules %v", items.Rules)
	}
	if rules, _ := cs.Rules(); len(rules) < 4 {
		t.Errorf("core has %d rules", len(rules))
	}

	v.Group = nil
	v.Rules = []string{"EASYCONNECT,VPN,VPN"}
	if err := s.SetVPN(v, "VPN"); err != nil {
		t.Fatal(err)
	}
	list, _ = s.VPNs()
	got = &list[0]
	if got.Group != nil || !reflect.DeepEqual(got.Rules, v.Rules) {
		t.Errorf("after resave %+v", got)
	}
	items, _ = s.UserItems()
	if len(items.Groups) != 0 || len(items.Proxies) != 1 {
		t.Errorf("leftovers: groups %v proxies %v", items.Groups, items.Proxies)
	}

	if err := s.RemoveVPN("VPN"); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.VPNs(); len(list) != 0 {
		t.Errorf("still there: %+v", list)
	}
	items, _ = s.UserItems()
	if len(items.Rules) != 1 || len(items.Proxies) != 0 {
		t.Errorf("leftovers: %v %v", items.Rules, items.Proxies)
	}
}

func TestRuleTarget(t *testing.T) {
	for rule, want := range map[string]string{"EASYCONNECT,VPN,VPN 路由": "VPN 路由", "IP-CIDR,10.0.0.0/8,VPN,no-resolve": "VPN", "MATCH,DIRECT": "DIRECT", "DOMAIN": ""} {
		if got := ruleTarget(rule); got != want {
			t.Errorf("ruleTarget(%q) = %q, want %q", rule, got, want)
		}
	}
}
