// Package core runs connaio-core as a child process and talks to its REST API over a Unix socket.
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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Core is the connaio-core the GUI talks to: its own child process, or an already running root
// daemon (enhanced mode). Dir is the GUI's data dir: config.yaml, core.log for the child.
type Core struct {
	Dir  string
	http *http.Client

	mu      sync.Mutex
	socket  string
	logPath string
	daemon  bool
	cmd     *exec.Cmd
	exited  chan struct{}
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
	c.logPath = filepath.Join(filepath.Dir(socket), "core.log") // the daemon logs next to its socket
	c.mu.Unlock()
	return c.wait(ctx, nil, socket)
}

func (c *Core) wait(ctx context.Context, exited <-chan struct{}, logPath string) error {
	deadline := time.After(15 * time.Second)
	for {
		if _, err := c.Version(ctx); err == nil {
			return nil
		}
		select {
		case <-exited:
			out, _ := os.ReadFile(logPath)
			return fmt.Errorf("内核已退出: %s（日志 %s）", problems(string(out)), logPath)
		case <-deadline:
			return fmt.Errorf("connaio-core at %s did not answer within 15s", c.Socket())
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Binary finds connaio-core: $CONNAIO_CORE for development, else next to the app executable.
func Binary() (string, error) {
	if p := os.Getenv("CONNAIO_CORE"); p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	p := filepath.Join(filepath.Dir(exe), "connaio-core")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("connaio-core not found next to %s (set CONNAIO_CORE): %w", exe, err)
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
	pidFile := filepath.Join(c.Dir, "core.pid")
	stopStale(pidFile) // a core left by a crashed or force-quit GUI still holds the ports

	logPath := filepath.Join(c.Dir, "core.log")
	_ = os.Rename(logPath, filepath.Join(c.Dir, "core.prev.log")) // keep the last run's log for diagnosis
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
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
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
	exited := make(chan struct{})
	c.mu.Lock()
	c.socket, c.logPath, c.daemon, c.cmd, c.exited = socket, logPath, false, cmd, exited
	c.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		logFile.Close()
		_ = os.Remove(pidFile)
		close(exited)
	}()
	return c.wait(ctx, exited, logPath)
}

// stopStale ends the core recorded in pidFile if it is still running, waiting up to 5s.
func stopStale(pidFile string) {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		return
	}
	// only signal it if the pid still belongs to a connaio-core (pids get reused)
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil || filepath.Base(strings.TrimSpace(string(out))) != "connaio-core" {
		_ = os.Remove(pidFile)
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	_ = os.Remove(pidFile)
}

// BindErrors returns the listeners the core failed to open in its latest config load, from its
// log: a taken port doesn't stop the core, it just runs without that listener.
func (c *Core) BindErrors() []string {
	c.mu.Lock()
	path := c.logPath
	c.mu.Unlock()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := string(b)
	if i := strings.LastIndex(text, "Start initial configuration in progress"); i >= 0 {
		text = text[i:]
	}
	return msgs(text, func(line string) bool { return strings.Contains(line, "address already in use") })
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
// never reads files inside the user's home. force=true: without it the core skips inbound settings
// (ports, allow-lan); listeners whose address is unchanged are kept either way.
func (c *Core) Reload(ctx context.Context, config []byte) error {
	return c.Do(ctx, http.MethodPut, "/configs?force=true", map[string]string{"payload": string(config)}, nil)
}

// problems keeps the core's error/fatal lines and its verdict from `-t` output.
func problems(out string) string {
	keep := msgs(out, func(line string) bool {
		return strings.Contains(line, "level=error") || strings.Contains(line, "level=fatal") || strings.Contains(line, "failed")
	})
	if len(keep) == 0 {
		return strings.TrimSpace(out)
	}
	return strings.Join(keep, "\n")
}

// msgs returns the log lines matching keep, reduced to their msg="..." text.
func msgs(text string, keep func(string) bool) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if keep(line) {
			if _, msg, ok := strings.Cut(line, `msg="`); ok {
				line = strings.TrimSuffix(msg, `"`)
			}
			out = append(out, line)
		}
	}
	return out
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

// API is the connaio-core controller API level this GUI needs (its constant.ConnaioAPI).
const API = 1

// Info is the core's GET /version: the mihomo release it is based on, and its Connaio API level
// (0 for a core that predates the field).
type Info struct {
	Version string `json:"version"`
	API     int    `json:"connaio"`
}

func (c *Core) Version(ctx context.Context) (Info, error) {
	var v Info
	err := c.Do(ctx, http.MethodGet, "/version", nil, &v)
	return v, err
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
