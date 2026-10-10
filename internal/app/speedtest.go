package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// speedURL is downloaded for bandwidth tests (a var so tests can point it at a local server).
var speedURL = "https://speed.cloudflare.com/__down?bytes=100000000"

const speedDuration = 10 * time.Second

type Speed struct {
	Bytes   int64   `json:"bytes"`
	Seconds float64 `json:"seconds"`
	Mbps    float64 `json:"mbps"`
}

var speedMu sync.Mutex // one test at a time: they share the temporary rule's host

// SpeedTest measures download bandwidth through proxy (a node or group): a temporary rule sends
// the test host to it, the file is fetched through the mixed-port for up to 10s, then the rule
// is removed. Needs rule mode, since global/direct mode skip rules.
func (s *CoreService) SpeedTest(proxy string) (Speed, error) {
	speedMu.Lock()
	defer speedMu.Unlock()
	ctx := context.Background()
	var cfg struct {
		Mode      string `json:"mode"`
		MixedPort int    `json:"mixed-port"`
	}
	if err := s.core.Do(ctx, http.MethodGet, "/configs", nil, &cfg); err != nil {
		return Speed{}, err
	}
	if cfg.Mode != "rule" {
		return Speed{}, errors.New("带宽测速需要「规则判定」模式")
	}
	u, err := url.Parse(speedURL)
	if err != nil {
		return Speed{}, err
	}
	rule := "DOMAIN," + u.Hostname() + "," + proxy
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		rule = "IP-CIDR," + u.Hostname() + "/32," + proxy + ",no-resolve"
	}
	id, err := s.AddTempRule(rule)
	if err != nil {
		return Speed{}, err
	}
	defer s.RemoveTempRule(id)

	via := &url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(cfg.MixedPort)}
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(via), DisableKeepAlives: true}}
	reqCtx, cancel := context.WithTimeout(ctx, speedDuration+5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, speedURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return Speed{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Speed{}, fmt.Errorf("测速服务器返回 %s", resp.Status)
	}

	start := time.Now() // from the first response byte: excludes connection setup
	buf := make([]byte, 64<<10)
	var total int64
	for time.Since(start) < speedDuration {
		n, err := resp.Body.Read(buf)
		total += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			if total == 0 {
				return Speed{}, err
			}
			break
		}
	}
	secs := time.Since(start).Seconds()
	return Speed{Bytes: total, Seconds: secs, Mbps: float64(total) * 8 / secs / 1e6}, nil
}
