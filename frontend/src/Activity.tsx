import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { CoreService, ProfileService, SettingsService, ToolService } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import type { Activity as ActivityData, DiagSection, Latency, Status, Traffic, Usage } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import { size, sizeText, speed, useCachedURL } from "./format";
import { DiagLog } from "./ProxyDiagnose";

const modes: Record<string, string> = { rule: "规则判定", global: "全局代理", direct: "直接连接" };

function Num({ value, unit, cls = "big" }: { value: string; unit: string; cls?: string }) {
  return <div className={cls}>{value}<small>{unit}</small></div>;
}

function Ms({ ms }: { ms: number }) {
  return ms > 0 ? <Num cls="mid" value={String(ms)} unit="ms" /> : <div className="mid muted">—</div>;
}

// Sparkline plots the recent seconds across the full width, oldest at the left, scaled to the peak.
function Sparkline({ values, color }: { values: number[]; color: string }) {
  const [hover, setHover] = useState<number | null>(null);
  if (values.length < 2) return null;
  const top = Math.max(1000, ...values); // the peak is the top gridline; 1 KB/s keeps idle lines flat
  const pts = values.map((v, i) => [(i / (values.length - 1)) * 100, 96 - (v / top) * 74]);
  // quadratic curves through the midpoints: smooth, and never overshoots below the baseline
  let line = `M${pts[0]}`;
  for (let i = 1; i < pts.length - 1; i++) line += ` Q${pts[i]} ${(pts[i][0] + pts[i + 1][0]) / 2},${(pts[i][1] + pts[i + 1][1]) / 2}`;
  line += ` L${pts[pts.length - 1]}`;
  const id = `fill-${color.replace(/\W/g, "")}`;
  const move = (e: React.PointerEvent) => {
    const r = e.currentTarget.getBoundingClientRect();
    const i = Math.round(((e.clientX - r.left) / r.width) * (values.length - 1));
    setHover(Math.min(values.length - 1, Math.max(0, i)));
  };
  const h = hover != null ? pts[hover] : null;
  const ago = hover != null ? values.length - 1 - hover : 0;
  return (
    <div className="spark" onPointerMove={move} onPointerLeave={() => setHover(null)}>
      <div className="spark-grid" style={{ top: "22%" }}><span>{speed(top).join(" ")}</span></div>
      <svg viewBox="0 0 100 100" preserveAspectRatio="none">
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor={color} stopOpacity="0.2" />
            <stop offset="1" stopColor={color} stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={`${line} L${pts[pts.length - 1][0]},100 L${pts[0][0]},100 Z`} fill={`url(#${id})`} />
        <path d={line} fill="none" stroke={color} strokeWidth="2" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
      </svg>
      {h && hover != null && (
        <>
          <i className="spark-cross" style={{ left: `${h[0]}%` }} />
          <i className="spark-dot" style={{ left: `${h[0]}%`, top: `${h[1]}%`, background: color }} />
          <div className="spark-tip" style={{ left: `${h[0]}%`, transform: `translateX(${h[0] > 60 ? -100 : h[0] < 40 ? 0 : -50}%)` }}>
            {speed(values[hover]).join(" ")} · {ago === 0 ? "现在" : `${ago} 秒前`}
          </div>
        </>
      )}
    </div>
  );
}

function SpeedCard({ label, values, color }: { label: string; values: number[]; color: string }) {
  const [v, u] = speed(values[values.length - 1] ?? 0);
  return (
    <div className="card speed-card">
      <div className="label">{label}</div>
      <Num value={v} unit={u} />
      <Sparkline values={values} color={color} />
    </div>
  );
}

// TotalCard shows today's or this month's traffic, split into direct and proxied.
function TotalCard({ activity }: { activity: ActivityData | null }) {
  const [range, setRange] = useState<"today" | "month">("today");
  const { direct, proxy } = activity?.[range] ?? { direct: 0, proxy: 0 };
  const [total, unit] = size(direct + proxy);
  const part = (bytes: number) => { const [v, u] = size(bytes); return <Num cls="mid" value={v} unit={u} />; };
  return (
    <div className="card">
      <div className="head">
        <div className="label">总计</div>
        <div className="seg">
          <button className={range === "today" ? "on" : ""} onClick={() => setRange("today")}>今日</button>
          <button className={range === "month" ? "on" : ""} onClick={() => setRange("month")}>本月</button>
        </div>
      </div>
      <Num value={total} unit={unit} />
      <div className="split">
        <div><div className="label">DIRECT</div>{part(direct)}</div>
        <div className="right"><div className="label">代理</div>{part(proxy)}</div>
      </div>
      <div className="split-bar">
        {direct > 0 && <i style={{ flexGrow: direct, background: "var(--up)" }} />}
        {proxy > 0 && <i style={{ flexGrow: proxy, background: "var(--down)" }} />}
      </div>
    </div>
  );
}

