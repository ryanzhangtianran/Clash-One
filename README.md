# Clash One

macOS 桌面客户端（Wails 3 + React），内核是 [clash-one-core](https://github.com/ryanzhangtianran/clash-one-core)。

## 开发

需要 Go 1.26、Node 24，以及与 `go.mod` 相同版本的 Wails CLI：

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.28
```

内核源码默认在 `../clash-one-core`，可用 `CORE_DIR=...` 覆盖。

```bash
wails3 dev        # 开发模式（前端热更新）
wails3 package    # 产出 bin/Clash One.app，内核打包在 Contents/MacOS/clash-one-core
```

## 结构

- `main.go`：窗口（关闭即隐藏）与菜单栏（实时速度、出站模式、打开/退出）
- `coreservice.go`：暴露给前端的内核 API，推送 `status` / `mode` / `traffic` 事件
- `internal/core`：以子进程启动内核，经 Unix socket（`~/Library/Application Support/Clash One/core.sock`）访问 REST API
- `frontend/src`：概览、代理页面

运行数据（`config.yaml`、`core.log`、`cache.db`）在 `~/Library/Application Support/Clash One/`；首次启动写入默认配置。
