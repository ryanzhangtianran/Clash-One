import { useCallback, useEffect, useState } from "react";
import { CoreService, DeviceService } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import type { Device, LanInfo, Status } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import { sizeText } from "./format";

function PolicyMenu({ device, policies, onDone }: { device: Device; policies: string[]; onDone: (err?: unknown) => void }) {
  const [policy, setPolicy] = useState(device.policy || "DIRECT");
  const [permanent, setPermanent] = useState(!device.temporary && !!device.policy);
  return (
    <div className="menu members detail" onClick={(e) => e.stopPropagation()}>
      <div className="detail-head">{device.ip}{device.mac && ` · ${device.mac}`} · {device.connections} 个连接</div>
      <div className="detail-row"><span>上传 {sizeText(device.upload)}</span><span>下载 {sizeText(device.download)}</span></div>
      <div className="detail-row" style={{ alignItems: "center" }}>
        <span>策略</span>
        <select value={policy} onChange={(e) => setPolicy(e.target.value)}>{policies.map((p) => <option key={p}>{p}</option>)}</select>
      </div>
      <div className="detail-row">
        <label className="radio"><input type="radio" checked={!permanent} onChange={() => setPermanent(false)} /> 临时</label>
        <label className="radio"><input type="radio" checked={permanent} onChange={() => setPermanent(true)} /> 永久（写入覆写）</label>
      </div>
      <button onClick={() => DeviceService.SetPolicy(device.ip, policy, permanent).then(() => onDone(), onDone)}>应用：此设备全部走 {policy}</button>
      {device.policy && <button className="danger" onClick={() => DeviceService.ClearPolicy(device.ip).then(() => onDone(), onDone)}>恢复按规则分流</button>}
    </div>
  );
}

export default function Devices({ status }: { status: Status }) {
  const [lan, setLan] = useState<LanInfo | null>(null);
  const [devices, setDevices] = useState<Device[]>([]);
  const [policies, setPolicies] = useState<string[]>([]);
  const [open, setOpen] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(() => {
    DeviceService.Lan().then(setLan).catch((e) => setError(String(e)));
    DeviceService.Devices().then((d) => setDevices(d ?? [])).catch((e) => setError(String(e)));
  }, []);

  useEffect(() => {
    if (!status.running) return;
    load();
    CoreService.Policies().then((p) => setPolicies(p ?? []));
    const t = setInterval(load, 2000);
    return () => clearInterval(t);
  }, [status.running, load]);

  const toggleLan = () => {
    setError("");
    DeviceService.SetAllowLan(!lan?.allowLan).then(load).catch((e) => setError(String(e)));
  };

  return (
    <>
      <div className="seg big-seg" style={{ maxWidth: 400 }}>
        <button className={!lan?.allowLan ? "on accent" : ""} disabled={!status.running} onClick={() => lan?.allowLan && toggleLan()}>仅本机</button>
        <button className={lan?.allowLan ? "on accent" : ""} disabled={!status.running} onClick={() => !lan?.allowLan && toggleLan()}>允许局域网设备</button>
      </div>
      <div className="hint">
        {lan?.allowLan
          ? <>在其他设备的 Wi-Fi 设置中把 HTTP 或 SOCKS5 代理设为 <b>{(lan.addresses ?? []).join(" 或 ")}</b> 端口 <b>{lan.port}</b>。</>
          : "开启后，局域网内的手机、电脑等可以通过本机代理上网。"}
      </div>
      {error && <div className="error" style={{ marginTop: 10 }}>{error}</div>}

      <div className="tile-head"><span className="tile-title groups">局域网设备</span></div>
      {devices.length === 0 && <div className="muted">{lan?.allowLan ? "还没有设备连接。" : "未开启局域网访问。"}</div>}
      <div className="tiles">
        {devices.map((d) => (
          <div key={d.ip} className="tile-wrap">
            <button className={`tile ${open === d.ip ? "selected" : ""}`} onClick={() => setOpen(open === d.ip ? "" : d.ip)}>
              <span className="kind">{d.ip}{d.connections > 0 ? ` · ${d.connections} 个连接` : " · 离线"}</span>
              <span className="name">{d.name || d.mac || "未知设备"}</span>
              <span className="foot">{d.policy ? `全部走 ${d.policy}${d.temporary ? "（临时）" : ""}` : sizeText(d.upload + d.download)}</span>
            </button>
            {open === d.ip && (
              <PolicyMenu device={d} policies={policies} onDone={(err) => { if (err) setError(String(err)); setOpen(""); load(); }} />
            )}
          </div>
        ))}
      </div>
    </>
  );
}
