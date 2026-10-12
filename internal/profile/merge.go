package profile

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// appDefaults are what Connaio needs; a profile and the override may change them.
var appDefaults = map[string]any{
	"mixed-port":        7899,
	"mode":              "rule",
	"log-level":         "info",
	"find-process-mode": "always", // the activity page ranks traffic by process
	"capture":           map[string]any{"mode": "system-proxy"},
	"unified-delay":     true, // latency tests time a request on an open connection, like Surge, not the handshakes
}

// emptyProfile is used until the first profile is added. A fresh copy each time: Merge writes into
// the profile's maps.
func emptyProfile() map[string]any {
	return map[string]any{"rules": []any{"MATCH,DIRECT"}}
}

// controlKeys are dropped from profiles: the GUI reaches the core over its own Unix socket.
var controlKeys = []string{"external-controller", "external-controller-tls", "external-controller-unix",
	"external-controller-pipe", "external-controller-cors", "secret", "external-ui", "external-ui-url", "external-ui-name"}

// Build returns the config for the core: app defaults, then the current profile, then the override.
// enhanced is the GUI's enhanced-mode switch: TUN capture needs the root daemon.
func (s *Store) Build(enhanced bool) ([]byte, error) {
	cfg, err := s.merged(enhanced)
	if err != nil {
		return nil, err
	}
	// process-tun with no processes picked yet captures nothing, and the core refuses it: run without
	// TUN until some are picked. Value still reads process-tun, so the GUI keeps showing the choice.
	if capture, _ := cfg["capture"].(map[string]any); capture != nil && capture["mode"] == "process-tun" {
		if ps, _ := capture["processes"].([]any); len(ps) == 0 {
			capture["mode"] = "system-proxy"
		}
	}
	return yaml.Marshal(cfg)
}

// merged is the config as the user set it: defaults, profile and override, aligned with enhanced mode.
func (s *Store) merged(enhanced bool) (map[string]any, error) {
	s.mu.Lock()
	current := s.index.Current
	s.mu.Unlock()

	profile := emptyProfile()
	if current != "" {
		b, err := os.ReadFile(s.path(current + ".yaml"))
		if err != nil {
			return nil, err
		}
		if profile, err = parseMap(b); err != nil {
			return nil, fmt.Errorf("配置文件无效: %w", err)
		}
	}
	override := map[string]any{}
	if text, err := s.Override(); err != nil {
		return nil, err
	} else if text != "" {
		if override, err = parseMap([]byte(text)); err != nil {
			return nil, fmt.Errorf("覆写无效: %w", err)
		}
	}
	cfg := Merge(profile, override)
	Capture(cfg, enhanced)
	return cfg, nil
}

// tunDNS is used when TUN hijacks DNS and the profile brings no DNS section of its own.
var tunDNS = map[string]any{
	"enable":             true,
	"enhanced-mode":      "fake-ip",
	"default-nameserver": []any{"223.5.5.5", "119.29.29.29"},
	"nameserver":         []any{"https://223.5.5.5/dns-query", "https://1.12.12.12/dns-query"},
}

// Capture aligns capture.mode with enhanced mode. On: system-proxy (or none) becomes tun, an
// explicit process-tun is kept, and DNS is switched on for the hijack. Off: TUN modes fall back
// to system-proxy, since only the daemon can create a TUN.
func Capture(cfg map[string]any, enhanced bool) {
	capture, _ := cfg["capture"].(map[string]any)
	if capture == nil {
		capture = map[string]any{}
		cfg["capture"] = capture
	}
	mode, _ := capture["mode"].(string)
	if !enhanced {
		if mode == "tun" || mode == "process-tun" {
			capture["mode"] = "system-proxy"
		}
		return
	}
	if mode == "" || mode == "system-proxy" {
		capture["mode"] = "tun"
	}
	dns, _ := cfg["dns"].(map[string]any)
	if dns == nil || dns["enable"] != true {
		cfg["dns"] = merge(clone(tunDNS), dns)
		cfg["dns"].(map[string]any)["enable"] = true
	}
}

