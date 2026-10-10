// Package profile stores subscriptions and the user's override, and merges them into the core's config.
package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
const UserAgent = "clash.meta/1.10.0 mihomo/1.10.0 ClashOne/0.1"

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

// appDefaults are what Clash One needs; a profile and the override may change them.
var appDefaults = map[string]any{
	"mixed-port":        7899,
	"mode":              "rule",
	"log-level":         "info",
	"find-process-mode": "always", // the activity page ranks traffic by process
	"capture":           map[string]any{"mode": "system-proxy"},
}

// emptyProfile is used until the first profile is added. A fresh copy each time: Merge writes into
// the profile's maps.
func emptyProfile() map[string]any {
	return map[string]any{
		"proxy-groups": []any{map[string]any{"name": "代理", "type": "select", "proxies": []any{"DIRECT"}}},
		"rules":        []any{"MATCH,代理"},
	}
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
	if err := os.WriteFile(s.path(p.ID+".yaml"), content, 0o600); err != nil {
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
	if err := os.WriteFile(s.path(id+".yaml"), content, 0o600); err != nil {
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

// Build returns the config for the core: app defaults, then the current profile, then the override.
// enhanced is the GUI's enhanced-mode switch: TUN capture needs the root daemon.
func (s *Store) Build(enhanced bool) ([]byte, error) {
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
	return yaml.Marshal(cfg)
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
// and group-members (name -> proxies) puts proxies at the front of existing groups.
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
	return cfg
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
	text, err := s.Override()
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return err
	}
	if doc.Kind == 0 { // empty override
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return errors.New("覆写不是 YAML 字典")
	}
	item := &yaml.Node{Kind: yaml.ScalarNode, Value: rule}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "prepend-rules" && root.Content[i+1].Kind == yaml.SequenceNode {
			seq := root.Content[i+1]
			seq.Content = append([]*yaml.Node{item}, seq.Content...)
			return s.writeOverride(&doc)
		}
	}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "prepend-rules"},
		&yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{item}})
	return s.writeOverride(&doc)
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

// RemoveProxy deletes a user node and its group memberships.
func (s *Store) RemoveProxy(name string) error {
	return s.editOverride(func(root *yaml.Node) error {
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

func (s *Store) RemoveGroup(name string) error {
	return s.editOverride(func(root *yaml.Node) error {
		removeNamed(root, "prepend-proxy-groups", name)
		return nil
	})
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

func (s *Store) RemoveRule(rule string) error {
	return s.editOverride(func(root *yaml.Node) error {
		if seq := listOf(root, "prepend-rules"); seq != nil {
			seq.Content = slices.DeleteFunc(seq.Content, func(n *yaml.Node) bool { return n.Value == rule })
		}
		return nil
	})
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
