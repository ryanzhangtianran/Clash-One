import { useCallback, useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import type { Status, Tunnel } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import TunnelDialog from "./TunnelDialog";
import { nonNull } from "./format";

const stateLabel: Record<string, string> = { running: "运行中", connecting: "连接中", retrying: "重试中" };
const stateClass: Record<string, string> = { running: "delay-good", connecting: "delay-warn", retrying: "delay-bad" };
const kinds: [string, string, string][] = [["frp", "frp", "groups"], ["ssh", "SSH -R", "proxies"]];

type Menu = { x: number; y: number; name: string };

export default function Tunnels({ status }: { status: Status }) {
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [mine, setMine] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState(false);
  const [open, setOpen] = useState("");
  const [menu, setMenu] = useState<Menu | null>(null);

  const load = useCallback(() => {
    CoreService.ReverseTunnels().then((t) => { setTunnels(t ?? []); setError(""); }).catch((e) => setError(String(e)));
    ProfileService.UserItems().then((u) => setMine(nonNull(u.tunnels).map((t) => String(t.name)))).catch(() => {});
  }, []);

  useEffect(() => {
    if (!status.running) return;
    load();
    const t = setInterval(load, 2000);
    return () => clearInterval(t);
  }, [status.running, load]);

  useEffect(() => {
    const close = () => setMenu(null);
    window.addEventListener("click", close);
    return () => window.removeEventListener("click", close);
  }, []);

  const remove = (name: string) => ProfileService.RemoveTunnel(name).then(load).catch((e) => setError(String(e)));

  return (
    <>
      <h1 className="page-title">反向隧道</h1>
      <div className="hint">把本机服务经 frps 或 SSH 服务器暴露出去；反代进来的流量按规则分流（IN-TYPE 为 FRP / SSH，IN-NAME 为隧道名）。</div>
      {error && <div className="error" style={{ marginTop: 10 }}>{error}</div>}

      {kinds.map(([type, title, color]) => (
        <div key={type}>
          <div className="tile-head"><span className={`tile-title ${color}`}>{title}</span></div>
          <div className="tiles">
            {tunnels.filter((t) => t.type === type).map((t) => {
              const forwards = t.forwards ?? [];
              return (
                <div key={t.name} className="tile-wrap">
                  <button className={`tile ${open === t.name ? "selected" : ""}`} onClick={() => setOpen(open === t.name ? "" : t.name)}
                    onContextMenu={(e) => { e.preventDefault(); if (mine.includes(t.name)) setMenu({ x: e.clientX, y: e.clientY, name: t.name }); }}>
                    <span className="kind">{t.type.toUpperCase()} · {forwards.length} 条转发</span>
                    <span className="name">{t.name}</span>
                    <span className={`foot ${stateClass[t.state] ?? ""}`}>{stateLabel[t.state] ?? t.state}{t.error && " · 有错误"}</span>
                  </button>
                  {open === t.name && (
                    <div className="menu members detail" onClick={(e) => e.stopPropagation()}>
                      <div className="detail-head">
                        {stateLabel[t.state] ?? t.state} · 自 {new Date(t.since).toLocaleTimeString()}
                      </div>
                      {t.error && <div className="error small detail-head">{t.error}</div>}
                      {forwards.map((f, i) => (
                        <div key={i} className="detail-row">
                          <span>{f.name ? `${f.name} · ` : ""}{f.type || "tcp"}</span>
                          <span className="muted">{f.remote || "—"} → {f.local}</span>
                          {f.state && <span className={f.state === "running" ? "delay-good" : "delay-warn"}>{f.state}</span>}
                          {f.error && <span className="error small">{f.error}</span>}
                        </div>
                      ))}
                      {mine.includes(t.name) && <button className="danger" onClick={() => { remove(t.name); setOpen(""); }}>删除此隧道</button>}
                    </div>
                  )}
                </div>
              );
            })}
            <button className="tile add" disabled={!status.running} onClick={() => setDialog(true)} title="增加隧道">＋</button>
          </div>
        </div>
      ))}

      {menu && (
        <div className="menu" style={{ position: "fixed", left: menu.x, top: menu.y }}>
          <button className="danger" onClick={() => remove(menu.name)}>删除隧道</button>
        </div>
      )}
      {dialog && <TunnelDialog onClose={() => { setDialog(false); load(); }} />}
    </>
  );
}
