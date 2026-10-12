package app

import (
	"bufio"
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/crypto/ssh"
)

func init() {
	application.RegisterEvent[string]("frps-deploy") // one line of the deploy's progress
}

// FrpsDeploy is where and how to log in to set frps up.
type FrpsDeploy struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
	Key      string `json:"key,omitempty"` // private key text
	BindPort int    `json:"bindPort"`      // frps listens here for frpc
}

// Frps is a deployed frps, for the tunnel form.
type Frps struct {
	Token    string `json:"token"`
	BindPort int    `json:"bindPort"`
}

// frpsScript installs the latest frps release for the server's architecture as a systemd service
// with token auth. %[1]d is the bind port, %[2]s the token. Needs Linux, systemd and curl; root,
// or sudo without a password.
const frpsScript = `set -e
SUDO=""; [ "$(id -u)" = 0 ] || SUDO="sudo -n"
case "$(uname -m)" in x86_64|amd64) a=amd64;; aarch64|arm64) a=arm64;; armv7l|armv6l) a=arm;; *) echo "不支持的架构 $(uname -m)"; exit 1;; esac
echo "==> 查询 frp 最新版本"
v=$(curl -fsSL https://api.github.com/repos/fatedier/frp/releases/latest | sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -1)
[ -n "$v" ] || { echo "无法获取 frp 版本，服务器能访问 GitHub 吗？"; exit 1; }
echo "==> 下载 frp $v（linux/$a）"
t=$(mktemp -d); cd "$t"
curl -fsSL -o frp.tgz "https://github.com/fatedier/frp/releases/download/v$v/frp_${v}_linux_$a.tar.gz"
tar xzf frp.tgz
echo "==> 安装 /usr/local/bin/frps"
$SUDO install -m 755 "frp_${v}_linux_$a/frps" /usr/local/bin/frps
$SUDO mkdir -p /etc/frp
printf 'bindPort = %[1]d\nauth.method = "token"\nauth.token = "%[2]s"\n' | $SUDO tee /etc/frp/frps.toml >/dev/null
printf '[Unit]\nDescription=frps\nAfter=network.target\n\n[Service]\nExecStart=/usr/local/bin/frps -c /etc/frp/frps.toml\nRestart=always\n\n[Install]\nWantedBy=multi-user.target\n' | $SUDO tee /etc/systemd/system/frps.service >/dev/null
echo "==> 启动 frps 服务"
$SUDO systemctl daemon-reload
$SUDO systemctl enable frps >/dev/null 2>&1
$SUDO systemctl restart frps
sleep 1
rm -rf "$t"
$SUDO systemctl is-active --quiet frps || { echo "frps 未能启动，可在服务器上运行 journalctl -u frps 查看原因"; exit 1; }
echo "==> frps 正在端口 %[1]d 上运行"
`

// DeployFRPS logs in over SSH and installs frps with a fresh token, streaming progress lines.
// ponytail: the host key is accepted on first use and only shown; pin it if this grows past setup.
func (s *ToolService) DeployFRPS(d FrpsDeploy) (Frps, error) {
	if d.Port == 0 {
		d.Port = 22
	}
	if d.BindPort == 0 {
		d.BindPort = 7000
	}
	var auth []ssh.AuthMethod
	if d.Key != "" {
		signer, err := ssh.ParsePrivateKey([]byte(d.Key))
		if err != nil {
			return Frps{}, fmt.Errorf("私钥无法解析：%w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if d.Password != "" {
		auth = append(auth, ssh.Password(d.Password))
	}
	if len(auth) == 0 {
		return Frps{}, errors.New("需要密码或私钥")
	}
	cfg := &ssh.ClientConfig{
		User: d.User, Auth: auth, Timeout: 15 * time.Second,
		HostKeyCallback: func(host string, _ net.Addr, key ssh.PublicKey) error {
			emit("frps-deploy", "==> 服务器指纹 "+ssh.FingerprintSHA256(key))
			return nil
		},
	}
	emit("frps-deploy", "==> 连接 "+d.User+"@"+d.Host)
	client, err := ssh.Dial("tcp", net.JoinHostPort(d.Host, strconv.Itoa(d.Port)), cfg)
	if err != nil {
		return Frps{}, fmt.Errorf("SSH 连接失败：%w", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return Frps{}, err
	}
	defer session.Close()

	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	session.Stdin = strings.NewReader(fmt.Sprintf(frpsScript, d.BindPort, token))
	out, err := session.StdoutPipe()
	if err != nil {
		return Frps{}, err
	}
	if err := session.Start("sh -s 2>&1"); err != nil {
		return Frps{}, err
	}
	sc := bufio.NewScanner(out)
	var last string
	for sc.Scan() {
		last = sc.Text()
		emit("frps-deploy", last)
	}
	if err := session.Wait(); err != nil {
		return Frps{}, fmt.Errorf("部署失败：%s", strings.TrimSpace(last))
	}
	return Frps{Token: token, BindPort: d.BindPort}, nil
}

// HostKey is the SSH server's public key as "type base64", read in the handshake before any login.
func (s *ToolService) HostKey(host string, port int) (string, error) {
	var got ssh.PublicKey
	cfg := &ssh.ClientConfig{Timeout: 10 * time.Second, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		got = key
		return errors.New("done")
	}}
	_, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(cmp.Or(port, 22))), cfg)
	if got == nil {
		return "", fmt.Errorf("无法读取服务器公钥：%w", err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(got))), nil
}
