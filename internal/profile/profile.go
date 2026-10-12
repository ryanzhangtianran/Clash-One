// Package profile stores subscriptions and the user's override, and merges them into the core's config.
package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// UserAgent makes subscription services answer with Clash/mihomo YAML.
const UserAgent = "clash.meta/1.10.0 mihomo/1.10.0 Connaio/0.1"

type Usage struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"` // unix seconds, 0 = never
}

type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url,omitempty"` // empty for a local profile
	UpdatedAt time.Time `json:"updatedAt"`
	Interval  int       `json:"interval"` // auto-update hours, 0 = never
	Usage     *Usage    `json:"usage,omitempty"`
}

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

type Store struct {
	dir   string
	mu    sync.Mutex
	index struct {
		Current  string    `json:"current"`
		Profiles []Profile `json:"profiles"`
	}
}

// Open loads the store kept in dir/profiles.
func Open(dir string) (*Store, error) {
	s := &Store{dir: filepath.Join(dir, "profiles")}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.path("index.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, &s.index)
}

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path("index.json"), b, 0o644)
}

func (s *Store) List() (string, []Profile) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.index.Current, slices.Clone(s.index.Profiles)
}

// AddRemote downloads a subscription and stores it; the first profile becomes current.
func (s *Store) AddRemote(name, url string, clients ...*http.Client) (Profile, error) {
	content, usage, interval, err := download(url, clients)
	if err != nil {
		return Profile{}, err
	}
	if name == "" {
		name = "订阅"
	}
	return s.add(Profile{Name: name, URL: url, Usage: usage, Interval: interval}, content)
}

// AddLocal stores a Clash config the user provides.
func (s *Store) AddLocal(name string, content []byte) (Profile, error) {
	if err := validate(content); err != nil {
		return Profile{}, err
	}
	return s.add(Profile{Name: name}, content)
}

func (s *Store) add(p Profile, content []byte) (Profile, error) {
	id := make([]byte, 6)
	_, _ = rand.Read(id)
	p.ID, p.UpdatedAt = hex.EncodeToString(id), time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.WriteFile(s.path(p.ID+".yaml"), formatted(content), 0o600); err != nil {
		return Profile{}, err
	}
	s.index.Profiles = append(s.index.Profiles, p)
	if s.index.Current == "" {
		s.index.Current = p.ID
	}
	return p, s.save()
}

// Update re-downloads a remote profile.
func (s *Store) Update(id string, clients ...*http.Client) (Profile, error) {
	s.mu.Lock()
	i := slices.IndexFunc(s.index.Profiles, func(p Profile) bool { return p.ID == id })
	if i < 0 || s.index.Profiles[i].URL == "" {
		s.mu.Unlock()
		return Profile{}, fmt.Errorf("profile %s is not a subscription", id)
	}
	url := s.index.Profiles[i].URL
	s.mu.Unlock()

	content, usage, interval, err := download(url, clients)
	if err != nil {
		return Profile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if i = slices.IndexFunc(s.index.Profiles, func(p Profile) bool { return p.ID == id }); i < 0 {
		return Profile{}, fmt.Errorf("profile %s was removed", id)
	}
	if err := os.WriteFile(s.path(id+".yaml"), formatted(content), 0o600); err != nil {
		return Profile{}, err
	}
	p := &s.index.Profiles[i]
	p.UpdatedAt, p.Usage = time.Now(), usage
	if interval > 0 && p.Interval == 0 {
		p.Interval = interval
	}
	return *p, s.save()
}

func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index.Profiles = slices.DeleteFunc(s.index.Profiles, func(p Profile) bool { return p.ID == id })
	if s.index.Current == id {
		s.index.Current = ""
		if len(s.index.Profiles) > 0 {
			s.index.Current = s.index.Profiles[0].ID
		}
	}
	_ = os.Remove(s.path(id + ".yaml"))
	return s.save()
}

// Content is a profile's YAML as stored.
func (s *Store) Content(id string) (string, error) {
	if !s.has(id) {
		return "", fmt.Errorf("profile %s not found", id)
	}
	b, err := os.ReadFile(s.path(id + ".yaml"))
	return string(b), err
}

// SetContent replaces a profile's YAML after the same check a new local profile gets.
func (s *Store) SetContent(id, content string) error {
	if !s.has(id) {
		return fmt.Errorf("profile %s not found", id)
	}
	if err := validate([]byte(content)); err != nil {
		return err
	}
	formatted, err := Format(content)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(id+".yaml"), []byte(formatted), 0o600)
}

