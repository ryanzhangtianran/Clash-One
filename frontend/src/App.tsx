import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { CoreService, EnhancedService, SettingsService, SystemProxyService } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import type { Activity as ActivityData, Event as AppEvent, LogLine, Status, Traffic } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import Activity from "./Activity";
import Processes from "./Processes";
import Proxies from "./Proxies";
import Dashboard from "./Dashboard";
import Logs from "./Logs";
import Rules from "./Rules";
import Tunnels from "./Tunnels";
import Devices from "./Devices";
import { AboutPage, Dns, General } from "./Settings";
import Profiles from "./Profiles";
import ErrorBoundary from "./ErrorBoundary";
import { useCachedURL } from "./format";

const HISTORY = 60; // seconds of traffic kept for the sparklines

const LOG_LINES = 500;

// NavIcon is an SF Symbol in white on a coloured rounded square, as in System Settings.
function NavIcon({ symbol, color }: { symbol: string; color: string }) {
  const src = useCachedURL(symbol, SettingsService.Symbol);
  return <span className="nav-icon" style={{ "--c": color } as React.CSSProperties}>{src && <img src={src} alt="" />}</span>;
}

type Page = { key: string; title: string; symbol: string; color: string };
// one list, from what you check most to what you touch least
const pages: Page[] = [
  { key: "activity", title: "活动", symbol: "waveform.path.ecg", color: "#ff3b30" },
  { key: "proxies", title: "策略", symbol: "arrow.triangle.branch", color: "#007aff" },
  { key: "tunnels", title: "反向", symbol: "arrow.left.arrow.right", color: "#5856d6" },
  { key: "devices", title: "设备", symbol: "laptopcomputer.and.iphone", color: "#34c759" },
  { key: "rules", title: "规则", symbol: "line.3.horizontal.decrease", color: "#ff9500" },
  { key: "processes", title: "增强", symbol: "bolt.fill", color: "#af52de" },
  { key: "profiles", title: "配置", symbol: "doc.text", color: "#a2845e" },
  { key: "dns", title: "DNS", symbol: "globe", color: "#30b0c7" },
  { key: "dashboard", title: "连接", symbol: "point.3.connected.trianglepath.dotted", color: "#5ac8fa" },
  { key: "logs", title: "日志", symbol: "list.bullet", color: "#64748b" },
  { key: "general", title: "通用", symbol: "gearshape.fill", color: "#8e8e93" },
  { key: "about", title: "关于", symbol: "info", color: "#0a84ff" },
];

const levelNames: Record<string, string> = { info: "状态", warning: "警告", error: "错误" };

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
          <div className="events-head"><span>事件</span>{list.length > 0 && <button className="link" onClick={clear}>清除</button>}</div>
          {list.length === 0 && <div className="muted event-text">暂无事件</div>}
          {[...list].reverse().map((ev, i) => (
            <div key={i} className={`event-row ${ev.level}`}>
              <div className="event-meta"><span className="dot" />{levelNames[ev.level] ?? ev.level} · {ev.time}</div>
              <div className="event-text">{ev.text}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default function App() {
  const [current, setCurrent] = useState("activity");
  const [status, setStatus] = useState<Status>({ running: false, version: "", error: "" });
  const [activity, setActivity] = useState<ActivityData | null>(null);
  const [history, setHistory] = useState<Traffic[]>([]);
  const [sysProxy, setSysProxy] = useState(false);
  const [enhanced, setEnhanced] = useState(""); // EnhancedService.Mode: "", "tun" or "process-tun"
  const [rooted, setRooted] = useState(false); // root granted: enhancing needs it
  const [logs, setLogs] = useState<LogLine[]>([]);
  const [enhancedBusy, setEnhancedBusy] = useState(false);

  useEffect(() => {
    CoreService.Status().then(setStatus);
    SystemProxyService.Enabled().then(setSysProxy);
    const mode = () => { EnhancedService.Mode().then(setEnhanced); EnhancedService.DaemonInstalled().then(setRooted); };
    mode();
    const offs = [
      Events.On("enhanced", mode),
      Events.On("navigate", (e) => setCurrent(e.data)),
      Events.On("setting", (e) => e.data === "capture.mode" && mode()),
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
        {pages.map((p) => (
          <button key={p.key} className={`item ${p.key === current ? "active" : ""}`} onClick={() => setCurrent(p.key)}><NavIcon symbol={p.symbol} color={p.color} />{p.title}</button>
        ))}
      </nav>
      <main className="main">
        <div className="topbar">
          <EventsButton />
          <button
            className="pill"
            disabled={!status.running}
            title="将 macOS 系统代理指向本机代理端口"
            onClick={() => SystemProxyService.SetEnabled(!sysProxy).catch(() => {})}
          >
            <span className={`dot ${sysProxy ? "on" : ""}`} />系统代理
          </button>
          <button
            className="pill"
            disabled={!status.running || enhancedBusy || !rooted}
            title={rooted ? "TUN 接管全部流量" : "需先在「通用」中授予 root 权限"}
            onClick={() => {
              setEnhancedBusy(true);
              EnhancedService.SetMode(enhanced === "tun" ? "" : "tun").catch(() => {}).finally(() => setEnhancedBusy(false));
            }}
          >
            <span className={`dot ${enhanced === "tun" && activity?.tun ? "on" : ""}`} />
            {enhancedBusy ? "切换中…" : "增强模式"}
          </button>
        </div>
        <div className="page">
          <ErrorBoundary key={current} onReset={() => setCurrent("activity")}>
          {current === "activity" && <Activity status={status} activity={activity} history={history} />}
          {current === "processes" && <Processes status={status} mode={enhanced} rooted={rooted} />}
          {current === "proxies" && <Proxies status={status} />}
          {current === "dashboard" && <Dashboard status={status} />}
          {current === "logs" && <Logs lines={logs} onClear={() => setLogs([])} />}
          {current === "rules" && <Rules status={status} />}
          {current === "tunnels" && <Tunnels status={status} />}
          {current === "general" && <div className="grouped"><General /></div>}
          {current === "dns" && <div className="grouped"><Dns /></div>}
          {current === "profiles" && <div className="grouped profiles-page"><Profiles /></div>}
          {current === "about" && <div className="grouped"><AboutPage /></div>}
          </ErrorBoundary>
          {current === "devices" && <Devices status={status} />}
        </div>
      </main>
    </div>
  );
}
