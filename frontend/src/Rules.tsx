import { useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import type { Rule, Status } from "../bindings/github.com/ryanzhangtianran/Clash-One";

const ruleTypes = ["DOMAIN-SUFFIX", "DOMAIN", "DOMAIN-KEYWORD", "DOMAIN-REGEX", "DOMAIN-WILDCARD", "GEOSITE", "IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR", "GEOIP", "IP-ASN",
  "DST-PORT", "SRC-PORT", "PROCESS-NAME", "PROCESS-PATH", "PROCESS-ANCESTOR", "PROCESS-ANCESTOR-PATH", "IN-NAME", "IN-TYPE", "NETWORK", "RULE-SET"];
const ipTypes = new Set(["IP-CIDR", "IP-CIDR6", "GEOIP", "IP-ASN"]);

function AddRule({ onAdded }: { onAdded: () => void }) {
  const [type, setType] = useState("DOMAIN-SUFFIX");
  const [payload, setPayload] = useState("");
  const [policies, setPolicies] = useState<string[]>([]);
  const [policy, setPolicy] = useState("DIRECT");
  const [noResolve, setNoResolve] = useState(true);
  const [error, setError] = useState("");
  useEffect(() => { CoreService.Policies().then((p) => setPolicies(p ?? [])); }, []);
  const add = () => {
    setError("");
    const rule = [type, payload, policy, ...(ipTypes.has(type) && noResolve ? ["no-resolve"] : [])].join(",");
    ProfileService.AddRule(rule).then(() => { setPayload(""); onAdded(); }).catch((e) => setError(String(e)));
  };
  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <div className="label">增加规则（写入覆写，排在订阅规则之前）</div>
      <div className="form">
        <select value={type} onChange={(e) => setType(e.target.value)}>{ruleTypes.map((t) => <option key={t}>{t}</option>)}</select>
        <input placeholder="值，如 google.com、10.0.0.0/8、ghostty" value={payload} onChange={(e) => setPayload(e.target.value)} style={{ flex: 1 }} />
        <select value={policy} onChange={(e) => setPolicy(e.target.value)}>{policies.map((p) => <option key={p}>{p}</option>)}</select>
        {ipTypes.has(type) && <label className="radio"><input type="checkbox" checked={noResolve} onChange={(e) => setNoResolve(e.target.checked)} /> no-resolve</label>}
        <button className="btn primary" disabled={!payload} onClick={add}>增加</button>
      </div>
      {error && <div className="error">{error}</div>}
    </div>
  );
}

export default function Rules({ status }: { status: Status }) {
  const [rules, setRules] = useState<Rule[]>([]);
  const [filter, setFilter] = useState("");
  const [error, setError] = useState("");
  const [mine, setMine] = useState<string[]>([]);
  const load = () => {
    CoreService.Rules().then((r) => setRules(r ?? [])).catch((e) => setError(String(e)));
    ProfileService.UserItems().then((u) => setMine(u.rules ?? [])).catch(() => {});
  };

  useEffect(() => {
    if (!status.running) return;
    load();
    const t = setInterval(load, 3000); // hit counts move
    return () => clearInterval(t);
  }, [status.running]);

  const q = filter.toLowerCase();
  const shown = rules.filter((r) => !q || `${r.type} ${r.payload} ${r.proxy}`.toLowerCase().includes(q));
  const toggle = (r: Rule) => CoreService.SetRuleDisabled(r.index, !r.disabled).then(load).catch((e) => setError(String(e)));

  return (
    <>
      <h1 className="page-title">规则</h1>
      {error && <div className="card error" style={{ marginBottom: 16 }}>{error}</div>}
      {status.running && <AddRule onAdded={load} />}
      {mine.length > 0 && (
        <div className="card" style={{ marginBottom: 16 }}>
          <div className="label">我添加的规则</div>
          {mine.map((r) => (
            <div key={r} className="profile-row">
              <code>{r}</code>
              <button className="btn" onClick={() => ProfileService.RemoveRule(r).then(load).catch((e) => setError(String(e)))}>删除</button>
            </div>
          ))}
        </div>
      )}
      <div className="card">
        <div className="head">
          <div className="label">共 {rules.length} 条（停用仅到下次重载配置）</div>
          <input placeholder="搜索类型、内容、策略…" value={filter} onChange={(e) => setFilter(e.target.value)} style={{ width: 240 }} />
        </div>
        <div className="table-wrap">
          <table className="table">
            <thead><tr><th>#</th><th>类型</th><th>内容</th><th>策略</th><th>命中</th><th /></tr></thead>
            <tbody>
              {shown.map((r) => (
                <tr key={r.index} className={r.disabled ? "muted" : ""}>
                  <td>{r.index}</td>
                  <td>{r.type}</td>
                  <td title={r.payload}>{r.payload}</td>
                  <td>{r.proxy}</td>
                  <td>{r.hits}</td>
                  <td><button className="btn" onClick={() => toggle(r)}>{r.disabled ? "启用" : "停用"}</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}
