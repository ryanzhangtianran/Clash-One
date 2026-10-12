import { useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";

const types = [
  ["select", "手动选择"],
  ["url-test", "自动测速（最低延迟）"],
  ["fallback", "故障转移"],
  ["load-balance", "负载均衡"],
  ["subnet", "Subnet（按网络自动选择）"],
];

export default function GroupDialog({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState("");
  const [type, setType] = useState("select");
  const [names, setNames] = useState<string[]>([]);
  const [members, setMembers] = useState<string[]>([]);
  const [url, setURL] = useState("https://www.gstatic.com/generate_204");
  const [period, setPeriod] = useState("300");
  const [strategy, setStrategy] = useState("consistent-hashing");
  const [def, setDef] = useState("DIRECT");
  const [rules, setRules] = useState<[string, string][]>([["SSID:", "DIRECT"]]);
  useEffect(() => { CoreService.ProxyNames().then((n) => setNames([...(n ?? []), "DIRECT", "REJECT"])); }, []);

  const probes = type !== "select" && type !== "subnet"; // url-test, fallback, load-balance
  const add = () => {
    const group: Record<string, any> = type === "subnet"
      ? { name, type, default: def, rules: Object.fromEntries(rules.filter(([c]) => c.trim() && !c.trim().endsWith(":"))) }
      : { name, type, proxies: members };
    if (probes) Object.assign(group, { url, interval: Number(period) });
    if (type === "load-balance") group.strategy = strategy;
    ProfileService.AddGroup(group).then(onClose);
  };

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal card" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">增加策略组</h2>
        <div className="form-grid">
          <label>名称</label><input autoFocus placeholder="必填" value={name} onChange={(e) => setName(e.target.value)} />
          <label>类型</label>
          <select value={type} onChange={(e) => setType(e.target.value)}>{types.map(([k, l]) => <option key={k} value={k}>{l}</option>)}</select>
          {probes && <><label>测试 URL</label><input value={url} onChange={(e) => setURL(e.target.value)} /><label>间隔（秒）</label><input value={period} onChange={(e) => setPeriod(e.target.value)} /></>}
          {type === "subnet" && <><label>默认</label><select value={def} onChange={(e) => setDef(e.target.value)}>{names.map((n) => <option key={n}>{n}</option>)}</select></>}
          {type === "load-balance" && (
            <><label>策略</label><select value={strategy} onChange={(e) => setStrategy(e.target.value)}>{["consistent-hashing", "round-robin", "sticky-sessions"].map((s) => <option key={s}>{s}</option>)}</select></>
          )}
        </div>
        {type === "subnet" ? (
          <>
            <div className="section-title">规则（SSID:名称、BSSID:地址、ROUTER:路由器 IP、网段如 192.168.1.0/24、TYPE:WIFI / TYPE:WIRED）</div>
            {rules.map(([cond, target], i) => (
              <div key={i} className="inline" style={{ marginBottom: 6 }}>
                <input value={cond} placeholder="SSID:我的 Wi-Fi" onChange={(e) => setRules(rules.map((r, j) => (j === i ? [e.target.value, r[1]] : r)))} className="grow" />
                <span className="muted">→</span>
                <select value={target} onChange={(e) => setRules(rules.map((r, j) => (j === i ? [r[0], e.target.value] : r)))}>{names.map((n) => <option key={n}>{n}</option>)}</select>
                <button className="btn" onClick={() => setRules(rules.filter((_, j) => j !== i))}>×</button>
              </div>
            ))}
            <button className="btn" onClick={() => setRules([...rules, ["SSID:", "DIRECT"]])}>增加一条</button>
          </>
        ) : (
          <>
            <div className="section-title">成员（按顺序）</div>
            <div className="checks scroll">
              {names.map((n) => (
                <label key={n} className="radio"><input type="checkbox" checked={members.includes(n)} onChange={(e) => setMembers(e.target.checked ? [...members, n] : members.filter((x) => x !== n))} /> {n}</label>
              ))}
            </div>
          </>
        )}
        <div className="actions end">
          <button className="btn" onClick={onClose}>取消</button>
          <button className="btn primary" disabled={!name || (type !== "subnet" && members.length === 0)} onClick={add}>完成</button>
        </div>
      </div>
    </div>
  );
}
