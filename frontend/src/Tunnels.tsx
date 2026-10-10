import { useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import TunnelDialog from "./TunnelDialog";
import { nonNull } from "./format";
import type { Status, Tunnel } from "../bindings/github.com/ryanzhangtianran/Clash-One";

const stateLabel: Record<string, string> = { running: "运行中", connecting: "连接中", retrying: "重试中" };

export default function Tunnels({ status }: { status: Status }) {
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState(false);
  const [mine, setMine] = useState<string[]>([]);
  const load = () => {
    CoreService.ReverseTunnels().then((t) => { setTunnels(t ?? []); setError(""); }).catch((e) => setError(String(e)));
    ProfileService.UserItems().then((u) => setMine(nonNull(u.tunnels).map((t) => String(t.name)))).catch(() => {});
  };

  useEffect(() => {
    if (!status.running) return;
    load();
    const t = setInterval(load, 2000);
    return () => clearInterval(t);
  }, [status.running]);

  return (
    <>
      <h1 className="page-title">反向隧道</h1>
      <div className="toolbar"><button className="btn" disabled={!status.running} onClick={() => setDialog(true)}>增加隧道</button></div>
      {error && <div className="card error" style={{ marginBottom: 16 }}>{error}</div>}
      {!error && tunnels.length === 0 && (
        <div className="card muted">还没有反向隧道，点「增加隧道」添加 frp 或 SSH -R。</div>
      )}
      {tunnels.map((t) => (
        <div className="card group-card" key={t.name}>
          <h3>
            <span className={`dot ${t.state === "running" ? "on" : t.state === "retrying" ? "err" : ""}`} />
            {t.name}
            <span className="muted">{t.type.toUpperCase()} · {stateLabel[t.state] ?? t.state} · 自 {new Date(t.since).toLocaleTimeString()}</span>
            <span className="grow" />
            {mine.includes(t.name) && <button className="btn" onClick={() => ProfileService.RemoveTunnel(t.name).then(load).catch((e) => setError(String(e)))}>删除</button>}
          </h3>
          {t.error && <div className="error small" style={{ marginBottom: 8 }}>{t.error}</div>}
          <table className="table">
            <thead><tr><th>名称</th><th>类型</th><th>远端</th><th>本地</th><th>状态</th></tr></thead>
            <tbody>
              {(t.forwards ?? []).map((f, i) => (
                <tr key={i}>
                  <td>{f.name || "—"}</td>
                  <td>{f.type || "tcp"}</td>
                  <td>{f.remote || "—"}</td>
                  <td>{f.local}</td>
                  <td title={f.error}>{f.state || stateLabel[t.state] || t.state}{f.error && " ⚠︎"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
      {dialog && <TunnelDialog onClose={() => { setDialog(false); load(); }} />}
    </>
  );
}
