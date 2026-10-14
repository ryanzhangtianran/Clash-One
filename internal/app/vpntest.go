package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// VPNTest connects to target (host:port) through the core's proxy port, as any app would, and
// reports whether it got through and which rule and node the core sent it by. The core lists a
// connection only once its outbound dial succeeded, so that listing is the proof.
func (s *ToolService) VPNTest(target string) ([]string, error) {
	host, _, err := net.SplitHostPort(target)
	if err != nil || host == "" {
		return nil, errors.New("请填 主机:端口，如 10.1.2.3:22")
	}
	port, err := mixedPort(s.cs.core)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连不上内核代理端口 %d：%w", port, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return nil, fmt.Errorf("代理端口没有应答：%w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("内核拒绝：%s", resp.Status)
	}

	// The core answers before dialing out; a failed dial just closes our side.
	closed := make(chan struct{})
	go func() {
		_, _ = conn.Read(make([]byte, 1))
		close(closed)
	}()
	local := conn.LocalAddr().(*net.TCPAddr).Port
	for i := 0; i < 40; i++ {
		select {
		case <-closed:
			return nil, errors.New("连接失败：内核没能连上目标（看日志里的原因）")
		case <-time.After(250 * time.Millisecond):
		}
		if rule, chain, ok := s.cs.connectionBySourcePort(local); ok {
			return []string{
				fmt.Sprintf("连接 %s 成功：%d ms", target, time.Since(start).Milliseconds()),
				fmt.Sprintf("规则 %s，经 %s", rule, chain),
			}, nil
		}
	}
	return nil, errors.New("连接超时：目标没有响应")
}

// VPNProbe logs in to an EasyConnect gateway with settings that aren't saved yet and reports what
// it handed out; the core logs out again at once.
func (s *ToolService) VPNProbe(spec map[string]any) ([]string, error) {
	var resp struct {
		Address string   `json:"address"`
		Ranges  int      `json:"ranges"`
		Domains int      `json:"domains"`
		DNS     []string `json:"dns"`
		Message string   `json:"message"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	if err := s.cs.core.Do(ctx, http.MethodPost, "/easyconnect/test", spec, &resp); err != nil {
		return nil, fmt.Errorf("登录失败：%w", err)
	}
	return []string{
		fmt.Sprintf("登录成功，隧道地址 %s", resp.Address),
		fmt.Sprintf("网关路由 %d 个网段、%d 个域名，DNS %s", resp.Ranges, resp.Domains, strings.Join(resp.DNS, ", ")),
	}, nil
}

// connectionBySourcePort finds this process's live connection from the given local port.
func (s *CoreService) connectionBySourcePort(port int) (rule, chain string, ok bool) {
	conns, _ := s.fetchConnections()
	for _, c := range conns {
		if c.Source == "127.0.0.1" && c.SrcPort == strconv.Itoa(port) {
			return c.Rule, c.Chain, true
		}
	}
	return "", "", false
}
