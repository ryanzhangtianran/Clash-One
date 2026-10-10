import { useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";

const types = [
  ["select", "手动选择"],
  ["url-test", "自动测速（最低延迟）"],
  ["fallback", "故障转移"],
  ["load-balance", "负载均衡"],
];

export default function GroupDialog({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState("");
  const [type, setType] = useState("select");
  const [names, setNames] = useState<string[]>([]);
  const [members, setMembers] = useState<string[]>([]);
  const [url, setURL] = useState("https://www.gstatic.com/generate_204");
  const [period, setPeriod] = useState("300");
  const [strategy, setStrategy] = useState("consistent-hashing");
  const [error, setError] = useState("");
  useEffect(() => { CoreService.ProxyNames().then((n) => setNames([...(n ?? []), "DIRECT", "REJECT"])); }, []);

  const add = () => {
    const group: Record<string, any> = { name, type, proxies: members };
    if (type !== "select") Object.assign(group, { url, interval: Number(period) });
    if (type === "load-balance") group.strategy = strategy;
    ProfileService.AddGroup(group).then(onClose).catch((e) => setError(String(e)));
  };

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal card" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">增加策略组</h2>
        <div className="form-grid">
          <label>名称</label><input autoFocus placeholder="必填" value={name} onChange={(e) => setName(e.target.value)} />
          <label>类型</label>
          <select value={type} onChange={(e) => setType(e.target.value)}>{types.map(([k, l]) => <option key={k} value={k}>{l}</option>)}</select>
          {type !== "select" && <><label>测试 URL</label><input value={url} onChange={(e) => setURL(e.target.value)} /></>}
          {type !== "select" && <><label>间隔（秒）</label><input value={period} onChange={(e) => setPeriod(e.target.value)} /></>}
          {type === "load-balance" && (
            <><label>策略</label><select value={strategy} onChange={(e) => setStrategy(e.target.value)}>{["consistent-hashing", "round-robin", "sticky-sessions"].map((s) => <option key={s}>{s}</option>)}</select></>
          )}
        </div>
        <div className="section-title">成员（按顺序）</div>
        <div className="checks scroll">
          {names.map((n) => (
            <label key={n} className="radio"><input type="checkbox" checked={members.includes(n)} onChange={(e) => setMembers(e.target.checked ? [...members, n] : members.filter((x) => x !== n))} /> {n}</label>
          ))}
        </div>
        {error && <div className="error">{error}</div>}
        <div className="actions" style={{ justifyContent: "flex-end", marginTop: 12 }}>
          <button className="btn" onClick={onClose}>取消</button>
          <button className="btn primary" disabled={!name || members.length === 0} onClick={add}>完成</button>
        </div>
      </div>
    </div>
  );
}
