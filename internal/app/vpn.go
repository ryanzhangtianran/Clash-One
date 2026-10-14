package app

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

// VPN is the VPN page's whole state in the override: the node, the rules that send traffic to it,
// and the subnet group that bypasses it on chosen networks. The page builds it; the store keeps it
// as ordinary user items, so the rules and proxies pages show the same things.
type VPN struct {
	Proxy map[string]any `json:"proxy"`
	Rules []string       `json:"rules"` // whole rule lines, in order
	Group map[string]any `json:"group"` // the subnet group, or nil when the VPN is always used
	// Active says the node is in the override (connected); false for the draft kept after 断开.
	Active bool `json:"active"`
}

// vpnTypes are the node types the VPN page manages.
var vpnTypes = []string{"easyconnect", "wireguard", "openvpn", "tailscale"}

// groupOf names the subnet group that stands in for the node when networks bypass it.
func groupOf(name string) string { return name + " 路由" }

// VPN reads the page's state back: the node in the override, else the draft kept from the last
// 断开, else nil.
func (s *ProfileService) VPN() (*VPN, error) {
	v, err := s.activeVPN()
	if v != nil || err != nil {
		return v, err
	}
	if s.settings != nil {
		return s.settings.Get().VPN, nil
	}
	return nil, nil
}

func (s *ProfileService) activeVPN() (*VPN, error) {
	items, err := s.store.UserItems()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(items.Proxies, func(p map[string]any) bool { t, _ := p["type"].(string); return slices.Contains(vpnTypes, t) })
	if i < 0 {
		return nil, nil
	}
	v := &VPN{Proxy: items.Proxies[i], Active: true}
	name, _ := v.Proxy["name"].(string)
	if j := slices.IndexFunc(items.Groups, func(g map[string]any) bool { return g["name"] == groupOf(name) }); j >= 0 {
		v.Group = items.Groups[j]
	}
	for _, r := range items.Rules {
		if t := ruleTarget(r); t == name || t == groupOf(name) {
			v.Rules = append(v.Rules, r)
		}
	}
	return v, nil
}

// SetVPN replaces the VPN's node, rules and group in one edit, so the core reloads once, and
// keeps a copy as the draft.
func (s *ProfileService) SetVPN(v VPN) error {
	name, _ := v.Proxy["name"].(string)
	if s.settings != nil {
		draft := v
		draft.Active = false
		_ = s.settings.Update(func(st *Settings) { st.VPN = &draft })
	}
	return s.edit(func() error {
		if err := s.clearVPN(); err != nil {
			return err
		}
		if err := s.store.AddProxy(v.Proxy, nil); err != nil {
			return err
		}
		if v.Group != nil {
			v.Group["name"] = groupOf(name)
			if err := s.store.AddGroup(v.Group); err != nil {
				return err
			}
		}
		for i := len(v.Rules) - 1; i >= 0; i-- { // prepending in reverse keeps the order
			if err := s.store.PrependRule(v.Rules[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveVPN disconnects: the node, rules and group leave the override; the draft stays.
func (s *ProfileService) RemoveVPN() error { return s.edit(s.clearVPN) }

// clearVPN removes the existing VPN node, its group and its rules, whatever the page had made.
func (s *ProfileService) clearVPN() error {
	old, err := s.activeVPN()
	if err != nil || old == nil {
		return err
	}
	name, _ := old.Proxy["name"].(string)
	for _, r := range old.Rules {
		if err := s.store.RemoveRule(r); err != nil {
			return err
		}
	}
	if old.Group != nil {
		if err := s.store.RemoveGroup(groupOf(name)); err != nil {
			return err
		}
	}
	return s.store.RemoveProxy(name)
}

// ruleTarget is the policy a rule line names: the third field, or the second for MATCH.
func ruleTarget(rule string) string {
	f := strings.Split(rule, ",")
	if len(f) >= 3 {
		return strings.TrimSpace(f[2])
	}
	if len(f) == 2 && strings.EqualFold(strings.TrimSpace(f[0]), "MATCH") {
		return strings.TrimSpace(f[1])
	}
	return ""
}

// Network is what the core sees of the current network, for subnet group conditions.
type Network struct {
	SSID   string `json:"ssid"`
	Router string `json:"router"`
}

func (s *CoreService) CurrentNetwork() (Network, error) {
	var n Network
	err := s.core.Do(context.Background(), http.MethodGet, "/network", nil, &n)
	return n, err
}
