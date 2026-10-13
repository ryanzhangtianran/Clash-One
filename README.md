# Connaio

macOS 桌面客户端（Wails 3 + React），内核是 [connaio-core](https://github.com/ryanzhangtianran/connaio-core)。

## 安装

从 [Releases](https://github.com/ryanzhangtianran/Connaio/releases) 下载对应平台的文件。增强模式（TUN）需要先在「通用」中授予 root 权限（输入一次管理员密码）。

**macOS**（Apple 芯片，macOS 15 及以上）：打开 `Connaio-<版本>-macos-arm64.dmg`，把 Connaio 拖到 Applications。App 没有经过 Apple 公证，首次打开前在终端执行一次：

```bash
xattr -cr /Applications/Connaio.app
```

**Linux**（Ubuntu 22.04 及以上，x86-64）：

```bash
sudo apt install ./Connaio-<版本>-linux-amd64.deb
```

托盘图标需要桌面支持 AppIndicator（Ubuntu 自带的 GNOME 默认开启）。

**Windows**（Windows 10 1809 及以上 / Windows 11，x64）：运行 `Connaio-<版本>-windows-amd64.exe` 安装。增强模式（TUN）需要先在「通用」中授予管理员权限（一次 UAC 确认），内核会装成一个 Windows 服务；Wintun 驱动随安装包附带。

## 开发

需要 Go 1.26、Node 24，以及与 `go.mod` 相同版本的 Wails CLI：

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.28
```

内核源码默认在 `../connaio-core`，可用 `CORE_DIR=...` 覆盖。

```bash
wails3 dev        # 开发模式（前端热更新）
wails3 package    # 产出 bin/Connaio.app，内核打包在 Contents/MacOS/connaio-core
build/darwin/make-dmg.sh bin/Connaio.app bin/Connaio-1.0.0-macos-arm64.dmg   # 发布用的拖拽安装盘
wails3 task linux:package   # 在 Ubuntu 22.04 上：产出 bin/connaio.deb（GitHub Actions 打标签时自动构建并附到 Release）
wails3 task windows:package # 产出 bin/Connaio-<版本>-windows-amd64.exe，需要 makensis；GUI 和内核都不用 cgo，在 macOS 上也能交叉编译（GitHub Actions 打标签时在 Windows 上构建并附到 Release）
```

## 结构

- `internal/app`：暴露给前端的服务（内核控制、配置、增强模式、设置、设备）、菜单栏、流量账本、诊断与测速
- `internal/core`：以子进程启动内核，或连接已安装的 root 内核，经 Unix socket 访问 REST API
- `internal/daemon`：安装 / 卸载 root 内核（LaunchDaemon），增强模式（TUN）需要它
- `internal/profile`：订阅与本地配置的存储（`profile.go`）、合并成内核配置（`merge.go`）、编辑覆写（`override.go`）
- `internal/sysproxy`：macOS 系统代理
- `frontend/src`：各页面，`frontend/bindings` 由 `wails3 generate bindings` 生成

运行数据（`config.yaml`、`core.log`、`cache.db`、`traffic.json`、`profiles/`）在 `~/Library/Application Support/Connaio/`；root 内核的数据在 `/Library/Application Support/Connaio Core/`。

## 覆写

`profiles/override.yaml` 叠加在当前配置之上，订阅更新不会覆盖它。按顺序合并：

1. 普通键逐层合并：字典按键合并，其他值直接替换
2. `prepend-` / `append-` 加 `rules`、`proxies`、`proxy-groups`：放到配置里对应列表的前 / 后
3. `remove-rules`：删除配置里文本完全相同的规则
4. `group-members`（策略组 → 节点列表）：把节点放到已有策略组的最前
5. `patch-proxies`（节点 → 字段）：把字段合并到该节点上，值为 `null` 表示删除该字段
6. `remove-proxies` / `remove-proxy-groups`：按名字删除，并清理所有引用（空了的策略组回落到 DIRECT，删掉指向它们的规则）

`reverse-tunnels`（反向隧道）和 `capture`（流量接入方式）是内核直接读取的普通键。GUI 编辑覆写时保留用户的注释和格式。
