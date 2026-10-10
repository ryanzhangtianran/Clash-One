import { useCallback, useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import ProxyDialog from "./ProxyDialog";
import { nonNull } from "./format";
import GroupDialog from "./GroupDialog";
import type { Group, Status } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import type { UserItems } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/profile";

const selectable = new Set(["Selector"]); // url-test / fallback / load-balance pick by themselves

function delayClass(ms: number) {
  if (ms <= 0) return "";
  return ms < 300 ? "delay-good" : ms < 800 ? "delay-warn" : "delay-bad";
}

export default function Proxies({ status }: { status: Status }) {
  const [groups, setGroups] = useState<Group[]>([]);
  const [testing, setTesting] = useState<string>("");
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<"proxy" | "group" | "">("");
  const [mine, setMine] = useState<UserItems | null>(null);
  const [delays, setDelays] = useState<Record<string, string>>({});
  const [bandwidth, setBandwidth] = useState<Record<string, string>>({});
  const [measuring, setMeasuring] = useState("");

  const load = useCallback(() => {
    CoreService.Groups().then((g) => { setGroups(g ?? []); setError(""); }).catch((e) => setError(String(e)));
    ProfileService.UserItems().then(setMine).catch(() => {});
  }, []);
  const remove = (action: Promise<void>) => action.then(load).catch((e) => setError(String(e)));
  // bandwidth tests run one at a time (they share a temporary rule)
  const speed = (name: string) => {
    if (measuring) return;
    setMeasuring(name);
    setBandwidth((b) => ({ ...b, [name]: "测速中…" }));
    CoreService.SpeedTest(name)
      .then((r) => setBandwidth((b) => ({ ...b, [name]: `${r.mbps.toFixed(1)} Mbps` })))
      .catch((e) => { setBandwidth((b) => ({ ...b, [name]: "失败" })); setError(String(e)); })
      .finally(() => setMeasuring(""));
  };
  const testOne = (name: string) => {
    setDelays((d) => ({ ...d, [name]: "…" }));
    CoreService.TestProxy(name).then((ms) => setDelays((d) => ({ ...d, [name]: `${ms} ms` }))).catch(() => setDelays((d) => ({ ...d, [name]: "超时" })));
  };

  useEffect(() => {
    if (status.running) load();
  }, [status.running, load]);

  const select = (group: Group, name: string) => {
    if (!selectable.has(group.type) || group.now === name) return;
    CoreService.Select(group.name, name).then(load).catch((e) => setError(String(e)));
  };

  const test = (group: Group) => {
    setTesting(group.name);
    CoreService.TestGroup(group.name).catch((e) => setError(String(e))).finally(() => { setTesting(""); load(); });
  };

  return (
    <>
      <h1 className="page-title">策略</h1>
      <div className="toolbar">
        <button className="btn" disabled={!status.running} onClick={() => setDialog("proxy")}>增加代理</button>
        <button className="btn" disabled={!status.running} onClick={() => setDialog("group")}>增加策略组</button>
      </div>
      {mine && ((mine.proxies?.length ?? 0) > 0 || (mine.groups?.length ?? 0) > 0) && (
        <div className="card group-card">
          <h3>我添加的 <span className="muted">保存在覆写中，订阅更新不影响</span></h3>
          {nonNull(mine.proxies).map((p) => (
            <div key={"p" + p.name} className="profile-row">
              <span><b>{p.name}</b> <span className="muted small">{String(p.type)} · {String(p.server)}:{String(p.port)}</span></span>
              <span className="actions">
                <span className="muted small">{delays[p.name]}</span>
                <span className="muted small">{bandwidth[p.name]}</span>
                <button className="btn" onClick={() => testOne(p.name)}>测速</button>
                <button className="btn" disabled={!!measuring} onClick={() => speed(p.name)}>测带宽</button>
                <button className="btn" onClick={() => remove(ProfileService.RemoveProxy(p.name))}>删除</button>
              </span>
            </div>
          ))}
          {nonNull(mine.groups).map((g) => (
            <div key={"g" + g.name} className="profile-row">
              <span><b>{g.name}</b> <span className="muted small">策略组 · {String(g.type)} · {(g.proxies ?? []).join("、")}</span></span>
              <button className="btn" onClick={() => remove(ProfileService.RemoveGroup(g.name))}>删除</button>
            </div>
          ))}
        </div>
      )}
      {error && <div className="card error">{error}</div>}
      {!error && groups.length === 0 && <div className="card muted">暂无策略组</div>}
      {groups.map((g) => (
        <div className="card group-card" key={g.name}>
          <h3>
            {g.name}
            <span className="muted">{g.type} · {g.now}</span>
            <span className="grow" />
            {bandwidth[g.now] && <span className="muted small">{g.now}：{bandwidth[g.now]}</span>}
            <button className="btn" disabled={testing !== ""} onClick={() => test(g)}>
              {testing === g.name ? "测速中…" : "测速"}
            </button>
            <button className="btn" disabled={!!measuring} title="测当前节点的下载带宽（约 10 秒）；右键任意节点可单独测" onClick={() => speed(g.now)}>
              {measuring === g.now ? "测带宽中…" : "测带宽"}
            </button>
          </h3>
          <div className="nodes">
            {(g.all ?? []).map((p) => (
              <button key={p.name} className={`node ${p.name === g.now ? "selected" : ""}`} onClick={() => select(g, p.name)}
                onContextMenu={(e) => { e.preventDefault(); speed(p.name); }} title="右键测带宽">
                <span className="name">{p.name}</span>
                <span className="meta">
                  <span>{p.type}</span>
                  <span className={delayClass(p.delay)}>{p.delay > 0 ? `${p.delay} ms` : "—"}</span>
                </span>
                {bandwidth[p.name] && <span className="meta"><span>带宽</span><span>{bandwidth[p.name]}</span></span>}
              </button>
            ))}
          </div>
        </div>
      ))}
      {dialog === "proxy" && <ProxyDialog groups={groups.filter((g) => g.type === "Selector" && g.name !== "GLOBAL").map((g) => g.name)} onClose={() => { setDialog(""); load(); }} />}
      {dialog === "group" && <GroupDialog onClose={() => { setDialog(""); load(); }} />}
    </>
  );
}
