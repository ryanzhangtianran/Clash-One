import { useCallback, useEffect, useState } from "react";
import { CoreService, EnhancedService, ProfileService, SettingsService, ToolService } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import type { About, DNSLeak, Resolver, Update } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import { on } from "./hooks";
import { words, validPort } from "./format";


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
export function useSetting<T>(path: string, fallback: T): [T, (v: T) => Promise<void>] {
  const [value, setValue] = useState<T>(fallback);
  useEffect(() => {
    const load = () => ProfileService.Value(path).then((v) => setValue(sameShape(v, fallback)));
    load();
    return on("setting", (d) => d === path && load()); // also changed from the menu bar
  }, [path]);
  const save = (v: T) => ProfileService.Set(path, v).then(() => setValue(v));
  return [value, save];
}

export function General() {
  const [daemon, setDaemon] = useState(false);
  const [busy, setBusy] = useState("");
  const [login, setLogin] = useState(false);
  const [traySpeed, setTraySpeed] = useState(true);
  const [port, setPort] = useSetting<number>("mixed-port", 7899);
  const [portText, setPortText] = useState("");
  const [logLevel, setLogLevel] = useSetting<string>("log-level", "info");
  const [version, setVersion] = useState("");
  const [update, setUpdate] = useState<Update | null>(null);
  useEffect(() => { SettingsService.About().then((a) => setVersion(a.version)); }, []);

  const load = useCallback(() => {
    EnhancedService.DaemonInstalled().then(setDaemon);
    SettingsService.LaunchAtLogin().then(setLogin);
    SettingsService.TraySpeed().then(setTraySpeed);
  }, []);
  useEffect(() => { load(); return on("enhanced", load); }, [load]);
  useEffect(() => setPortText(String(port)), [port]);

  const run = (p: Promise<unknown>, key = "") => {
    setBusy(key);
    p.finally(() => { setBusy(""); load(); });
  };

  return (
    <>
      <div className="card settings-card">
        <div className="section-title">内核</div>
        <Row title="代理端口" desc="HTTP 与 SOCKS5 共用">
          <input className="ctl" value={portText} onChange={(e) => setPortText(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && (validPort(portText) && portText !== String(port) ? setPort(Number(portText)) : setPortText(String(port)))}
            onBlur={() => setPortText(String(port))} />
        </Row>
        <Row title="日志级别">
          <select className="ctl" value={logLevel} onChange={(e) => setLogLevel(e.target.value)}>
            {["debug", "info", "warning", "error", "silent"].map((l) => <option key={l}>{l}</option>)}
          </select>
        </Row>
        <Row title="重启内核" desc="重新启动内核进程并载入当前配置">
          <button className="btn ctl" disabled={!!busy} onClick={() => run(CoreService.Restart(), "restart")}>{busy === "restart" ? "重启中…" : "重启"}</button>
        </Row>
        <Row title="root 权限" desc={daemon ? "已授予，内核以 root 运行" : "授予后才能开启增强模式"}>
          {daemon
            ? <button className="btn ctl" disabled={!!busy} onClick={() => run(EnhancedService.Uninstall(), "root")}>{busy === "root" ? "移除中…" : "移除"}</button>
            : <button className="btn ctl" disabled={!!busy} onClick={() => run(EnhancedService.Grant(), "root")}>{busy === "root" ? "授予中…" : "授予"}</button>}
        </Row>
      </div>

      <div className="card settings-card">
        <div className="section-title">应用</div>
        <Row title="开机时启动">
          <Switch on={login} onChange={(v) => run(SettingsService.SetLaunchAtLogin(v))} />
        </Row>
        <Row title={`${words.tray}显示网速`}>
          <Switch on={traySpeed} onChange={(v) => run(SettingsService.SetTraySpeed(v))} />
        </Row>
      </div>

      <div className="card settings-card">
        <div className="section-title">更新</div>
        <Row title={`当前版本 ${version}`} desc={update ? (update.newer ? `发现新版本 ${update.latest}，从 GitHub Releases 下载安装后自动重启` : "已是最新版本") : undefined}>
          {update?.newer
            ? <button className="btn ctl" disabled={!!busy} onClick={() => run(SettingsService.InstallUpdate(), "update")}>{busy === "update" ? "更新中…" : "更新"}</button>
            : <button className="btn ctl" disabled={!!busy} onClick={() => run(SettingsService.CheckUpdate().then(setUpdate), "check")}>{busy === "check" ? "检查中…" : "检查更新"}</button>}
        </Row>
      </div>
    </>
  );
}

// ListSetting edits a string list setting item by item: each one a row that can be removed, and an
// input to add one; every change is saved right away.
function ListSetting({ title, desc, path, placeholder }: { title: string; desc?: string; path: string; placeholder: string }) {
  const [list, setList] = useSetting<string[]>(path, []);
  const [item, setItem] = useState("");
  const items = list ?? [];
  const add = () => { const v = item.trim(); if (v && !items.includes(v)) setList([...items, v]).then(() => setItem("")); };
  return (
    <ListBlock title={title} desc={desc}>
      {items.map((v, i) => <ListItem key={v} onRemove={() => setList(items.filter((_, j) => j !== i))}><span className="list-value">{v}</span></ListItem>)}
      <div className="list-add">
        <input className="code-input" placeholder={placeholder} value={item} onChange={(e) => setItem(e.target.value)} onKeyDown={(e) => e.key === "Enter" && add()} />
        <button className="btn" disabled={!item.trim()} onClick={add}>添加</button>
      </div>
    </ListBlock>
  );
}

// PolicySetting edits dns.nameserver-policy: a domain (or geosite:, rule-set:) per row and its servers.
function PolicySetting() {
  const [policy, setPolicy] = useSetting<Record<string, unknown>>("dns.nameserver-policy", {});
  const [domain, setDomain] = useState("");
  const [servers, setServers] = useState("");
  const entries = Object.entries(policy ?? {});
  const list = (v: unknown) => (Array.isArray(v) ? v : [v]).map(String);
  const add = () => {
    const d = domain.trim(), sv = servers.split(",").map((x) => x.trim()).filter(Boolean);
    if (!d || !sv.length) return;
    setPolicy({ ...(policy ?? {}), [d]: sv.length === 1 ? sv[0] : sv }).then(() => { setDomain(""); setServers(""); });
  };
  return (
    <ListBlock title="分域名服务器" desc="指定域名用指定的服务器解析。">
      {entries.map(([d, v]) => (
        <ListItem key={d} onRemove={() => setPolicy(Object.fromEntries(entries.filter(([k]) => k !== d)))}>
          <span className="list-value">{d}</span><span className="list-value muted">{list(v).join(", ")}</span>
        </ListItem>
      ))}
      <div className="list-add">
        <input className="code-input" placeholder="geosite:cn" value={domain} onChange={(e) => setDomain(e.target.value)} style={{ flex: "0 1 40%" }} />
        <input className="code-input" placeholder="服务器，多个用逗号分隔" value={servers} onChange={(e) => setServers(e.target.value)} onKeyDown={(e) => e.key === "Enter" && add()} />
        <button className="btn" disabled={!domain.trim() || !servers.trim()} onClick={add}>添加</button>
      </div>
    </ListBlock>
  );
}

function ListBlock({ title, desc, children }: { title: string; desc?: string; children: React.ReactNode }) {
  return (
    <div className="setting-row list-row">
      <div><div className="setting-title">{title}</div>{desc && <div className="muted small">{desc}</div>}</div>
      <div className="list-items">{children}</div>
    </div>
  );
}

function ListItem({ onRemove, children }: { onRemove: () => void; children: React.ReactNode }) {
  return <div className="list-item">{children}<button className="list-remove" title="删除" onClick={onRemove}>−</button></div>;
}

// LeakTest resolves a fresh domain through the proxy and directly, and compares the resolvers.
function LeakTest() {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<DNSLeak | null>(null);
  const run = () => { setBusy(true); ToolService.DNSLeakTest().then(setResult).catch(() => setResult(null)).finally(() => setBusy(false)); };
  const who = (r: Resolver) => r.err ? `失败：${r.err}` : `${r.ip}（${r.geo}）`;
  return (
    <>
      <Row title="DNS 泄漏检测" desc={result ? (result.leak ? "可能泄漏：走代理的域名也由本地 DNS 解析" : result.proxy.err ? "无法通过代理检测" : "没有泄漏：走代理的域名由代理端解析") : undefined}>
        <button className="btn" disabled={busy} onClick={run}>{busy ? "检测中…" : "开始检测"}</button>
      </Row>
      {result && <>
        <Row title="通过代理"><span className="muted small selectable">{who(result.proxy)}</span></Row>
        <Row title="直连"><span className="muted small selectable">{who(result.direct)}</span></Row>
      </>}
    </>
  );
}

export function Dns() {
  const [enable, setEnable] = useSetting<boolean>("dns.enable", false);
  const [mode, setMode] = useSetting<string>("dns.enhanced-mode", "redir-host");
  const [ipv6, setIPv6] = useSetting<boolean>("dns.ipv6", false);
  const [h3, setH3] = useSetting<boolean>("dns.prefer-h3", false);
  const [split, setSplit] = useSetting<boolean>("dns.use-system-resolvers", false);
  const [hosts, setHosts] = useSetting<boolean>("dns.use-system-hosts", true);
  const [respect, setRespect] = useSetting<boolean>("dns.respect-rules", false);
  const [range, setRange] = useSetting<string>("dns.fake-ip-range", "198.18.0.1/16");
  const [rangeText, setRangeText] = useState("");
  useEffect(() => setRangeText(range ?? ""), [range]);
  return (
    <>
      <div className="card settings-card">
        <div className="section-title">基本</div>
        <Row title="启用 DNS"><Switch on={enable} onChange={setEnable} /></Row>
        <Row title="模式">
          <div className="seg">
            {["fake-ip", "redir-host"].map((m) => <button key={m} className={mode === m ? "on" : ""} onClick={() => setMode(m)}>{m.toUpperCase()}</button>)}
          </div>
        </Row>
        <Row title="解析 IPv6"><Switch on={ipv6} onChange={setIPv6} /></Row>
        <Row title="DoH 优先使用 HTTP/3"><Switch on={h3} onChange={setH3} /></Row>
        <Row title="DNS 查询遵循规则" desc="解析也按规则走代理，减少泄漏。"><Switch on={respect} onChange={setRespect} /></Row>
      </div>

      <div className="card settings-card">
        <div className="section-title">上游服务器</div>
        <ListSetting title="主服务器" path="dns.nameserver" placeholder="如 https://doh.pub/dns-query 或 223.5.5.5" />
        <ListSetting title="引导服务器" desc="解析 DoH 地址本身的域名，只能填 IP。" path="dns.default-nameserver" placeholder="如 223.5.5.5" />
        <ListSetting title="代理节点域名" desc="只用来解析代理服务器的域名。" path="dns.proxy-server-nameserver" placeholder="如 https://doh.pub/dns-query" />
        <ListSetting title="直连域名" desc="直连的连接用它解析。" path="dns.direct-nameserver" placeholder="如 system" />
        <ListSetting title="备用服务器" desc="主服务器结果可能被污染时使用。" path="dns.fallback" placeholder="如 https://1.1.1.1/dns-query" />
        <PolicySetting />
      </div>

      <div className="card settings-card">
        <div className="section-title">Fake-IP</div>
        <Row title="地址段">
          <div className="inline">
            <input value={rangeText} onChange={(e) => setRangeText(e.target.value)} style={{ width: 160 }} />
            <button className="btn" disabled={rangeText === range || !/^\d{1,3}(\.\d{1,3}){3}\/\d{1,2}$/.test(rangeText.trim())} onClick={() => setRange(rangeText.trim())}>保存</button>
          </div>
        </Row>
        <ListSetting title="不分配虚拟 IP 的域名" desc="局域网、时间服务器、游戏主机等需要真实 IP 的域名。" path="dns.fake-ip-filter" placeholder="如 +.lan、time.*.com" />
      </div>

      <div className="card settings-card">
        <div className="section-title">系统</div>
        <Row title="跟随系统分域名 DNS" desc="VPN 等为特定域名设置的 DNS 继续生效。"><Switch on={split} onChange={setSplit} /></Row>
        <Row title="使用系统 HOSTS"><Switch on={hosts} onChange={setHosts} /></Row>
      </div>

      <div className="card settings-card">
        <div className="section-title">检测</div>
        <LeakTest />
      </div>
    </>
  );
}

export function AboutPage() {
  const [about, setAbout] = useState<About | null>(null);
  useEffect(() => { SettingsService.About().then(setAbout); }, []);
  if (!about) return null;
  return (
    <div className="card settings-card">
      <Row title="Connaio">{about.version}</Row>
      <Row title="内核">{about.coreVersion || "—"}</Row>
      <Row title="内核运行方式">{about.attached ? "root" : "当前用户"}</Row>
      <Row title="数据目录" desc={about.dataDir}><button className="btn" onClick={() => SettingsService.OpenDataDir()}>{words.showInFiles}</button></Row>
    </div>
  );
}
