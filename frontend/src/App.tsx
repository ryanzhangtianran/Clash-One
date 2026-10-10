import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { CoreService, EnhancedService, SystemProxyService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import type { Activity as ActivityData, LogLine, Status, Traffic } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import Activity from "./Activity";
import Proxies from "./Proxies";
import Profiles from "./Profiles";
import Connections from "./Connections";
import Rules from "./Rules";
import Tunnels from "./Tunnels";
import Logs from "./Logs";

const HISTORY = 60; // seconds of traffic kept for the sparklines
const LOG_LINES = 500;

const icon = (d: string) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
    <path d={d} />
  </svg>
);

type Page = { key: string; title: string; icon: JSX.Element; ready: boolean };
const nav: { section?: string; pages: Page[] }[] = [
  { pages: [{ key: "activity", title: "活动", ready: true, icon: icon("M3 12h4l3-7 4 14 3-7h4") }] },
  {
    section: "客户端",
    pages: [
      { key: "connections", title: "连接", ready: true, icon: icon("M4 5h16v14H4zM8 10l2 2-2 2M12 14h4") },
      { key: "devices", title: "设备", ready: false, icon: icon("M3 14h18v5H3zM7 16.5h.01M7 14l4-9M14 9a4 4 0 0 1 5 0") },
    ],
  },
  {
    section: "代理",
    pages: [
      { key: "proxies", title: "策略", ready: true, icon: icon("M12 21v-8M12 13 6 4M12 13l6-9M4 4h4M16 4h4") },
      { key: "rules", title: "规则", ready: true, icon: icon("M9 6h11M9 12h11M9 18h11M4 6l1 1 2-2M4 12h2M4 18h2") },
      { key: "tunnels", title: "反向隧道", ready: true, icon: icon("M4 17V7l8-4 8 4v10l-8 4zM12 3v18") },
    ],
  },
  {
    section: "设置",
    pages: [
      { key: "profiles", title: "配置", ready: true, icon: icon("M6 3h9l4 4v14H6zM14 3v5h5M9 13h7M9 17h7") },
      { key: "logs", title: "日志", ready: true, icon: icon("M4 6h16M4 10h16M4 14h10M4 18h7") },
    ],
  },
];

export default function App() {
  const [current, setCurrent] = useState("activity");
  const [status, setStatus] = useState<Status>({ running: false, version: "", error: "" });
  const [activity, setActivity] = useState<ActivityData | null>(null);
  const [history, setHistory] = useState<Traffic[]>([]);
  const [sysProxy, setSysProxy] = useState(false);
  const [sysProxyError, setSysProxyError] = useState("");
  const [enhanced, setEnhanced] = useState(false);
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [enhancedBusy, setEnhancedBusy] = useState(false);
  const [enhancedError, setEnhancedError] = useState("");

  useEffect(() => {
    CoreService.Status().then(setStatus);
    SystemProxyService.Enabled().then(setSysProxy);
    EnhancedService.Enabled().then(setEnhanced);
    const offs = [
      Events.On("enhanced", (e) => setEnhanced(e.data)),
      Events.On("log", (e) => setLogs((l) => [...l.slice(1 - LOG_LINES), e.data])),
      Events.On("sysproxy", (e) => setSysProxy(e.data)),
      Events.On("status", (e) => setStatus(e.data)),
      Events.On("traffic", (e) => setHistory((h) => [...h.slice(1 - HISTORY), e.data])),
    ];
    return () => offs.forEach((off) => off());
  }, []);

  useEffect(() => {
    if (!status.running) return;
    const poll = () => CoreService.Activity().then(setActivity).catch(() => {});
    poll();
    const t = setInterval(poll, 1000);
    return () => clearInterval(t);
  }, [status.running]);

  return (
    <div className="app">
      <nav className="sidebar">
        {nav.map((group, i) => (
          <div key={i}>
            {group.section && <div className="section">{group.section}</div>}
            {group.pages.map((p) => (
              <button
                key={p.key}
                className={`item ${p.key === current ? "active" : ""}`}
                disabled={!p.ready}
                title={p.ready ? "" : "即将推出"}
                onClick={() => setCurrent(p.key)}
              >
                {p.icon}
                {p.title}
              </button>
            ))}
          </div>
        ))}
      </nav>
      <main className="main">
        <div className="topbar">
          <button
            className="pill"
            disabled={!status.running}
            title={sysProxyError || "将 macOS 系统代理指向本机代理端口"}
            onClick={() => {
              setSysProxyError("");
              SystemProxyService.SetEnabled(!sysProxy).catch((e) => setSysProxyError(String(e)));
            }}
          >
            <span className={`dot ${sysProxy ? "on" : ""} ${sysProxyError ? "err" : ""}`} />系统代理
          </button>
          <button
            className="pill"
            disabled={!status.running || enhancedBusy}
            title={enhancedError || "TUN 接管全部流量；首次开启需要管理员授权安装特权服务"}
            onClick={() => {
              setEnhancedError("");
              setEnhancedBusy(true);
              EnhancedService.SetEnabled(!enhanced).catch((e) => setEnhancedError(String(e))).finally(() => setEnhancedBusy(false));
            }}
          >
            <span className={`dot ${enhanced && activity?.tun ? "on" : ""} ${enhancedError ? "err" : ""}`} />
            {enhancedBusy ? "切换中…" : "增强模式"}
          </button>
        </div>
        <div className="page">
          {current === "activity" && <Activity status={status} activity={activity} history={history} />}
          {current === "proxies" && <Proxies status={status} />}
          {current === "profiles" && <Profiles />}
          {current === "connections" && <Connections status={status} />}
          {current === "rules" && <Rules status={status} />}
          {current === "tunnels" && <Tunnels status={status} />}
          {current === "logs" && <Logs lines={logs} onClear={() => setLogs([])} />}
        </div>
      </main>
    </div>
  );
}