// Diagnose runs the network diagnosis and shows each step as it finishes.
function Diagnose({ onClose }: { onClose: () => void }) {
  const [sections, setSections] = useState<DiagSection[]>([]);
  const [running, setRunning] = useState(true);
  useEffect(() => {
    const off = Events.On("diagnose", (e) => setSections((s) => [...s, e.data]));
    ToolService.Diagnose().finally(() => setRunning(false));
    return () => { off(); ToolService.CancelDiagnose(); };
  }, []);
  return (
    <div className="modal-bg">
      <div className="sheet" onClick={(e) => e.stopPropagation()}>
        <div className="sheet-head"><div className="sheet-title">网络诊断</div><div className="muted small">逐项检查配置、网络、DNS 和代理，帮你快速找到问题。</div></div>
        <div className="sheet-body diag">
          <DiagLog sections={sections} />
        </div>
        <div className="sheet-foot">
          {running && <span className="spinner" />}
          <span className="grow" />
          {running
            ? <button className="btn primary" onClick={() => ToolService.CancelDiagnose()}>终止</button>
            : <button className="btn primary" onClick={onClose}>完成</button>}
        </div>
      </div>
    </div>
  );
}

// niceScale rounds a peak up to 1, 2 or 5 × 10ⁿ bytes, so the gridlines land on round values.
function niceScale(v: number) {
  const p = 10 ** Math.floor(Math.log10(Math.max(v, 1)));
  return [1, 2, 5, 10].map((m) => m * p).find((m) => m >= v)!;
}

// HourChart is today's traffic per hour: a bar where there was any, a dot where there was none.
function HourChart({ values }: { values: number[] }) {
  const peak = Math.max(0, ...values);
  const top = niceScale(Math.max(peak, 1));
  const now = new Date().getHours();
  return (
    <div className="hours">
      {[1, 0.5].map((f) => <div key={f} className="hours-grid" style={{ bottom: `calc(18px + ${f} * (100% - 18px))` }}>{peak > 0 && <span>{sizeText(top * f)}</span>}</div>)}
      <div className="hours-bars">
        {values.map((v, h) => (
          <div key={h} className="hours-col" title={`${h}:00 · ${sizeText(v)}`}>
            {v > 0 ? <i style={{ height: `${Math.max(4, (v / top) * 100)}%` }} /> : <b className={h > now ? "later" : ""} />}
          </div>
        ))}
      </div>
      <div className="hours-axis">{["12AM", "6AM", "12PM", "6PM"].map((l) => <span key={l}>{l}</span>)}</div>
    </div>
  );
}

// ClientIcon is a client's Finder icon (an app's, or an executable's generic one), or a device glyph.
function ClientIcon({ path }: { path?: string }) {
  const src = useCachedURL(path, SettingsService.AppIcon);
  if (src) return <img className="client-icon" src={src} alt="" />;
  return (
    <span className="client-icon device">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
        <path d={path ? "M5 4h14a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1zM8 9l3 3-3 3M13 15h3" : "M4 6h12a1 1 0 0 1 1 1v8H3V7a1 1 0 0 1 1-1zM1.5 18h17M19 9h3v10h-3z"} />
      </svg>
    </span>
  );
}

