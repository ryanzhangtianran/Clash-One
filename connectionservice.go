package main

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Conn struct {
	ID       string `json:"id"`
	Network  string `json:"network"`
	Inbound  string `json:"inbound"` // TUN, HTTP, FRP, ...
	Process  string `json:"process"`
	Path     string `json:"path"`
	Source   string `json:"source"`
	Host     string `json:"host"` // domain, or destination IP when there is none
	Port     string `json:"port"`
	Rule     string `json:"rule"` // e.g. "DomainSuffix(google.com)"
	Chain    string `json:"chain"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Start    string `json:"start"`
}

type TempRule struct {
	ID      int    `json:"id"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
	Hits    uint64 `json:"hits"`
}

// Connections lists live connections, newest first.
func (s *CoreService) Connections() ([]Conn, error) {
	var snap struct {
		Connections []struct {
			ID          string    `json:"id"`
			Upload      int64     `json:"upload"`
			Download    int64     `json:"download"`
			Start       time.Time `json:"start"`
			Chains      []string  `json:"chains"`
			Rule        string    `json:"rule"`
			RulePayload string    `json:"rulePayload"`
			Metadata    struct {
				Network     string `json:"network"`
				Type        string `json:"type"`
				SourceIP    string `json:"sourceIP"`
				Host        string `json:"host"`
				DestIP      string `json:"destinationIP"`
				DestPort    string `json:"destinationPort"`
				Process     string `json:"process"`
				ProcessPath string `json:"processPath"`
			} `json:"metadata"`
		} `json:"connections"`
	}
	if err := s.core.Do(context.Background(), http.MethodGet, "/connections", nil, &snap); err != nil {
		return nil, err
	}
	conns := make([]Conn, 0, len(snap.Connections))
	for _, c := range snap.Connections {
		m := c.Metadata
		host := m.Host
		if host == "" {
			host = m.DestIP
		}
		process := m.Process
		if process == "" && m.ProcessPath != "" {
			process = filepath.Base(m.ProcessPath)
		}
		rule := c.Rule
		if c.RulePayload != "" {
			rule += "(" + c.RulePayload + ")"
		}
		chain := slices.Clone(c.Chains)
		slices.Reverse(chain) // the core lists the exit first; show group → node
		conns = append(conns, Conn{
			ID: c.ID, Network: m.Network, Inbound: m.Type, Process: process, Path: m.ProcessPath, Source: m.SourceIP,
			Host: host, Port: m.DestPort, Rule: rule, Chain: strings.Join(chain, " → "),
			Upload: c.Upload, Download: c.Download, Start: c.Start.Format(time.RFC3339),
		})
	}
	slices.SortFunc(conns, func(a, b Conn) int { return strings.Compare(b.Start, a.Start) })
	return conns, nil
}

// CloseAll closes every live connection.
func (s *CoreService) CloseAll() error {
	return s.core.Do(context.Background(), http.MethodDelete, "/connections", nil, nil)
}

// Reload re-applies the current config (profile and override) to the core.
func (s *CoreService) Reload() error {
	if s.onReady == nil {
		return nil
	}
	return s.onReady()
}

func (s *CoreService) CloseConnection(id string) error {
	return s.core.Do(context.Background(), http.MethodDelete, "/connections/"+url.PathEscape(id), nil, nil)
}

// CloseHost closes every live connection to host, so they reconnect under a new rule.
func (s *CoreService) CloseHost(host string) error {
	conns, err := s.Connections()
	if err != nil {
		return err
	}
	for _, c := range conns {
		if c.Host == host || strings.HasSuffix(c.Host, "."+host) {
			_ = s.CloseConnection(c.ID)
		}
	}
	return nil
}

// CloseSource closes every live connection from a source IP, so a device picks up a new policy.
func (s *CoreService) CloseSource(ip string) error {
	conns, err := s.Connections()
	if err != nil {
		return err
	}
	for _, c := range conns {
		if c.Source == ip {
			_ = s.CloseConnection(c.ID)
		}
	}
	return nil
}

// AddTempRule adds a runtime rule like "DOMAIN-SUFFIX,google.com,DIRECT"; it lasts until the next reload.
func (s *CoreService) AddTempRule(rule string) (int, error) {
	var resp struct {
		ID int `json:"id"`
	}
	err := s.core.Do(context.Background(), http.MethodPost, "/rules/temporary", map[string]string{"rule": rule}, &resp)
	return resp.ID, err
}

func (s *CoreService) TempRules() ([]TempRule, error) {
	var resp struct {
		Rules []struct {
			Index   int    `json:"index"`
			Type    string `json:"type"`
			Payload string `json:"payload"`
			Proxy   string `json:"proxy"`
			Extra   *struct {
				HitCount uint64 `json:"hitCount"`
			} `json:"extra"`
		} `json:"rules"`
	}
	if err := s.core.Do(context.Background(), http.MethodGet, "/rules/temporary", nil, &resp); err != nil {
		return nil, err
	}
	rules := make([]TempRule, 0, len(resp.Rules))
	for _, r := range resp.Rules {
		t := TempRule{ID: r.Index, Type: r.Type, Payload: r.Payload, Proxy: r.Proxy}
		if r.Extra != nil {
			t.Hits = r.Extra.HitCount
		}
		rules = append(rules, t)
	}
	return rules, nil
}

func (s *CoreService) RemoveTempRule(id int) error {
	return s.core.Do(context.Background(), http.MethodDelete, "/rules/temporary/"+strconv.Itoa(id), nil, nil)
}

// Policies are the targets a rule can point at: the config's groups, DIRECT and REJECT.
func (s *CoreService) Policies() ([]string, error) {
	groups, err := s.Groups()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, g := range groups {
		if g.Name != "GLOBAL" {
			names = append(names, g.Name)
		}
	}
	return append(names, "DIRECT", "REJECT"), nil
}
