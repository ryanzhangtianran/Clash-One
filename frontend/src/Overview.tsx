import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { CoreService, DeviceService, EnhancedService, SystemProxyService } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import type { LanInfo, Status, Tunnel } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import { ProcessPicker, Switch, useSetting } from "./Settings";

// Feature is one switchable capability: what it does, and whether it is working right now.
function Feature({ title, desc, state, ok, error, control, children }: { title: string; desc: string; state: string; ok: boolean; error?: string; control: React.ReactNode; children?: React.ReactNode }) {
  return (
    <div className="card feature">
      <div className="feature-head"><span className="feature-title">{title}</span>{control}</div>
      <div className="muted feature-desc">{desc}</div>
      {children}
      <div className="feature-state"><span className={`dot ${error ? "err" : ok ? "on" : "warn"}`} />{error || state}</div>
    </div>
  );
}

export default function Overview({ status, onOpen }: { status: Status; onOpen: (page: string) => void }) {
  const [sysProxy, setSysProxy] = useState(false);
  const [enhanced, setEnhanced] = useState(false);
  const [lan, setLan] = useState<LanInfo | null>(null);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [busy, setBusy] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [mode, setMode, modeErr] = useSetting<string>("capture.mode", "tun");
  const [processes, setProcesses, procErr] = useSetting<string[]>("capture.processes", []);

  const load = useCallback(() => {
    SystemProxyService.Enabled().then(setSysProxy);
    EnhancedService.Enabled().then(setEnhanced);
    DeviceService.Lan().then(setLan).catch(() => {});
    CoreService.ReverseTunnels().then((t) => setTunnels(t ?? [])).catch(() => {});
  }, []);
  useEffect(() => {
    if (!status.running) return;
    load();
    const offs = [Events.On("sysproxy", (e) => setSysProxy(e.data)), Events.On("enhanced", (e) => setEnhanced(e.data))];
    const t = setInterval(load, 3000);
    return () => { clearInterval(t); offs.forEach((o) => o()); };
  }, [status.running, load]);

  const run = (key: string, p: Promise<unknown>) => {
    setBusy(key);
    setErrors((e) => ({ ...e, [key]: "" }));
    p.catch((err) => setErrors((e) => ({ ...e, [key]: String(err) }))).finally(() => { setBusy(""); load(); });
  };
  const off = !status.running;
  const port = lan?.port ?? 0;
  const running = tunnels.filter((t) => t.state === "running").length;
  const partial = mode === "process-tun";
  const picked = processes ?? [];

  return (
    <>
      <div className="tile-head"><span className="tile-title proxies">网络接管</span></div>
      <div className="features">
        <Feature title="系统代理" desc="把 macOS 的系统代理指向本机，兼容性和性能最好。"
          ok={sysProxy} state={sysProxy ? `已设为系统代理 127.0.0.1:${port}` : "未启用"} error={errors.sys}
          control={<Switch on={sysProxy} disabled={off || busy === "sys"} onChange={(v) => run("sys", SystemProxyService.SetEnabled(v))} />} />
        <Feature title="增强模式" desc="以 TUN 接管全部流量，包括不遵循系统代理的应用。"
          ok={enhanced} state={busy === "enh" ? "切换中…" : enhanced ? "TUN 已接管" : "未启用"} error={errors.enh}
          control={<Switch on={enhanced} disabled={off || busy === "enh"} onChange={(v) => run("enh", EnhancedService.SetEnabled(v))} />} />
        <Feature title="部分接管" desc="增强模式只接管指定应用及其子进程，其余流量直连。"
          ok={partial && enhanced && picked.length > 0} error={modeErr || procErr}
          state={!partial ? "未启用：增强模式接管全部流量" : !enhanced ? "需先开启增强模式" : picked.length === 0 ? "还没有选择应用" : `接管 ${picked.length} 个应用：${picked.join("、")}`}
          control={<Switch on={partial} disabled={off} onChange={(v) => setMode(v ? "process-tun" : "tun")} />}>
          {partial && <div className="feature-picker"><ProcessPicker value={picked} onChange={setProcesses} /></div>}
        </Feature>
      </div>

      <div className="tile-head"><span className="tile-title groups">局域网设备接管</span></div>
      <div className="features">
        <Feature title="HTTP & SOCKS5 代理" desc="局域网内的设备可以把本机用作 HTTP / SOCKS5 代理。"
          ok={!!lan?.allowLan} state={lan?.allowLan ? `其他设备可连接 ${(lan.addresses ?? [])[0] ?? "本机"}:${port}` : "未启用"} error={errors.lan}
          control={<Switch on={!!lan?.allowLan} disabled={off || busy === "lan"} onChange={(v) => run("lan", DeviceService.SetAllowLan(v))} />} />
      </div>

      <div className="tile-head"><span className="tile-title remote">远程连接</span></div>
      <div className="features">
        <Feature title="反向隧道" desc="通过 FRP 或 SSH 把本机服务暴露到公网。"
          ok={running > 0} state={tunnels.length === 0 ? "未配置" : `${running} / ${tunnels.length} 个运行中`}
          control={<button className="btn small-btn" onClick={() => onOpen("tunnels")}>管理</button>} />
      </div>
    </>
  );
}
