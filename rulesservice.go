package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Rule struct {
	Index    int    `json:"index"`
	Type     string `json:"type"`
	Payload  string `json:"payload"`
	Proxy    string `json:"proxy"`
	Disabled bool   `json:"disabled"`
	Hits     uint64 `json:"hits"`
}

type Forward struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Remote string `json:"remote"`
	Local  string `json:"local"`
	State  string `json:"state"`
	Error  string `json:"error"`
}

type Tunnel struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	State    string    `json:"state"`
	Error    string    `json:"error"`
	Since    string    `json:"since"`
	Forwards []Forward `json:"forwards"`
}

type LogLine struct {
	Level   string `json:"type"`
	Payload string `json:"payload"`
}

func init() {
	application.RegisterEvent[LogLine]("log")
}

func (s *CoreService) Rules() ([]Rule, error) {
	var resp struct {
		Rules []struct {
			Index   int    `json:"index"`
			Type    string `json:"type"`
			Payload string `json:"payload"`
			Proxy   string `json:"proxy"`
			Extra   *struct {
				Disabled bool   `json:"disabled"`
				HitCount uint64 `json:"hitCount"`
			} `json:"extra"`
		} `json:"rules"`
	}
	if err := s.core.Do(context.Background(), http.MethodGet, "/rules", nil, &resp); err != nil {
		return nil, err
	}
	rules := make([]Rule, 0, len(resp.Rules))
	for _, r := range resp.Rules {
		rule := Rule{Index: r.Index, Type: r.Type, Payload: r.Payload, Proxy: r.Proxy}
		if r.Extra != nil {
			rule.Disabled, rule.Hits = r.Extra.Disabled, r.Extra.HitCount
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// SetRuleDisabled switches one configured rule off or on until the next reload.
func (s *CoreService) SetRuleDisabled(index int, disabled bool) error {
	return s.core.Do(context.Background(), http.MethodPatch, "/rules/disable", map[string]bool{strconv.Itoa(index): disabled}, nil)
}

func (s *CoreService) ReverseTunnels() ([]Tunnel, error) {
	var resp struct {
		Tunnels []Tunnel `json:"tunnels"`
	}
	err := s.core.Do(context.Background(), http.MethodGet, "/reverse-tunnels", nil, &resp)
	return resp.Tunnels, err
}

// streamLogs forwards core log lines (info and above) to the frontend, reconnecting on errors.
func (s *CoreService) streamLogs(ctx context.Context) {
	for ctx.Err() == nil {
		err := s.core.Stream(ctx, "/logs?level=info", func(line []byte) {
			var l LogLine
			if json.Unmarshal(line, &l) == nil {
				emit("log", l)
			}
		})
		if ctx.Err() == nil {
			log.Printf("log stream: %v, retrying", err)
			time.Sleep(time.Second)
		}
	}
}
