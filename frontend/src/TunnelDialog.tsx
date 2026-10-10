import { useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";

type Row = Record<string, string>;

function Rows({ cols, rows, setRows }: { cols: [string, string][]; rows: Row[]; setRows: (r: Row[]) => void }) {
  const update = (i: number, k: string, value: string) => setRows(rows.map((r, j) => (j === i ? { ...r, [k]: value } : r)));
  return (
    <table className="table">
      <thead><tr>{cols.map(([k, l]) => <th key={k}>{l}</th>)}<th /></tr></thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={i}>
            {cols.map(([k]) => (
              <td key={k}>
                {k === "type"
                  ? <select value={r[k] ?? "tcp"} onChange={(e) => update(i, k, e.target.value)}>{["tcp", "udp", "http", "https", "stcp"].map((t) => <option key={t}>{t}</option>)}</select>
                  : <input value={r[k] ?? ""} onChange={(e) => update(i, k, e.target.value)} style={{ width: "100%" }} />}
              </td>
            ))}
            <td><button className="btn" onClick={() => setRows(rows.filter((_, j) => j !== i))}>×</button></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

const num = (s: string) => (s && !isNaN(Number(s)) ? Number(s) : undefined);
const clean = (o: Record<string, any>) => Object.fromEntries(Object.entries(o).filter(([, v]) => v !== undefined && v !== ""));

export default function TunnelDialog({ onClose }: { onClose: () => void }) {
  const [kind, setKind] = useState<"frp" | "ssh">("frp");
  const [v, setV] = useState<Record<string, string>>({ serverPort: "7000", port: "22", protocol: "tcp" });
  const [frpRows, setFrpRows] = useState<Row[]>([{ name: "web", type: "tcp", localIP: "127.0.0.1", localPort: "8080", remotePort: "18080" }]);
  const [sshRows, setSshRows] = useState<Row[]>([{ remote: "0.0.0.0:8080", local: "127.0.0.1:80" }]);
  const [names, setNames] = useState<string[]>([]);
  const [error, setError] = useState("");
  useEffect(() => { CoreService.ProxyNames().then((n) => setNames(n ?? [])); }, []);
  const field = (k: string, placeholder = "", type = "text") => <input type={type} placeholder={placeholder} value={v[k] ?? ""} onChange={(e) => setV({ ...v, [k]: e.target.value })} />;

  const build = (): Record<string, any> => {
    const base = clean({ name: v.name, type: kind, "dialer-proxy": v.dialer });
    if (kind === "frp") {
      return {
        ...base,
        config: clean({
          serverAddr: v.serverAddr,
          serverPort: num(v.serverPort),
          auth: v.token ? { token: v.token } : undefined,
          transport: v.protocol !== "tcp" ? { protocol: v.protocol } : undefined,
          proxies: frpRows.map((r) => clean({
            name: r.name, type: r.type || "tcp", localIP: r.localIP, localPort: num(r.localPort),
            remotePort: num(r.remotePort), customDomains: r.customDomains ? r.customDomains.split(",").map((d) => d.trim()) : undefined,
          })),
        }),
      };
    }
    return { ...base, ...clean({ server: v.server, port: num(v.port), username: v.username, password: v.password, "private-key": v.key, "host-key": v.hostKey ? [v.hostKey] : undefined }), forwards: sshRows };
  };

  const add = () => {
    setError("");
    ProfileService.AddTunnel(build()).then(onClose).catch((e) => setError(String(e)));
  };

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal card wide" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">增加反向隧道</h2>
        <div className="seg" style={{ marginBottom: 8 }}>
          <button className={kind === "frp" ? "on" : ""} onClick={() => setKind("frp")}>frp</button>
          <button className={kind === "ssh" ? "on" : ""} onClick={() => setKind("ssh")}>SSH -R</button>
        </div>
        <div className="form-grid">
          <label>名称</label>{field("name", "必填")}
          {kind === "frp" ? (
            <>
              <label>frps 地址</label><div className="inline">{field("serverAddr", "frps.example.com")} : <span style={{ width: 90 }}>{field("serverPort", "7000")}</span></div>
              <label>Token</label>{field("token", "auth.token，可选", "password")}
              <label>传输协议</label>
              <select value={v.protocol} onChange={(e) => setV({ ...v, protocol: e.target.value })}>{["tcp", "websocket", "wss", "quic", "kcp"].map((p) => <option key={p}>{p}</option>)}</select>
            </>
          ) : (
            <>
              <label>SSH 服务器</label><div className="inline">{field("server", "vps.example.com")} : <span style={{ width: 90 }}>{field("port", "22")}</span></div>
              <label>用户名</label>{field("username", "必填")}
              <label>密码</label>{field("password", "密码或私钥二选一", "password")}
              <label>私钥</label><textarea className="code" rows={3} placeholder="私钥内容（增强模式下内核无法读取用户目录中的文件）" value={v.key ?? ""} onChange={(e) => setV({ ...v, key: e.target.value })} />
              <label>Host Key</label>{field("hostKey", "ssh-ed25519 AAAA…，建议填写")}
            </>
          )}
          <label>跳板代理</label>
          <select value={v.dialer ?? ""} onChange={(e) => setV({ ...v, dialer: e.target.value })}>
            <option value="">直连</option>
            {names.map((n) => <option key={n}>{n}</option>)}
          </select>
        </div>
        <div className="section-title">{kind === "frp" ? "frp 代理" : "转发（远端监听 → 本地目标）"}</div>
        {kind === "frp"
          ? <Rows cols={[["name", "名称"], ["type", "类型"], ["localIP", "本地 IP"], ["localPort", "本地端口"], ["remotePort", "远端端口"], ["customDomains", "域名（http/https）"]]} rows={frpRows} setRows={setFrpRows} />
          : <Rows cols={[["remote", "远端监听"], ["local", "本地目标"]]} rows={sshRows} setRows={setSshRows} />}
        <button className="btn" style={{ marginTop: 8 }} onClick={() => (kind === "frp" ? setFrpRows([...frpRows, { type: "tcp", localIP: "127.0.0.1" }]) : setSshRows([...sshRows, {}]))}>增加一行</button>
        <div className="muted small" style={{ marginTop: 8 }}>反代进来的流量按规则分流，本地服务通常需要一条 IN-NAME,{v.name || "隧道名"},DIRECT 规则。</div>
        {error && <div className="error">{error}</div>}
        <div className="actions" style={{ justifyContent: "flex-end", marginTop: 12 }}>
          <button className="btn" onClick={onClose}>取消</button>
          <button className="btn primary" disabled={!v.name} onClick={add}>完成</button>
        </div>
      </div>
    </div>
  );
}
