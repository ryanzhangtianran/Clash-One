import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import type { Group, Proxy, Status } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import ProxyDialog from "./ProxyDialog";
import GroupDialog from "./GroupDialog";
import { nonNull } from "./format";

const modes = [
  { key: "direct", label: "直接连接", hint: "所有请求都将被直接发往目标服务器。" },
  { key: "global", label: "全局代理", hint: "所有请求都将被转发给 GLOBAL 中选中的代理。" },
  { key: "rule", label: "规则判定", hint: "根据规则系统判定处理方式。" },
];
const builtin = new Set(["Direct", "Reject", "RejectDrop", "Compatible", "Pass", "Dns"]);
const groupTypes: Record<string, string> = { Selector: "手动选择", URLTest: "自动测速", Fallback: "故障转移", LoadBalance: "负载均衡", Relay: "链式" };

function delayClass(ms: number) {
  if (ms <= 0) return "";
  return ms < 300 ? "delay-good" : ms < 800 ? "delay-warn" : "delay-bad";
}

type Menu = { x: number; y: number; name: string; mine: "proxy" | "group" | "" };

export default function Proxies({ status }: { status: Status }) {
  const [mode, setMode] = useState("");
  const [groups, setGroups] = useState<Group[]>([]);
  const [mineProxies, setMineProxies] = useState<string[]>([]);
  const [mineGroups, setMineGroups] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [testing, setTesting] = useState(false);
  const [bandwidth, setBandwidth] = useState<Record<string, string>>({});
  const [measuring, setMeasuring] = useState("");
  const [dialog, setDialog] = useState<"proxy" | "group" | "">("");
  const [open, setOpen] = useState(""); // group whose members are shown
  const [menu, setMenu] = useState<Menu | null>(null);

  const load = useCallback(() => {
    CoreService.Groups().then((g) => { setGroups(g ?? []); setError(""); }).catch((e) => setError(String(e)));
    ProfileService.UserItems().then((u) => {
      setMineProxies(nonNull(u.proxies).map((p) => String(p.name)));
      setMineGroups(nonNull(u.groups).map((g) => String(g.name)));
    }).catch(() => {});
  }, []);

  useEffect(() => {
    if (!status.running) return;
    load();
    CoreService.Mode().then(setMode).catch(() => {});
    return Events.On("mode", (e) => setMode(e.data));
  }, [status.running, load]);

  useEffect(() => {
    const close = () => setMenu(null);
    window.addEventListener("click", close);
    return () => window.removeEventListener("click", close);
  }, []);

  // nodes: every non-group member across groups, built-ins (DIRECT, REJECT…) left out
  const groupNames = new Set(groups.map((g) => g.name));
  const nodes: Proxy[] = [];
  for (const g of groups) for (const p of g.all ?? []) {
    if (!groupNames.has(p.name) && !builtin.has(p.type) && !nodes.some((n) => n.name === p.name)) nodes.push(p);
  }
  const shownGroups = groups.filter((g) => g.name !== "GLOBAL");
  const global = groups.find((g) => g.name === "GLOBAL");

  const choose = (m: string) => CoreService.SetMode(m).catch((e) => setError(String(e)));
  const select = (group: string, name: string) => CoreService.Select(group, name).then(load).catch((e) => setError(String(e)));
  const testAll = () => {
    setTesting(true);
    CoreService.TestGroup("GLOBAL").catch((e) => setError(String(e))).finally(() => { setTesting(false); load(); });
  };
  const testOne = (name: string) => CoreService.TestProxy(name).catch(() => 0).then(load);
  const speed = (name: string) => {
    if (measuring) return;
    setMeasuring(name);
    setBandwidth((b) => ({ ...b, [name]: "测带宽中…" }));
    CoreService.SpeedTest(name)
      .then((r) => setBandwidth((b) => ({ ...b, [name]: `${r.mbps.toFixed(1)} Mbps` })))
      .catch((e) => { setBandwidth((b) => ({ ...b, [name]: "失败" })); setError(String(e)); })
      .finally(() => setMeasuring(""));
  };
  const contextMenu = (e: React.MouseEvent, name: string, mine: Menu["mine"]) => {
    e.preventDefault();
    setMenu({ x: e.clientX, y: e.clientY, name, mine });
  };

  const hint = modes.find((m) => m.key === mode)?.hint;

  return (
    <>
      <h1 className="page-title">策略</h1>
      <div className="seg big-seg">
        {modes.map((m) => (
          <button key={m.key} className={m.key === mode ? "on accent" : ""} disabled={!status.running} onClick={() => choose(m.key)}>{m.label}</button>
        ))}
      </div>
      {hint && <div className="hint">{hint}</div>}
      {error && <div className="error" style={{ marginTop: 10 }}>{error}</div>}

      <div className="tile-head">
        <span className="tile-title proxies">代理</span>
        <button className="btn small-btn" disabled={testing || !status.running} onClick={testAll}>{testing ? "测试中…" : "测试全部"}</button>
      </div>
      <div className="tiles">
        {nodes.map((p) => {
          const selected = mode === "global" && global?.now === p.name;
          return (
            <button key={p.name} className={`tile ${selected ? "selected" : ""}`} title={mode === "global" ? "点击设为全局代理；右键更多操作" : "右键更多操作"}
              onClick={() => mode === "global" && select("GLOBAL", p.name)}
              onContextMenu={(e) => contextMenu(e, p.name, mineProxies.includes(p.name) ? "proxy" : "")}>
              <span className="kind">{p.type}</span>
              <span className="name">{p.name}</span>
              <span className={`foot ${delayClass(p.delay)}`}>{bandwidth[p.name] ?? (p.delay > 0 ? `${p.delay} ms` : "")}</span>
            </button>
          );
        })}
        <button className="tile add" disabled={!status.running} onClick={() => setDialog("proxy")} title="增加代理">＋</button>
      </div>

      <div className="tile-head"><span className="tile-title groups">策略组</span></div>
      <div className="tiles">
        {shownGroups.map((g) => (
          <div key={g.name} className="tile-wrap">
            <button className={`tile ${open === g.name ? "selected" : ""}`} onClick={() => setOpen(open === g.name ? "" : g.name)}
              onContextMenu={(e) => contextMenu(e, g.now, mineGroups.includes(g.name) ? "group" : "")}>
              <span className="kind">{groupTypes[g.type] ?? g.type}</span>
              <span className="name">{g.name}</span>
              <span className="foot">{g.now}</span>
            </button>
            {open === g.name && (
              <div className="menu members" onClick={(e) => e.stopPropagation()}>
                {(g.all ?? []).map((p) => (
                  <button key={p.name} disabled={g.type !== "Selector"} className={p.name === g.now ? "current" : ""}
                    onClick={() => { select(g.name, p.name); setOpen(""); }}>
                    <span>{p.name === g.now ? "✓ " : ""}{p.name}</span>
                    <span className={delayClass(p.delay)}>{p.delay > 0 ? `${p.delay} ms` : ""}</span>
                  </button>
                ))}
                {mineGroups.includes(g.name) && (
                  <button className="danger" onClick={() => ProfileService.RemoveGroup(g.name).then(load).catch((e) => setError(String(e)))}>删除此策略组</button>
                )}
              </div>
            )}
          </div>
        ))}
        <button className="tile add" disabled={!status.running} onClick={() => setDialog("group")} title="增加策略组">＋</button>
      </div>

      {menu && (
        <div className="menu" style={{ position: "fixed", left: menu.x, top: menu.y }}>
          <button onClick={() => testOne(menu.name)}>测试延迟</button>
          <button disabled={!!measuring} onClick={() => speed(menu.name)}>测带宽（约 10 秒）</button>
          {menu.mine === "proxy" && <button className="danger" onClick={() => ProfileService.RemoveProxy(menu.name).then(load).catch((e) => setError(String(e)))}>删除代理</button>}
        </div>
      )}
      {dialog === "proxy" && <ProxyDialog groups={shownGroups.filter((g) => g.type === "Selector").map((g) => g.name)} onClose={() => { setDialog(""); load(); }} />}
      {dialog === "group" && <GroupDialog onClose={() => { setDialog(""); load(); }} />}
    </>
  );
}
