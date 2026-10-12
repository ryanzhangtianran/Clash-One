package daemon

import "errors"

// Socket is where a root core would listen. ponytail: Linux has no root service yet, so enhanced mode
// is off there; a systemd unit installed through pkexec is the planned equivalent of the LaunchDaemon.
const Socket = "/var/lib/connaio/core.sock"

var errUnsupported = errors.New("Linux 版暂不支持增强模式（TUN）")

func Installed() bool { return false }

func Install(string) error { return errUnsupported }

func Uninstall() error { return nil }

func Stale(string) bool { return false }
