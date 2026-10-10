import { useCallback, useEffect, useState } from "react";
import { CoreService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import type { Activity as ActivityData, Latency, Status, Traffic, Usage } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import { size, sizeText, speed } from "./format";

const modes: Record<string, string> = { rule: "规则判定", global: "全局代理", direct: "直接连接" };

function Num({ value, unit, cls = "big" }: { value: string; unit: string; cls?: string }) {
  return <div className={cls}>{value}<small>{unit}</small></div>;
}

function Ms({ ms }: { ms: number }) {
  return ms > 0 ? <Num cls="mid" value={String(ms)} unit="ms" /> : <div className="mid muted">—</div>;
}

// Sparkline draws the last values as a filled area, scaled to the busiest second.
function Sparkline({ values, color }: { values: number[]; color: string }) {
  const n = Math.max(values.length, 2);
  const peak = Math.max(1, ...values);
  const pts = values.map((v, i) => `${(i / (n - 1)) * 100},${100 - (v / peak) * 92}`);
  if (pts.length < 2) return null;
  const line = pts.join(" ");
  const id = `fill-${color.replace(/\W/g, "")}`;
  return (
    <svg className="spark" viewBox="0 0 100 100" preserveAspectRatio="none">
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor={color} stopOpacity="0.28" />
          <stop offset="1" stopColor={color} stopOpacity="0.02" />
        </linearGradient>
      </defs>
      <polygon points={`0,100 ${line} ${(values.length - 1) / (n - 1) * 100},100`} fill={`url(#${id})`} />
      <polyline points={line} fill="none" stroke={color} strokeWidth="1.6" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

function SpeedCard({ label, values, color }: { label: string; values: number[]; color: string }) {
  const [v, u] = speed(values[values.length - 1] ?? 0);
  const peak = Math.max(0, ...values);
  return (
    <div className="card speed-card">
      <div className="label">{label}</div>
      <Num value={v} unit={u} />
      {peak > 0 && <div className="scale">{speed(peak).join(" ")}</div>}
      <Sparkline values={values} color={color} />
    </div>
  );
}

function Ranking({ activity }: { activity: ActivityData }) {
  const tabs: [string, Usage[] | null][] = [
    ["进程与设备", activity.byProcess],
    ["域名", activity.byHost],
    ["策略", activity.byPolicy],
  ];
  const [tab, setTab] = useState(0);
  const rows = tabs[tab][1] ?? [];
  const peak = Math.max(1, ...rows.map((r) => r.bytes));
  return (
    <div className="card">
      <div className="label">流量（当前连接）</div>
      <div className="seg wide">
        {tabs.map(([name], i) => (
          <button key={name} className={i === tab ? "on" : ""} onClick={() => setTab(i)}>{name}</button>
        ))}
      </div>
      {rows.length === 0 && <div className="muted">暂无连接</div>}
      <div className="rank">
        {rows.map((r) => (
          <div className="row" key={r.name}>
            <span className="avatar">{r.name.slice(0, 1).toUpperCase()}</span>
            <span className="name" title={r.name}>{r.name}</span>
            <span className="bytes">{sizeText(r.bytes)}</span>
            <span className="bar"><i style={{ width: `${(r.bytes / peak) * 100}%` }} /></span>
          </div>
        ))}
      </div>
    </div>
  );
}

export default function Activity({ status, activity, history }: { status: Status; activity: ActivityData | null; history: Traffic[] }) {
  const [latency, setLatency] = useState<Latency | null>(null);
  const [measuring, setMeasuring] = useState(false);
  const [ip, setIP] = useState("");
  const [menu, setMenu] = useState(false);

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
        <h1 className="page-title">活动</h1>
        <div className="card">{status.error ? <div className="error">{status.error}</div> : <div className="muted">内核启动中…</div>}</div>
      </>
    );
  }

  const choose = (m: string) => {
    setMenu(false);
    CoreService.SetMode(m).catch(console.error);
  };
  const [total, totalUnit] = size((activity?.uploadTotal ?? 0) + (activity?.downloadTotal ?? 0));

  return (
    <>
      <h1 className="page-title">活动</h1>
      <div className="facts">
        <div className="fact"><div className="label">网络</div><div className="value">{activity?.network ?? "—"}</div></div>
        <div className="fact"><div className="label">配置</div><div className="value">{activity?.profile ?? "—"}</div></div>
        <div className="fact" style={{ position: "relative" }}>
          <div className="label">出站模式</div>
          <button className="value" onClick={() => setMenu(!menu)}>{modes[mode] ?? mode} ▾</button>
          {menu && (
            <div className="menu">
              {Object.entries(modes).map(([k, label]) => <button key={k} onClick={() => choose(k)}>{label}</button>)}
            </div>
          )}
        </div>
        <div className="fact"><div className="label">外部 IP</div><div className="value">{ip || "…"}</div></div>
      </div>

      <div className="grid">
        <div className="col">
          <div className="card">
            <div className="head">
              <div className="label">
                INTERNET 延迟
                <button className="icon-btn" onClick={measure} disabled={measuring} title="重新测试">↻</button>
              </div>
            </div>
            {latency && latency.internet > 0 ? <Num value={String(latency.internet)} unit="ms" /> : <div className="big muted">—</div>}
            <div className="stats">
              <div><div className="label">DNS</div><Ms ms={latency?.dns ?? 0} /></div>
              <div><div className="label">{(latency?.group ?? "代理").toUpperCase()}</div><Ms ms={latency?.proxy ?? 0} /></div>
              <div><div className="label">内核</div><div className="mid">{status.version}</div></div>
            </div>
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
              <div><div className="mid">{activity?.tun ? "TUN" : "代理端口"}</div><div className="label">接入</div></div>
            </div>
          </div>

          <div className="card">
            <div className="head">
              <div className="label">总计</div>
              <div className="seg"><button className="on">本次运行</button></div>
            </div>
            <Num value={total} unit={totalUnit} />
          </div>
        </div>

        <div className="col">
          <div className="row2">
            <SpeedCard label="上传" values={history.map((t) => t.up)} color="var(--up)" />
            <SpeedCard label="下载" values={history.map((t) => t.down)} color="var(--down)" />
          </div>
          {activity && <Ranking activity={activity} />}
        </div>
      </div>
    </>
  );
}
