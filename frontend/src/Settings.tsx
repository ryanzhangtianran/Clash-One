import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { EnhancedService, ProfileService, SettingsService, SystemProxyService } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import type { About, ProcessChoice, Status } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import Profiles from "./Profiles";

const tiles = [
  { key: "general", title: "通用", desc: "系统代理、增强模式、端口、开机启动等基础设置。", color: "#3b82f6", icon: "M5 7h14M5 17h14M9 7a2 2 0 1 1-4 0 2 2 0 0 1 4 0zM19 17a2 2 0 1 1-4 0 2 2 0 0 1 4 0z" },
  { key: "dns", title: "DNS", desc: "DNS 模式、上游服务器、系统分域名 DNS 和 hosts。", color: "#f59e0b", icon: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM15 9l-2 4-4 2 2-4z" },
  { key: "profiles", title: "配置", desc: "订阅与本地配置，以及合并在其上的覆写。", color: "#8b5cf6", icon: "M6 3h9l4 4v14H6zM14 3v5h5M9 13h7M9 17h7" },
  { key: "about", title: "关于", desc: "版本、数据目录与特权服务状态。", color: "#ec4899", icon: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v6M12 7.5v.5" },
];

function Row({ title, desc, children }: { title: string; desc?: string; children: React.ReactNode }) {
  return (
    <div className="setting-row">
      <div><div className="setting-title">{title}</div>{desc && <div className="muted small">{desc}</div>}</div>
      <div className="setting-control">{children}</div>
    </div>
  );
}

// Switch is WebKit's native toggle (<input type=checkbox switch>, Safari 17.4+).
export function Switch({ on, onChange, disabled }: { on: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return <input type="checkbox" {...{ switch: "" }} checked={on} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />;
}

// sameShape keeps a value only if it matches the fallback's type: an unset Go value can reach JS
// as a non-null placeholder (not null), which must not replace e.g. an empty list.
function sameShape<T>(v: unknown, fallback: T): T {
  if (Array.isArray(fallback)) return (Array.isArray(v) ? v : fallback) as T;
  return (v != null && typeof v === typeof fallback ? v : fallback) as T;
}

// useSetting reads a dotted config value and writes it through the override.
export function useSetting<T>(path: string, fallback: T): [T, (v: T) => Promise<void>, string] {
  const [value, setValue] = useState<T>(fallback);
  const [error, setError] = useState("");
  useEffect(() => {
    const load = () => ProfileService.Value(path).then((v) => setValue(sameShape(v, fallback))).catch(() => {});
    load();
    return Events.On("setting", (e) => e.data === path && load()); // also changed from the menu bar
  }, [path]);
  const save = (v: T) => {
    setError("");
    return ProfileService.Set(path, v).then(() => setValue(v)).catch((e) => setError(String(e)));
  };
  return [value, save, error];
}

// ProcessPicker edits the enhanced-mode process list by picking running apps, networked
// processes or an app from Finder; each change is saved right away.
export function ProcessPicker({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const [open, setOpen] = useState(false);
  const [choices, setChoices] = useState<ProcessChoice[]>([]);
  const [query, setQuery] = useState("");
  const [names, setNames] = useState<Record<string, string>>({}); // exec -> display name
  useEffect(() => {
    if (!open) return;
    SettingsService.ProcessChoices().then((c) => {
      const list = c ?? [];
      setChoices(list);
      setNames((n) => ({ ...n, ...Object.fromEntries(list.map((x) => [x.exec, x.name])) }));
    });
  }, [open]);
  useEffect(() => {
    const close = () => setOpen(false);
    window.addEventListener("click", close);
    return () => window.removeEventListener("click", close);
  }, []);

  const has = (exec: string) => value.some((v) => v.toLowerCase() === exec.toLowerCase());
  const add = (c: ProcessChoice) => {
    setNames((n) => ({ ...n, [c.exec]: c.name }));
    if (!has(c.exec)) onChange([...value, c.exec]);
    setOpen(false);
    setQuery("");
  };
  const pick = () => SettingsService.PickApp().then((c) => c.exec && add(c)).catch(() => {});
  const q = query.toLowerCase();
  const shown = choices.filter((c) => !has(c.exec) && (!q || `${c.name} ${c.exec}`.toLowerCase().includes(q)));
  const label = (exec: string) => (names[exec] && names[exec] !== exec ? `${names[exec]}（${exec}）` : exec);

  return (
    <div className="picker" onClick={(e) => e.stopPropagation()}>
      <div className="chips">
        {value.length === 0 && <span className="muted small">未选择进程</span>}
        {value.map((exec) => (
          <span key={exec} className="chip">{label(exec)}<button onClick={() => onChange(value.filter((v) => v !== exec))} title="移除">×</button></span>
        ))}
        <button className="btn small-btn" onClick={() => setOpen(!open)}>添加 ▾</button>
      </div>
      {open && (
        <div className="menu picker-menu">
          <input autoFocus placeholder="搜索应用或进程，回车直接添加输入的名称" value={query} onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && query.trim()) add({ name: query.trim(), exec: query.trim(), source: "manual" }); }} />
          <div className="picker-list">
            {(["app", "network"] as const).map((src) => {
              const items = shown.filter((c) => c.source === src);
              return items.length === 0 ? null : (
                <div key={src}>
                  <div className="detail-head">{src === "app" ? "正在运行的应用" : "正在联网的进程"}</div>
                  {items.map((c) => (
                    <button key={c.exec} onClick={() => add(c)}>
                      <span>{c.name}</span>
                      {c.name !== c.exec && <span className="muted small">{c.exec}</span>}
                    </button>
                  ))}
                </div>
              );
            })}
            {shown.length === 0 && <div className="detail-head">{choices.length ? "没有匹配项" : "加载中…"}</div>}
          </div>
          <button onClick={pick}>从「应用程序」中选择…</button>
        </div>
      )}
    </div>
  );
}

function General({ status }: { status: Status }) {
  const [sysProxy, setSysProxy] = useState(false);
  const [enhanced, setEnhanced] = useState(false);
  const [daemon, setDaemon] = useState(false);
  const [login, setLogin] = useState(false);
  const [traySpeed, setTraySpeed] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [mode, setMode, modeErr] = useSetting<string>("capture.mode", "system-proxy");
  const [processes, setProcesses, procErr] = useSetting<string[]>("capture.processes", []);
  const [port, setPort, portErr] = useSetting<number>("mixed-port", 7899);
  const [portText, setPortText] = useState("");
  const [logLevel, setLogLevel, logErr] = useSetting<string>("log-level", "info");

  const load = useCallback(() => {
    SystemProxyService.Enabled().then(setSysProxy);
    EnhancedService.Enabled().then(setEnhanced);
    EnhancedService.DaemonInstalled().then(setDaemon);
    SettingsService.LaunchAtLogin().then(setLogin);
    SettingsService.TraySpeed().then(setTraySpeed);
  }, []);
  useEffect(() => {
    load();
    const offs = [Events.On("sysproxy", (e) => setSysProxy(e.data)), Events.On("enhanced", (e) => { setEnhanced(e.data); load(); })];
    return () => offs.forEach((o) => o());
  }, [load]);
  useEffect(() => setPortText(String(port)), [port]);

  const run = (key: string, p: Promise<unknown>) => {
    setBusy(key); setError("");
    p.catch((e) => setError(String(e))).finally(() => { setBusy(""); load(); });
  };
  const scope = mode === "process-tun" ? "process" : "all";

  return (
    <>
      {[error, modeErr, procErr, portErr, logErr].filter(Boolean).map((e, i) => <div key={i} className="card error" style={{ marginBottom: 12 }}>{e}</div>)}
      <div className="card settings-card">
        <div className="section-title">接入</div>
        <Row title="系统代理">
          <Switch on={sysProxy} disabled={!status.running} onChange={(v) => run("sys", SystemProxyService.SetEnabled(v))} />
        </Row>
        <Row title="增强模式">
          <Switch on={enhanced} disabled={!status.running || busy === "enh"} onChange={(v) => run("enh", EnhancedService.SetEnabled(v))} />
        </Row>
        <Row title="增强模式范围">
          <div className="seg">
            <button className={scope === "all" ? "on" : ""} onClick={() => setMode("tun")}>全部流量</button>
            <button className={scope === "process" ? "on" : ""} onClick={() => setMode("process-tun")}>仅指定进程</button>
          </div>
        </Row>
        {scope === "process" && (
          <Row title="指定进程">
            <ProcessPicker value={processes ?? []} onChange={setProcesses} />
          </Row>
        )}
        <Row title="特权服务">
          <span className="muted">{daemon ? "已安装" : "未安装"}</span>
          {daemon && <button className="btn" disabled={!!busy} onClick={() => run("daemon", EnhancedService.Uninstall())}>{busy === "daemon" ? "卸载中…" : "卸载"}</button>}
        </Row>
      </div>

      <div className="card settings-card">
        <div className="section-title">内核</div>
        <Row title="代理端口">
          <div className="inline">
            <input value={portText} onChange={(e) => setPortText(e.target.value)} style={{ width: 90 }} />
            <button className="btn" disabled={portText === String(port)} onClick={() => setPort(Number(portText))}>保存</button>
          </div>
        </Row>
        <Row title="日志级别">
          <select value={logLevel} onChange={(e) => setLogLevel(e.target.value)}>
            {["debug", "info", "warning", "error", "silent"].map((l) => <option key={l}>{l}</option>)}
          </select>
        </Row>
      </div>

      <div className="card settings-card">
        <div className="section-title">应用</div>
        <Row title="开机时启动">
          <Switch on={login} onChange={(v) => run("login", SettingsService.SetLaunchAtLogin(v))} />
        </Row>
        <Row title="菜单栏显示网速">
          <Switch on={traySpeed} onChange={(v) => run("tray", SettingsService.SetTraySpeed(v))} />
        </Row>
      </div>
    </>
  );
}

function Dns() {
  const [enable, setEnable, e1] = useSetting<boolean>("dns.enable", false);
  const [mode, setMode, e2] = useSetting<string>("dns.enhanced-mode", "redir-host");
  const [servers, setServers, e3] = useSetting<string[]>("dns.nameserver", []);
  const [text, setText] = useState("");
  const [split, setSplit, e4] = useSetting<boolean>("dns.use-system-resolvers", false);
  const [hosts, setHosts, e5] = useSetting<boolean>("dns.use-system-hosts", true);
  const [respect, setRespect, e6] = useSetting<boolean>("dns.respect-rules", false);
  useEffect(() => setText((servers ?? []).join("\n")), [servers]);
  return (
    <>
      {[e1, e2, e3, e4, e5, e6].filter(Boolean).map((e, i) => <div key={i} className="card error" style={{ marginBottom: 12 }}>{e}</div>)}
      <div className="card settings-card">
        <Row title="启用 DNS"><Switch on={enable} onChange={setEnable} /></Row>
        <Row title="模式">
          <div className="seg">
            {["fake-ip", "redir-host"].map((m) => <button key={m} className={mode === m ? "on" : ""} onClick={() => setMode(m)}>{m}</button>)}
          </div>
        </Row>
        <Row title="上游服务器">
          <div style={{ width: 320, display: "flex", flexDirection: "column", alignItems: "flex-end" }}>
            <textarea className="code" rows={4} placeholder={"每行一个，如\n223.5.5.5\nhttps://1.12.12.12/dns-query"} value={text} onChange={(e) => setText(e.target.value)} style={{ marginTop: 0 }} />
            <button className="btn" onClick={() => setServers(text.split("\n").map((s) => s.trim()).filter(Boolean))}>保存</button>
          </div>
        </Row>
        <Row title="跟随系统分域名 DNS" desc="VPN 等为特定域名设置的 DNS 继续生效。"><Switch on={split} onChange={setSplit} /></Row>
        <Row title="使用系统 hosts"><Switch on={hosts} onChange={setHosts} /></Row>
        <Row title="DNS 查询遵循规则"><Switch on={respect} onChange={setRespect} /></Row>
      </div>
    </>
  );
}

function AboutPage() {
  const [about, setAbout] = useState<About | null>(null);
  useEffect(() => { SettingsService.About().then(setAbout); }, []);
  if (!about) return null;
  return (
    <div className="card settings-card">
      <Row title="Clash One">{about.version}</Row>
      <Row title="内核 clash-one-core">{about.coreVersion || "—"}</Row>
      <Row title="内核运行方式">{about.attached ? "特权服务（root）" : "子进程（当前用户）"}</Row>
      <Row title="数据目录" desc={about.dataDir}><button className="btn" onClick={() => SettingsService.OpenDataDir()}>在 Finder 中显示</button></Row>
    </div>
  );
}

export default function Settings({ status }: { status: Status }) {
  const [sub, setSub] = useState("");
  const tile = tiles.find((t) => t.key === sub);
  useEffect(() => {
    if (!tile) return;
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setSub("");
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [tile]);
  return (
    <>
      <div className="settings-grid">
        {tiles.map((t) => (
          <button key={t.key} className="settings-tile" onClick={() => setSub(t.key)}>
            <span className="settings-icon" style={{ "--c": t.color } as React.CSSProperties}>
              <svg viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d={t.icon} /></svg>
            </span>
            <span className="settings-name">{t.title}</span>
            <span className="muted small">{t.desc}</span>
          </button>
        ))}
      </div>
      {tile && (
        <div className="modal-bg" onClick={() => setSub("")}>
          <div className="sheet" onClick={(e) => e.stopPropagation()}>
            <div className="sheet-head"><div className="sheet-title">{tile.title}</div><div className="muted small">{tile.desc}</div></div>
            <div className="sheet-body">
              {sub === "general" && <General status={status} />}
              {sub === "dns" && <Dns />}
              {sub === "profiles" && <Profiles />}
              {sub === "about" && <AboutPage />}
            </div>
            <div className="sheet-foot"><button className="btn primary" onClick={() => setSub("")}>完成</button></div>
          </div>
        </div>
      )}
    </>
  );
}