func (s *Store) has(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.index.Profiles, func(p Profile) bool { return p.ID == id })
}

func (s *Store) Select(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.ContainsFunc(s.index.Profiles, func(p Profile) bool { return p.ID == id }) {
		return fmt.Errorf("profile %s not found", id)
	}
	s.index.Current = id
	return s.save()
}

// Due lists remote profiles whose auto-update interval has passed.
func (s *Store) Due(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for _, p := range s.index.Profiles {
		if p.URL != "" && p.Interval > 0 && now.Sub(p.UpdatedAt) >= time.Duration(p.Interval)*time.Hour {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// Override is the user's YAML merged on top of the current profile.
func (s *Store) Override() (string, error) {
	b, err := os.ReadFile(s.path("override.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

func (s *Store) SetOverride(content string) error {
	if _, err := parseMap([]byte(content)); err != nil {
		return fmt.Errorf("覆写不是有效的 YAML 字典: %w", err)
	}
	return os.WriteFile(s.path("override.yaml"), []byte(content), 0o644)
}

// formatted is a profile as stored: Format'd, or as given if it can't be (validate already passed).
func formatted(content []byte) []byte {
	if f, err := Format(string(content)); err == nil {
		return []byte(f)
	}
	return content
}

// Format re-indents YAML to two spaces per level, keeping comments, order and quoting.
func Format(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return "", fmt.Errorf("不是有效的 YAML: %w", err)
	}
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", err
	}
	return b.String(), enc.Close()
}

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

func parseMap(b []byte) (map[string]any, error) {
	m := map[string]any{}
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// validate rejects content that isn't a Clash config, e.g. a base64 share-link list.
func validate(b []byte) error {
	m, err := parseMap(b)
	if err != nil {
		return fmt.Errorf("不是 Clash 配置（可能是 base64 节点列表）: %w", err)
	}
	if m["proxies"] == nil && m["proxy-providers"] == nil {
		return errors.New("配置中没有 proxies 或 proxy-providers")
	}
	return nil
}

// download fetches a subscription, trying each client in turn (e.g. direct, then through the core).
func download(url string, clients []*http.Client) ([]byte, *Usage, int, error) {
	if len(clients) == 0 {
		clients = []*http.Client{{Timeout: 30 * time.Second}}
	}
	var errs []error
	for _, c := range clients {
		content, usage, interval, err := fetch(c, url)
		if err == nil {
			return content, usage, interval, nil
		}
		errs = append(errs, err)
	}
	return nil, nil, 0, errors.Join(errs...)
}

func fetch(c *http.Client, url string) ([]byte, *Usage, int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, 0, fmt.Errorf("下载订阅: %s", resp.Status)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, nil, 0, err
	}
	if err := validate(content); err != nil {
		return nil, nil, 0, err
	}
	interval, _ := strconv.Atoi(resp.Header.Get("profile-update-interval"))
	return content, parseUsage(resp.Header.Get("subscription-userinfo")), interval, nil
}

// parseUsage reads "upload=1; download=2; total=3; expire=4".
func parseUsage(header string) *Usage {
	if header == "" {
		return nil
	}
	u := &Usage{}
	for _, field := range strings.Split(header, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(field), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseFloat(strings.TrimSpace(v), 64) // some services send floats
		switch strings.ToLower(k) {
		case "upload":
			u.Upload = int64(n)
		case "download":
			u.Download = int64(n)
		case "total":
			u.Total = int64(n)
		case "expire":
			u.Expire = int64(n)
		}
	}
	return u
}

// PrependRule puts rule first in the override's prepend-rules, keeping the rest of the override,
// comments included, as the user wrote it.
func (s *Store) PrependRule(rule string) error {
	return s.editOverride(func(root *yaml.Node) error {
		seq := child(root, "prepend-rules", yaml.SequenceNode)
		if seq.Kind != yaml.SequenceNode {
			return errors.New("覆写中的 prepend-rules 不是列表")
		}
		seq.Content = append([]*yaml.Node{{Kind: yaml.ScalarNode, Value: rule}}, seq.Content...)
		return nil
	})
}

func (s *Store) writeOverride(doc *yaml.Node) error {
	b, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return s.SetOverride(string(b))
}

// User items are what the GUI adds into the override: they survive subscription updates.

// UserItems lists the GUI-editable parts of the override.
type UserItems struct {
	Proxies []map[string]any `json:"proxies"` // prepend-proxies
	Groups  []map[string]any `json:"groups"`  // prepend-proxy-groups
	Rules   []string         `json:"rules"`   // prepend-rules
	Tunnels []map[string]any `json:"tunnels"` // reverse-tunnels
	Members map[string][]any `json:"members"` // group-members: group -> user proxies prepended into it
}

func (s *Store) UserItems() (UserItems, error) {
	text, err := s.Override()
	if err != nil {
		return UserItems{}, err
	}
	var raw struct {
		Proxies []map[string]any `yaml:"prepend-proxies"`
		Groups  []map[string]any `yaml:"prepend-proxy-groups"`
		Rules   []string         `yaml:"prepend-rules"`
		Tunnels []map[string]any `yaml:"reverse-tunnels"`
		Members map[string][]any `yaml:"group-members"`
	}
	if err := yaml.Unmarshal([]byte(text), &raw); err != nil {
		return UserItems{}, err
	}
	return UserItems(raw), nil
}

// EditProxy changes the named proxy by patch (only the edited fields; a null removes one). A
// GUI-added proxy is patched in place; one from the profile gets the patch stored under
// patch-proxies, which Merge lays over it, so the edit survives subscription updates while the
// fields left alone keep following the subscription.
func (s *Store) EditProxy(name string, patch map[string]any) error {
	return s.editOverride(func(root *yaml.Node) error {
		seq := listOf(root, "prepend-proxies")
		if i := findNamed(seq, name); i >= 0 {
			var proxy map[string]any
			if err := seq.Content[i].Decode(&proxy); err != nil {
				return err
			}
			applyPatch(proxy, patch)
			return seq.Content[i].Encode(proxy)
		}
		patches := mapOf(root, "patch-proxies")
		if patches.Kind != yaml.MappingNode {
			return errors.New("覆写中的 patch-proxies 不是字典")
		}
		n := child(patches, name, yaml.MappingNode)
		stored := map[string]any{}
		if err := n.Decode(&stored); err != nil {
			return err
		}
		return n.Encode(merge(stored, patch)) // nulls kept: they remove the profile's field at Merge
	})
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

// AddProxy adds a node and puts it at the front of each of groups.
func (s *Store) AddProxy(proxy map[string]any, groups []string) error {
	name, _ := proxy["name"].(string)
	if name == "" {
		return errors.New("代理名称不能为空")
	}
	return s.editOverride(func(root *yaml.Node) error {
		if findNamed(listOf(root, "prepend-proxies"), name) >= 0 {
			return fmt.Errorf("已存在名为 %s 的代理", name)
		}
		if err := appendItem(root, "prepend-proxies", proxy); err != nil {
			return err
		}
		members := mapOf(root, "group-members")
		for _, g := range groups {
			if err := appendItem(members, g, name); err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveProxy deletes a user node and its group memberships; a profile's node is listed under
// remove-proxies instead, which Merge applies.
func (s *Store) RemoveProxy(name string) error {
	return s.editOverride(func(root *yaml.Node) error {
		if findNamed(listOf(root, "prepend-proxies"), name) < 0 {
			return listName(root, "remove-proxies", name)
		}
		removeNamed(root, "prepend-proxies", name)
		if members := valueOf(root, "group-members"); members != nil && members.Kind == yaml.MappingNode {
			for i := 1; i < len(members.Content); i += 2 {
				seq := members.Content[i]
				seq.Content = slices.DeleteFunc(seq.Content, func(n *yaml.Node) bool { return n.Value == name })
			}
		}
		return nil
	})
}

func (s *Store) AddGroup(group map[string]any) error {
	name, _ := group["name"].(string)
	if name == "" {
		return errors.New("策略组名称不能为空")
	}
	return s.editOverride(func(root *yaml.Node) error {
		if findNamed(listOf(root, "prepend-proxy-groups"), name) >= 0 {
			return fmt.Errorf("已存在名为 %s 的策略组", name)
		}
		return appendItem(root, "prepend-proxy-groups", group)
	})
}

// RemoveGroup deletes a user group, or lists a profile's group under remove-proxy-groups.
func (s *Store) RemoveGroup(name string) error {
	return s.editOverride(func(root *yaml.Node) error {
		if findNamed(listOf(root, "prepend-proxy-groups"), name) < 0 {
			return listName(root, "remove-proxy-groups", name)
		}
		removeNamed(root, "prepend-proxy-groups", name)
		return nil
	})
}

// listName adds name to the override's string list key, once.
func listName(root *yaml.Node, key, name string) error {
	if seq := listOf(root, key); seq != nil && slices.ContainsFunc(seq.Content, func(n *yaml.Node) bool { return n.Value == name }) {
		return nil
	}
	return appendItem(root, key, name)
}

func (s *Store) AddTunnel(tunnel map[string]any) error {
	name, _ := tunnel["name"].(string)
	if name == "" {
		return errors.New("隧道名称不能为空")
	}
	return s.editOverride(func(root *yaml.Node) error {
		if findNamed(listOf(root, "reverse-tunnels"), name) >= 0 {
			return fmt.Errorf("已存在名为 %s 的隧道", name)
		}
		return appendItem(root, "reverse-tunnels", tunnel)
	})
}

func (s *Store) RemoveTunnel(name string) error {
	return s.editOverride(func(root *yaml.Node) error {
		removeNamed(root, "reverse-tunnels", name)
		return nil
	})
}

// MoveRule swaps the GUI-added rule at index (in prepend-rules) with the one delta places away.
// The profile's own rules keep their order, so subscription updates still reach them.
func (s *Store) MoveRule(index, delta int) error {
	return s.editOverride(func(root *yaml.Node) error {
		seq, j := listOf(root, "prepend-rules"), index+delta
		if seq == nil || min(index, j) < 0 || max(index, j) >= len(seq.Content) {
			return errors.New("只能调整自己添加的规则的顺序")
		}
		seq.Content[index], seq.Content[j] = seq.Content[j], seq.Content[index]
		return nil
	})
}

// RemoveRule drops a GUI-added rule from the override.
func (s *Store) RemoveRule(rule string) error {
	return s.editOverride(func(root *yaml.Node) error {
		if seq := listOf(root, "prepend-rules"); seq != nil {
			seq.Content = slices.DeleteFunc(seq.Content, func(n *yaml.Node) bool { return n.Value == rule })
		}
		return nil
	})
}

// HideRule drops one of the profile's rules by listing it under remove-rules, which Merge filters
// out; the rest of the profile's rules still follow the subscription.
func (s *Store) HideRule(rule string) error {
	return s.editOverride(func(root *yaml.Node) error { return appendItem(root, "remove-rules", rule) })
}

// editOverride applies change to the override's YAML tree, keeping comments and layout.
func (s *Store) editOverride(change func(root *yaml.Node) error) error {
	text, err := s.Override()
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return errors.New("覆写不是 YAML 字典")
	}
	if err := change(root); err != nil {
		return err
	}
	return s.writeOverride(&doc)
}

func valueOf(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// child returns m[key], creating it with kind when missing.
func child(m *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	if v := valueOf(m, key); v != nil {
		return v
	}
	v := &yaml.Node{Kind: kind}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
	return v
}

func listOf(m *yaml.Node, key string) *yaml.Node {
	if v := valueOf(m, key); v != nil && v.Kind == yaml.SequenceNode {
		return v
	}
	return nil
}

func mapOf(m *yaml.Node, key string) *yaml.Node { return child(m, key, yaml.MappingNode) }

func appendItem(m *yaml.Node, key string, item any) error {
	seq := child(m, key, yaml.SequenceNode)
	if seq.Kind != yaml.SequenceNode {
		return fmt.Errorf("覆写中的 %s 不是列表", key)
	}
	var n yaml.Node
	if err := n.Encode(item); err != nil {
		return err
	}
	seq.Content = append(seq.Content, &n)
	return nil
}

// findNamed returns the index of the mapping with name in seq, or -1.
func findNamed(seq *yaml.Node, name string) int {
	if seq == nil {
		return -1
	}
	return slices.IndexFunc(seq.Content, func(n *yaml.Node) bool {
		v := valueOf(n, "name")
		return n.Kind == yaml.MappingNode && v != nil && v.Value == name
	})
}

func removeNamed(m *yaml.Node, key, name string) {
	if seq := listOf(m, key); seq != nil {
		if i := findNamed(seq, name); i >= 0 {
			seq.Content = slices.Delete(seq.Content, i, i+1)
		}
	}
}

// SetPath sets a nested key such as dns.enhanced-mode in the override, creating the maps on the
// way and leaving sibling keys and comments alone.
func (s *Store) SetPath(path []string, value any) error {
	if len(path) == 0 {
		return errors.New("empty path")
	}
	return s.editOverride(func(root *yaml.Node) error {
		m := root
		for _, key := range path[:len(path)-1] {
			m = mapOf(m, key)
			if m.Kind != yaml.MappingNode {
				return fmt.Errorf("覆写中的 %s 不是字典", key)
			}
		}
		var n yaml.Node
		if err := n.Encode(value); err != nil {
			return err
		}
		last := path[len(path)-1]
		if v := valueOf(m, last); v != nil {
			*v = n
			return nil
		}
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: last}, &n)
		return nil
	})
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
