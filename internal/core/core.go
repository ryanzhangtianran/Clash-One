// Package core runs clash-one-core as a child process and talks to its REST API over a Unix socket.
package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Core is the clash-one-core the GUI talks to: its own child process, or an already running root
// daemon (enhanced mode). Dir is the GUI's data dir: config.yaml, core.log for the child.
type Core struct {
	Dir  string
	http *http.Client

	mu     sync.Mutex
	socket string
	daemon bool
	cmd    *exec.Cmd
	exited chan struct{}
}

func New(dir string) *Core {
	c := &Core{Dir: dir}
	c.http = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", c.Socket())
		},
		DisableKeepAlives: true, // the socket changes when switching between child and daemon
	}}
	return c
}

func (c *Core) Socket() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.socket
}

// Daemon reports whether the GUI is attached to the root daemon rather than its own child.
func (c *Core) Daemon() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.daemon
}

// Attach talks to a core that is already running on socket, e.g. the root daemon.
func (c *Core) Attach(ctx context.Context, socket string) error {
	c.mu.Lock()
	c.socket, c.daemon = socket, true
	c.mu.Unlock()
	return c.wait(ctx, nil, socket)
}

func (c *Core) wait(ctx context.Context, exited <-chan struct{}, what string) error {
	deadline := time.After(15 * time.Second)
	for {
		if _, err := c.Version(ctx); err == nil {
			return nil
		}
		select {
		case <-exited:
			return fmt.Errorf("clash-one-core exited, see %s", what)
		case <-deadline:
			return fmt.Errorf("clash-one-core at %s did not answer within 15s", c.Socket())
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Binary finds clash-one-core: $CLASH_ONE_CORE for development, else next to the app executable.
func Binary() (string, error) {
	if p := os.Getenv("CLASH_ONE_CORE"); p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	p := filepath.Join(filepath.Dir(exe), "clash-one-core")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("clash-one-core not found next to %s (set CLASH_ONE_CORE): %w", exe, err)
	}
	return p, nil
}

// Start launches the core and waits until its API answers.
func (c *Core) Start(ctx context.Context) error {
	bin, err := Binary()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(c.ConfigPath()); err != nil {
		return fmt.Errorf("no config to start with: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(c.Dir, "core.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	socket := filepath.Join(c.Dir, "core.sock") // keep short: macOS caps socket paths at 104 bytes
	_ = os.Remove(socket)                       // a stale socket from a crash would make the core fail to listen

	cmd := exec.Command(bin, "-d", c.Dir, "-ext-ctl-unix", socket)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	exited := make(chan struct{})
	c.mu.Lock()
	c.socket, c.daemon, c.cmd, c.exited = socket, false, cmd, exited
	c.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		logFile.Close()
		close(exited)
	}()
	return c.wait(ctx, exited, logFile.Name())
}

// ConfigPath is the config the core loads from its data dir.
func (c *Core) ConfigPath() string { return filepath.Join(c.Dir, "config.yaml") }

// Validate runs the core's config test on path, returning its message when the config is rejected.
func (c *Core) Validate(path string) error {
	bin, err := Binary()
	if err != nil {
		return err
	}
	out, err := exec.Command(bin, "-t", "-d", c.Dir, "-f", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("配置校验失败: %s", problems(string(out)))
	}
	return nil
}

// Reload hands the running core a config. It goes as a payload, not a path, so a root daemon
// never reads files inside the user's home.
func (c *Core) Reload(ctx context.Context, config []byte) error {
	return c.Do(ctx, http.MethodPut, "/configs?force=false", map[string]string{"payload": string(config)}, nil)
}

// problems keeps the core's error/fatal lines and its verdict from `-t` output.
func problems(out string) string {
	var keep []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, "level=error") || strings.Contains(line, "level=fatal") || strings.Contains(line, "failed") {
			if _, msg, ok := strings.Cut(line, `msg="`); ok {
				line = strings.TrimSuffix(msg, `"`)
			}
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 {
		return strings.TrimSpace(out)
	}
	return strings.Join(keep, "\n")
}

// Stop ends the GUI's own child (it restores routes and listeners on SIGTERM), killing it after 5s.
// An attached daemon keeps running.
func (c *Core) Stop() error {
	c.mu.Lock()
	cmd, exited := c.cmd, c.exited
	c.cmd = nil
	c.mu.Unlock()
	if cmd == nil {
		return nil
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-exited
	}
	return nil
}

func (c *Core) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	err := c.Do(ctx, http.MethodGet, "/version", nil, &v)
	return v.Version, err
}

// Do sends a JSON request; out may be nil. Non-2xx answers become errors carrying the core's message.
func (c *Core) Do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://core"+path, reader)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("%s %s: %s %s", method, path, resp.Status, e.Message)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Stream reads a streaming endpoint such as /traffic, calling fn per JSON line until ctx ends or the stream breaks.
func (c *Core) Stream(ctx context.Context, path string, fn func(line []byte)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://core"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fn(scanner.Bytes())
	}
	return scanner.Err()
}