// Ranking is today's traffic: per hour, then by client, host or policy; all of it or proxied only.
function Ranking({ activity }: { activity: ActivityData }) {
  const [proxyOnly, setProxyOnly] = useState(false);
  const [tab, setTab] = useState(0);
  const [children, setChildren] = useState(false); // list an app's own processes under it
  const pick = (s: { direct: number; proxy: number }) => (proxyOnly ? s.proxy : s.direct + s.proxy);
  const tabs: [string, Usage[] | null][] = [["进程与设备", activity.byProcess], ["域名", activity.byHost], ["策略", activity.byPolicy]];
  const rows = (tabs[tab][1] ?? []).filter((r) => pick(r) > 0).sort((a, b) => pick(b) - pick(a)).slice(0, 8);
  const peak = Math.max(1, ...rows.map(pick));
  return (
    <div className="card ranking">
      <div className="head">
        <div className="label">流量</div>
        <div className="seg">
          <button className={!proxyOnly ? "on" : ""} onClick={() => setProxyOnly(false)}>全部</button>
          <button className={proxyOnly ? "on" : ""} onClick={() => setProxyOnly(true)}>仅代理</button>
        </div>
      </div>
      <HourChart values={(activity.hours ?? []).map(pick)} />
      <div className="seg wide">
        {tabs.map(([name], i) => <button key={name} className={i === tab ? "on" : ""} onClick={() => setTab(i)}>{name}</button>)}
      </div>
      {rows.length === 0 && <div className="muted">今天还没有流量</div>}
      {tab === 0 && rows.some((r) => r.children?.length) && (
        <label className="rank-toggle"><input type="checkbox" checked={children} onChange={(e) => setChildren(e.target.checked)} /> 显示子进程</label>
      )}
      <div className={`rank ${tab === 0 ? "with-icons" : ""}`}>
        {rows.map((r) => (
          <div key={r.name}>
            <div className="row">
              {tab === 0 && <ClientIcon path={r.icon} />}
              <span className="name" title={r.name}>{r.name}</span>
              <span className="bytes">{sizeText(pick(r))}</span>
              <span className="bar"><i style={{ width: `${(pick(r) / peak) * 100}%` }} /></span>
            </div>
            {tab === 0 && children && (r.children ?? []).filter((c) => pick(c) > 0).map((c) => (
              <div className="row child" key={c.name}>
                <span className="name" title={c.name}>{c.name}</span>
                <span className="bytes">{sizeText(pick(c))}</span>
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

export default function Activity({ status, activity, history }: { status: Status; activity: ActivityData | null; history: Traffic[] }) {
  const [latency, setLatency] = useState<Latency | null>(null);
  const [measuring, setMeasuring] = useState(false);
  const [profile, setProfile] = useState(""); // the profile in use, by name
  useEffect(() => {
    const show = (p: { current: string; profiles: { id: string; name: string }[] | null }) =>
      setProfile((p.profiles ?? []).find((x) => x.id === p.current)?.name ?? "");
    ProfileService.Profiles().then(show);
    return Events.On("profiles", (e) => show(e.data));
  }, []);
  const [diagnosing, setDiagnosing] = useState(false);
  const [ip, setIP] = useState("");

  const measure = useCallback(() => {
    setMeasuring(true);
    CoreService.Latency().then(setLatency).finally(() => setMeasuring(false));
  }, []);

  useEffect(() => {
    if (!status.running) return;
    measure();
    const t = setInterval(measure, 60_000);
    return () => clearInterval(t);
  }, [status.running, measure]);

  const mode = activity?.mode ?? "";
  useEffect(() => {
    if (!status.running) return;
    setIP("");
    CoreService.ExternalIP().then(setIP).catch(() => setIP("—"));
  }, [status.running, mode]);

  if (!status.running) {
    return (
      <>
        <div className="card"><div className="muted">{status.error ? "内核未运行" : "内核启动中…"}</div></div>
      </>
    );
  }

  return (
    <div className="activity">
      <div className="facts">
        <div className="fact"><div className="label">网络</div><div className="value">{activity?.network ?? "—"}</div></div>
        <div className="fact"><div className="label">配置</div><div className="value">{profile || "—"}</div></div>
        <div className="fact"><div className="label">出站模式</div><div className="value">{modes[mode] ?? mode}</div></div>
        <div className="fact"><div className="label">外部 IP</div><div className="value">{ip || "…"}</div></div>
      </div>

      {/* one grid, so cards side by side share a row height */}
      <div className="grid">
        <div className="card">
          <div className="head">
            <div className="label">
              INTERNET 延迟
              <button className="icon-btn" onClick={measure} disabled={measuring} title="重新测试">↻</button>
            </div>
            <button className="btn small-btn" onClick={() => setDiagnosing(true)}>网络诊断</button>
          </div>
          {latency && latency.internet > 0 ? <Num value={String(latency.internet)} unit="ms" /> : <div className="big muted">—</div>}
          <div className="stats">
            <div><div className="label">路由</div><Ms ms={latency?.router ?? 0} /></div>
            <div><div className="label">DNS</div><Ms ms={latency?.dns ?? 0} /></div>
            <div><div className="label">{(latency?.group ?? "代理").toUpperCase()}</div><Ms ms={latency?.proxy ?? 0} /></div>
          </div>
        </div>

        <div className="row2">
          <SpeedCard label="上传" values={history.map((t) => t.up)} color="var(--up)" />
          <SpeedCard label="下载" values={history.map((t) => t.down)} color="var(--down)" />
        </div>

        <div className="card">
          <div className="head">
            <div className="label">活动连接</div>
            <span className="dot on" />
          </div>
          <div className="big">{activity?.connections ?? 0}</div>
          <div className="stats">
            <div><div className="mid">{activity?.processes ?? 0}</div><div className="label">进程</div></div>
            <div><div className="mid">{activity?.devices ?? 0}</div><div className="label">设备</div></div>
            <div><div className="mid">{activity?.hosts ?? 0}</div><div className="label">域名</div></div>
          </div>
        </div>

        <div className="span2">{activity && <Ranking activity={activity} />}</div>

        <TotalCard activity={activity} />
      </div>
      {diagnosing && <Diagnose onClose={() => setDiagnosing(false)} />}
    </div>
  );
}
