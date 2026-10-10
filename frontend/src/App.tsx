import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { CoreService, EnhancedService, SystemProxyService } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import type { Activity as ActivityData, Event as AppEvent, LogLine, Status, Traffic } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import Activity from "./Activity";
import Overview from "./Overview";
import Proxies from "./Proxies";
import Dashboard from "./Dashboard";
import Rules from "./Rules";
import Tunnels from "./Tunnels";
import Devices from "./Devices";
import Settings from "./Settings";
import ErrorBoundary from "./ErrorBoundary";

const HISTORY = 60; // seconds of traffic kept for the sparklines

const LOG_LINES = 500;

const icon = (d: string) => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
    <path d={d} />
  </svg>
);

type Page = { key: string; title: string; icon: JSX.Element; ready: boolean };
const nav: { section?: string; pages: Page[] }[] = [
  {
    pages: [
      { key: "activity", title: "活动", ready: true, icon: icon("M3 12h3.5l2.5-6.5 5 13 2.5-6.5H21") },
      { key: "overview", title: "概览", ready: true, icon: icon("M5 5h5v5H5zM14 5h5v5h-5zM5 14h5v5H5zM14 14h5v5h-5z") },
      { key: "dashboard", title: "面板", ready: true, icon: icon("M4.79 17.5A8.5 8.5 0 1 1 19.21 17.5M12 13l4-4M10.8 13a1.2 1.2 0 1 0 2.4 0a1.2 1.2 0 1 0-2.4 0") },
    ],
  },
  {
    section: "客户端",
    pages: [
      { key: "devices", title: "设备", ready: true, icon: icon("M3 15V6.5a1 1 0 0 1 1-1h9a1 1 0 0 1 1 1V15M1.5 18.5h14M18.5 9h2.5a1 1 0 0 1 1 1v8.5a1 1 0 0 1-1 1h-2.5a1 1 0 0 1-1-1V10a1 1 0 0 1 1-1z") },
    ],
  },
  {
    section: "代理",
    pages: [
      { key: "proxies", title: "策略", ready: true, icon: icon("M3 12h5.5c2 0 3-1.5 4-3s2-3 4-3H21M8.5 12c2 0 3 1.5 4 3s2 3 4 3H21M18 3l3 3-3 3M18 15l3 3-3 3") },
      { key: "rules", title: "规则", ready: true, icon: icon("M4 5h16l-6 7.5V18l-4 2v-7.5z") },
      { key: "tunnels", title: "反向", ready: true, icon: icon("M4 8.5h15M15.5 5l3.5 3.5-3.5 3.5M20 15.5H5M8.5 12 5 15.5 8.5 19") },
    ],
  },
];
const settingsPage: Page = { key: "settings", title: "设置", ready: true, icon: icon("M4 7h8M16 7h4M12 7a2 2 0 1 0 4 0a2 2 0 1 0-4 0M4 17h4M12 17h8M8 17a2 2 0 1 0 4 0a2 2 0 1 0-4 0") };

// EventsButton lists what the app has to tell (core started, proxy restored, failed actions…).
function EventsButton() {
  const [list, setList] = useState<AppEvent[]>([]);
  const [seen, setSeen] = useState(0);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    CoreService.Events().then((l) => setList(l ?? []));
    return Events.On("event", (e) => setList((l) => [...l.slice(-49), e.data]));
  }, []);
  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    document.addEventListener("click", close);
    return () => document.removeEventListener("click", close);
  }, [open]);
  const unread = Math.max(0, list.length - seen);
  const last = list[list.length - 1];
  const label = list.length === 0 ? "无事件" : unread > 0 ? `${unread} 个新事件` : `${list.length} 个事件`;
  const toggle = (e: React.MouseEvent) => { e.stopPropagation(); setOpen(!open); setSeen(list.length); };
  const clear = () => { CoreService.ClearEvents(); setList([]); setSeen(0); setOpen(false); };
  return (
    <div className="events">
      <button className="pill small" onClick={toggle}>
        <span className={`dot ${unread > 0 && last ? { error: "err", warning: "warn", info: "on" }[last.level] ?? "" : ""}`} />{label}
      </button>
      {open && (
        <div className="menu events-menu" onClick={(e) => e.stopPropagation()}>
          {list.length === 0 && <div className="detail-head">暂无事件</div>}
          {[...list].reverse().map((ev, i) => (
            <div key={i} className={`event-row ${ev.level}`}>
              <span className="dot" /><span className="event-text">{ev.text}</span><span className="muted">{ev.time}</span>
            </div>
          ))}
          {list.length > 0 && <div className="event-foot"><button className="btn small-btn" onClick={clear}>清除</button></div>}
        </div>
      )}
    </div>
  );
}

export default function App() {
  const [current, setCurrent] = useState("activity");
  const [status, setStatus] = useState<Status>({ running: false, version: "", error: "", warning: "" });
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

  // the "status" event can be missed while the page loads, and the first Status() reply can land
  // after it with the stale "starting"; ask again until the core has settled either way
  useEffect(() => {
    if (status.running || status.error) return;
    const t = setInterval(() => CoreService.Status().then(setStatus), 1000);
    return () => clearInterval(t);
  }, [status.running, status.error]);

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
        <div className="bottom">
          <button className={`item ${current === settingsPage.key ? "active" : ""}`} onClick={() => setCurrent(settingsPage.key)}>{settingsPage.icon}{settingsPage.title}</button>
        </div>
      </nav>
      <main className="main">
        <div className="topbar">
          <EventsButton />
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
          {status.warning && <div className="card error" style={{ marginBottom: 16 }}>⚠︎ {status.warning}</div>}
          <ErrorBoundary key={current} onReset={() => setCurrent("activity")}>
          {current === "activity" && <Activity status={status} activity={activity} history={history} />}
          {current === "overview" && <Overview status={status} onOpen={setCurrent} />}
          {current === "proxies" && <Proxies status={status} />}
          {current === "dashboard" && <Dashboard status={status} logs={logs} onClearLogs={() => setLogs([])} />}
          {current === "rules" && <Rules status={status} />}
          {current === "tunnels" && <Tunnels status={status} />}
          {current === "settings" && <Settings key={current} status={status} />}
          </ErrorBoundary>
          {current === "devices" && <Devices status={status} />}
        </div>
      </main>
    </div>
  );
}