// Merge layers profile and override over the app defaults. Maps merge deeply, other values are
// replaced; prepend-/append-{rules,proxies,proxy-groups} in the override extend the profile's lists,
// group-members (name -> proxies) puts proxies at the front of existing groups, and
// remove-proxies / remove-proxy-groups drop the profile's items along with every reference to them.
func Merge(profile, override map[string]any) map[string]any {
	cfg := merge(clone(appDefaults), profile)
	for _, k := range controlKeys {
		delete(cfg, k)
	}
	extend := map[string]any{}
	for k, v := range override {
		if strings.HasPrefix(k, "prepend-") || strings.HasPrefix(k, "append-") {
			extend[k] = v
			delete(override, k)
		}
	}
	members, _ := override["group-members"].(map[string]any)
	delete(override, "group-members")
	patches, _ := override["patch-proxies"].(map[string]any) // name -> fields merged over that proxy
	delete(override, "patch-proxies")
	hidden, _ := override["remove-rules"].([]any) // profile rules deleted from the GUI
	delete(override, "remove-rules")
	removed := map[string]bool{}
	for _, key := range []string{"remove-proxies", "remove-proxy-groups"} {
		names, _ := override[key].([]any)
		for _, n := range names {
			if name, ok := n.(string); ok {
				removed[name] = true
			}
		}
		delete(override, key)
	}
	cfg = merge(cfg, override)
	for _, key := range []string{"rules", "proxies", "proxy-groups"} {
		list, _ := cfg[key].([]any)
		if pre, ok := extend["prepend-"+key].([]any); ok {
			list = append(slices.Clone(pre), list...)
		}
		if post, ok := extend["append-"+key].([]any); ok {
			list = append(list, post...)
		}
		if list != nil {
			cfg[key] = list
		}
	}
	if len(hidden) > 0 {
		rules, _ := cfg["rules"].([]any)
		cfg["rules"] = slices.DeleteFunc(slices.Clone(rules), func(r any) bool { return slices.Contains(hidden, r) })
	}
	// group-members: the GUI's own key, prepends user proxies into existing groups
	groups, _ := cfg["proxy-groups"].([]any)
	for _, g := range groups {
		group, _ := g.(map[string]any)
		name, _ := group["name"].(string)
		extra, _ := members[name].([]any)
		if len(extra) == 0 {
			continue
		}
		current, _ := group["proxies"].([]any)
		group["proxies"] = append(slices.Clone(extra), slices.DeleteFunc(slices.Clone(current), func(p any) bool { return slices.Contains(extra, p) })...)
	}
	proxies, _ := cfg["proxies"].([]any)
	for _, p := range proxies {
		proxy, _ := p.(map[string]any)
		name, _ := proxy["name"].(string)
		if patch, ok := patches[name].(map[string]any); ok {
			applyPatch(proxy, patch)
		}
	}
	if len(removed) > 0 {
		dropRemoved(cfg, removed)
	}
	return cfg
}

// dropRemoved deletes the named proxies and groups, takes them out of other groups (an emptied
// group falls back to DIRECT) and drops the rules that target them, so the core still accepts it.
func dropRemoved(cfg map[string]any, removed map[string]bool) {
	named := func(v any) bool {
		m, _ := v.(map[string]any)
		name, _ := m["name"].(string)
		return removed[name]
	}
	for _, key := range []string{"proxies", "proxy-groups"} {
		if list, ok := cfg[key].([]any); ok {
			cfg[key] = slices.DeleteFunc(slices.Clone(list), named)
		}
	}
	groups, _ := cfg["proxy-groups"].([]any)
	for _, g := range groups {
		group, _ := g.(map[string]any)
		if rules, ok := group["rules"].(map[string]any); ok { // subnet group
			maps.DeleteFunc(rules, func(_ string, v any) bool { s, _ := v.(string); return removed[s] })
		}
		if def, _ := group["default"].(string); removed[def] {
			group["default"] = "DIRECT"
		}
		list, ok := group["proxies"].([]any)
		if !ok {
			continue
		}
		list = slices.DeleteFunc(slices.Clone(list), func(p any) bool { s, _ := p.(string); return removed[s] })
		if len(list) == 0 && group["use"] == nil {
			list = []any{"DIRECT"}
		}
		group["proxies"] = list
	}
	if rules, ok := cfg["rules"].([]any); ok {
		cfg["rules"] = slices.DeleteFunc(slices.Clone(rules), func(r any) bool { s, _ := r.(string); return removed[ruleTarget(s)] })
	}
}

// ruleTarget is the policy a rule sends traffic to: its last field, past options like no-resolve.
func ruleTarget(rule string) string {
	fields := strings.Split(rule, ",")
	for len(fields) > 1 {
		last := strings.TrimSpace(fields[len(fields)-1])
		if last != "no-resolve" && last != "src" {
			return last
		}
		fields = fields[:len(fields)-1]
	}
	return ""
}

// MergeYAML deep-merges the YAML mapping in text (may be empty) into dst, e.g. a form's
// "more settings" box over the fields it filled in.
func MergeYAML(dst map[string]any, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	extra, err := parseMap([]byte(text))
	if err != nil {
		return fmt.Errorf("更多设置不是有效的 YAML 字典: %w", err)
	}
	merge(dst, extra)
	return nil
}

func merge(dst, src map[string]any) map[string]any {
	for k, v := range src {
		if sub, ok := v.(map[string]any); ok {
			if cur, ok := dst[k].(map[string]any); ok {
				dst[k] = merge(cur, sub)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

func clone(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			v = clone(sub)
		}
		out[k] = v
	}
	return out
}

// applyPatch merges patch into dst like merge, except that a null removes the key (and a map
// left empty by that goes too).
func applyPatch(dst, patch map[string]any) {
	for k, v := range patch {
		sub, ok := v.(map[string]any)
		switch {
		case v == nil:
			delete(dst, k)
		case ok:
			cur, isMap := dst[k].(map[string]any)
			if !isMap {
				cur = map[string]any{}
			}
			if applyPatch(cur, sub); len(cur) > 0 {
				dst[k] = cur
			} else {
				delete(dst, k)
			}
		default:
			dst[k] = v
		}
	}
}

// Value reads a dotted path from the merged config, e.g. "dns.nameserver".
func (s *Store) Value(enhanced bool, path []string) (any, error) {
	cfg, err := s.merged(enhanced)
	if err != nil {
		return nil, err
	}
	b, err := yaml.Marshal(cfg) // round-trip so values come back in the same shapes as the core's YAML
	if err != nil {
		return nil, err
	}
	var cur any = map[string]any{}
	if err := yaml.Unmarshal(b, &cur); err != nil {
		return nil, err
	}
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, nil
		}
		cur = m[key]
	}
	return cur, nil
}
