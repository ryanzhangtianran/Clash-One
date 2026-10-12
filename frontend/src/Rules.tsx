import { useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import type { Rule, Status } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import { usePoll } from "./hooks";

const ruleTypes = ["DOMAIN-SUFFIX", "DOMAIN", "DOMAIN-KEYWORD", "DOMAIN-REGEX", "DOMAIN-WILDCARD", "GEOSITE", "IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR", "GEOIP", "IP-ASN",
  "DST-PORT", "SRC-PORT", "PROCESS-NAME", "PROCESS-PATH", "PROCESS-ANCESTOR", "PROCESS-ANCESTOR-PATH", "IN-NAME", "IN-TYPE", "NETWORK", "RULE-SET"];
const ipTypes = new Set(["IP-CIDR", "IP-CIDR6", "GEOIP", "IP-ASN"]);

function AddRule({ onAdded }: { onAdded: () => void }) {
  const [type, setType] = useState("DOMAIN-SUFFIX");
  const [payload, setPayload] = useState("");
  const [policies, setPolicies] = useState<string[]>([]);
  const [policy, setPolicy] = useState("DIRECT");
  const [noResolve, setNoResolve] = useState(true);
  useEffect(() => { CoreService.Policies().then((p) => setPolicies(p ?? [])); }, []);
  const add = () => {
    const rule = [type, payload, policy, ...(ipTypes.has(type) && noResolve ? ["no-resolve"] : [])].join(",");
    ProfileService.AddRule(rule).then(() => { setPayload(""); onAdded(); }).catch(() => {});
  };
  return (
    <div className="card" style={{ marginBottom: 16 }}>
      <div className="label">增加规则</div>
      <div className="form">
        <select value={type} onChange={(e) => setType(e.target.value)}>{ruleTypes.map((t) => <option key={t}>{t}</option>)}</select>
        <input placeholder="值，如 google.com、10.0.0.0/8、ghostty" value={payload} onChange={(e) => setPayload(e.target.value)} className="grow" />
        <select value={policy} onChange={(e) => setPolicy(e.target.value)}>{policies.map((p) => <option key={p}>{p}</option>)}</select>
        {ipTypes.has(type) && <label className="radio"><input type="checkbox" checked={noResolve} onChange={(e) => setNoResolve(e.target.checked)} /> no-resolve</label>}
        <button className="btn primary" disabled={!payload} onClick={add}>增加</button>
      </div>
    </div>
  );
}

export default function Rules({ status }: { status: Status }) {
  const [rules, setRules] = useState<Rule[]>([]);
  const [filter, setFilter] = useState("");
  const [mine, setMine] = useState(0); // the first `mine` rules are the user's own (prepend-rules), the only ones that move
  const load = () => {
    CoreService.Rules().then((r) => setRules(r ?? [])).catch(() => {}); // polling: a lost core shows up in the events, not here
    ProfileService.UserItems().then((u) => setMine(u.rules?.length ?? 0)).catch(() => {});
  };

  usePoll(load, 3000, status.running); // hit counts move

  const q = filter.toLowerCase();
  const shown = rules.filter((r) => !q || `${r.type} ${r.payload} ${r.proxy}`.toLowerCase().includes(q));
  const run = (p: Promise<unknown>) => { p.then(load).catch(() => {}); };
  const toggle = (r: Rule) => run(CoreService.SetRuleDisabled(r.index, !r.disabled));

  return (
    <div className="rules-page">
      {status.running && <AddRule onAdded={load} />}
      <div className="card rules-card">
        <div className="head">
          <div className="label">共 {rules.length} 条</div>
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
                  <td className="rule-actions">
                    {r.index < mine && <>
                      <button className="btn" title="上移" disabled={r.index === 0} onClick={() => run(ProfileService.MoveRule(r.index, -1))}>↑</button>
                      <button className="btn" title="下移" disabled={r.index === mine - 1} onClick={() => run(ProfileService.MoveRule(r.index, 1))}>↓</button>
                    </>}
                    <button className="btn" onClick={() => toggle(r)}>{r.disabled ? "启用" : "停用"}</button>
                    <button className="btn danger" disabled={r.type === "Match" && r.index >= mine} title={r.type === "Match" ? "兜底规则不能删除" : ""} onClick={() => run(ProfileService.DeleteRule(r.index))}>删除</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
