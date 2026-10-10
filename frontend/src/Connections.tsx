import { useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import type { Conn, Status, TempRule } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import { sizeText } from "./format";

const ruleTypes = ["DOMAIN-SUFFIX", "DOMAIN", "DOMAIN-KEYWORD", "IP-CIDR", "PROCESS-NAME", "PROCESS-ANCESTOR"];
const isIP = (h: string) => /^[\d.]+$/.test(h) || h.includes(":");

// suggest the registrable part of a host, e.g. translate.google.com -> google.com (naive: last two labels)
function suggest(c: Conn, type: string): string {
  if (type === "PROCESS-NAME" || type === "PROCESS-ANCESTOR") return c.process;
  if (type === "IP-CIDR") return isIP(c.host) ? `${c.host}/${c.host.includes(":") ? 128 : 32}` : "";
  if (type === "DOMAIN-SUFFIX") return c.host.split(".").slice(-2).join(".");
  return c.host;
}

function age(start: string) {
  const s = Math.max(0, Math.round((Date.now() - Date.parse(start)) / 1000));
  return s < 60 ? `${s}s` : s < 3600 ? `${Math.floor(s / 60)}m` : `${Math.floor(s / 3600)}h`;
}

function RuleDialog({ conn, onClose }: { conn: Conn; onClose: () => void }) {
  const [type, setType] = useState(isIP(conn.host) ? "IP-CIDR" : "DOMAIN-SUFFIX");
  const [value, setValue] = useState(suggest(conn, type));
  const [policies, setPolicies] = useState<string[]>([]);
  const [policy, setPolicy] = useState("DIRECT");
  const [permanent, setPermanent] = useState(false);
  const [reconnect, setReconnect] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => { CoreService.Policies().then((p) => setPolicies(p ?? [])); }, []);

  const add = async () => {
    setError("");
    const rule = `${type},${value},${policy}`;
    try {
      if (permanent) await ProfileService.AddRule(rule);
      else await CoreService.AddTempRule(rule);
      if (reconnect) await CoreService.CloseHost(type.startsWith("DOMAIN") ? value : conn.host);
      onClose();
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal card" onClick={(e) => e.stopPropagation()}>
        <div className="label">为 {conn.host} 添加规则</div>
        <div className="form-grid">
          <label>规则类型</label>
          <select value={type} onChange={(e) => { setType(e.target.value); setValue(suggest(conn, e.target.value)); }}>
            {ruleTypes.map((t) => <option key={t}>{t}</option>)}
          </select>
          <label>值</label>
          <input value={value} onChange={(e) => setValue(e.target.value)} />
          <label>策略</label>
          <select value={policy} onChange={(e) => setPolicy(e.target.value)}>
            {policies.map((p) => <option key={p}>{p}</option>)}
          </select>
          <label />
          <div>
            <label className="radio"><input type="radio" checked={!permanent} onChange={() => setPermanent(false)} /> 临时规则</label>
            <label className="radio"><input type="radio" checked={permanent} onChange={() => setPermanent(true)} /> 永久规则（写入覆写）</label>
            <div className="muted small">临时规则不会写入配置，重载配置后消失。</div>
            <label className="radio"><input type="checkbox" checked={reconnect} onChange={(e) => setReconnect(e.target.checked)} /> 断开现有连接以立即生效</label>
          </div>
        </div>
        {error && <div className="error">{error}</div>}
        <div className="actions" style={{ justifyContent: "flex-end", marginTop: 12 }}>
          <button className="btn" onClick={onClose}>取消</button>
          <button className="btn primary" disabled={!value} onClick={add}>增加</button>
        </div>
      </div>
    </div>
  );
}

export default function Connections({ status }: { status: Status }) {
  const [conns, setConns] = useState<Conn[]>([]);
  const [temp, setTemp] = useState<TempRule[]>([]);
  const [filter, setFilter] = useState("");
  const [dialog, setDialog] = useState<Conn | null>(null);

  useEffect(() => {
    if (!status.running) return;
    const poll = () => {
      CoreService.Connections().then((c) => setConns(c ?? [])).catch(() => {});
      CoreService.TempRules().then((r) => setTemp(r ?? [])).catch(() => {});
    };
    poll();
    const t = setInterval(poll, 1000);
    return () => clearInterval(t);
  }, [status.running]);

  const q = filter.toLowerCase();
  const shown = conns.filter((c) => !q || `${c.process} ${c.host} ${c.rule} ${c.chain}`.toLowerCase().includes(q));

  return (
    <>
      <h1 className="page-title">连接</h1>
      {temp.length > 0 && (
        <div className="card" style={{ marginBottom: 16 }}>
          <div className="label">临时规则（优先于配置规则，重载配置后消失）</div>
          {temp.map((r) => (
            <div key={r.id} className="profile-row">
              <span>{r.type},{r.payload} → <b>{r.proxy}</b> <span className="muted small">命中 {r.hits}</span></span>
              <button className="btn" onClick={() => CoreService.RemoveTempRule(r.id)}>删除</button>
            </div>
          ))}
        </div>
      )}
      <div className="card">
        <div className="head">
          <div className="label">活动连接 {shown.length}</div>
          <input placeholder="搜索进程、主机、规则…" value={filter} onChange={(e) => setFilter(e.target.value)} style={{ width: 240 }} />
        </div>
        <div className="table-wrap">
          <table className="table">
            <thead><tr><th>进程</th><th>主机</th><th>规则</th><th>策略</th><th>上传</th><th>下载</th><th>时长</th><th /></tr></thead>
            <tbody>
              {shown.map((c) => (
                <tr key={c.id}>
                  <td title={c.path}>{c.process || c.source}<span className="muted small"> {c.inbound}</span></td>
                  <td title={`${c.network} ${c.host}:${c.port}`}>{c.host}:{c.port}</td>
                  <td>{c.rule}</td>
                  <td>{c.chain}</td>
                  <td>{sizeText(c.upload)}</td>
                  <td>{sizeText(c.download)}</td>
                  <td>{age(c.start)}</td>
                  <td className="actions">
                    <button className="btn" onClick={() => setDialog(c)}>规则</button>
                    <button className="btn" onClick={() => CoreService.CloseConnection(c.id)}>断开</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      {dialog && <RuleDialog conn={dialog} onClose={() => setDialog(null)} />}
    </>
  );
}
