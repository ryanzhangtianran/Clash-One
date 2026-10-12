import { useCallback, useState } from "react";
import { EnhancedService, SettingsService } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import type { ProcessChoice, Status } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import { Switch, useSetting } from "./Settings";
import { usePoll } from "./hooks";
import { words } from "./format";

const sources: Record<string, string> = { app: "正在运行", network: "有网络连接" };

// Processes is the enhanced page: enhanced mode routes all traffic through TUN, partial enhancement
// only the selected apps (and their child processes). The two exclude each other (EnhancedService.Mode).
export default function Processes({ status, mode, rooted }: { status: Status; mode: string; rooted: boolean }) {
  const [busy, setBusy] = useState("");
  const [processes, setProcesses] = useSetting<string[]>("capture.processes", []);
  const [choices, setChoices] = useState<ProcessChoice[]>([]);
  const [query, setQuery] = useState("");

  const load = useCallback(() => { SettingsService.ProcessChoices().then((c) => setChoices(c ?? [])); }, []);
  usePoll(load, 5000);

  const run = (key: string, p: Promise<unknown>) => {
    setBusy(key);
    p.finally(() => setBusy(""));
  };
  const partial = mode === "process-tun";
  const picked = processes ?? [];
  const has = (exec: string) => picked.some((p) => p.toLowerCase() === exec.toLowerCase());
  const toggle = (exec: string, on: boolean) => setProcesses(on ? [...picked, exec] : picked.filter((p) => p.toLowerCase() !== exec.toLowerCase()));
  const pick = () => SettingsService.PickApp().then((c) => { if (c.exec && !has(c.exec)) toggle(c.exec, true); });

  const known = new Map(choices.map((c) => [c.exec.toLowerCase(), c]));
  const rows = [
    ...picked.map((exec) => known.get(exec.toLowerCase()) ?? { name: exec, exec, source: "" }),
    ...choices.filter((c) => !has(c.exec)),
  ];
  const q = query.toLowerCase();
  const shown = rows.filter((r) => !q || `${r.name} ${r.exec}`.toLowerCase().includes(q));
  const off = !status.running || !!busy || !rooted;
  const partialState = busy === "partial" ? "切换中…" : !rooted ? "需先在「通用」中授予 root 权限" : !partial ? "选中的应用及其子进程经 TUN 按规则分流，其他应用照常遵循系统代理" : picked.length === 0 ? "还没有选择应用，暂不接管" : `已增强 ${picked.length} 个应用`;

  return (
    <>
      <div className="group-title">增强模式</div>
      <div className="plist">
        <div className="plist-row static">
          <div className="plist-main"><div className="plist-name">增强模式</div><div className="muted small">{busy === "enh" ? "切换中…" : !rooted ? "需先在「通用」中授予 root 权限" : mode === "tun" ? "TUN 已接管全部流量" : "以 TUN 接管全部流量，包括不遵循系统代理的应用"}</div></div>
          <Switch on={mode === "tun"} disabled={off} onChange={(v) => run("enh", EnhancedService.SetMode(v ? "tun" : ""))} />
        </div>
      </div>

      <div className="group-title">部分增强</div>
      <div className="plist">
        <div className="plist-row static">
          <div className="plist-main"><div className="plist-name">部分增强</div><div className="muted small">{partialState}</div></div>
          <Switch on={partial} disabled={off} onChange={(v) => run("partial", EnhancedService.SetMode(v ? "process-tun" : ""))} />
        </div>
      </div>

      {partial && <>
        <div className="plist" style={{ marginTop: 12 }}>
          <div className="plist-tools">
            <input className="dash-search" placeholder="搜索" value={query} onChange={(e) => setQuery(e.target.value)} />
            <span className="grow" />
            <button className="btn" onClick={pick}>{words.pickApp}</button>
            <button className="btn" onClick={load}>刷新</button>
          </div>
          {shown.length === 0 && <div className="plist-empty muted">没有找到应用或进程</div>}
          {shown.map((r) => (
            <div key={r.exec} className="plist-row static">
              <div className="plist-main">
                <div className="plist-name">{r.name}</div>
                <div className="muted small">{r.exec !== r.name ? `${r.exec} · ` : ""}{sources[r.source] ?? "未运行"}</div>
              </div>
              <Switch on={has(r.exec)} onChange={(v) => toggle(r.exec, v)} />
            </div>
          ))}
        </div>
      </>}
    </>
  );
}
