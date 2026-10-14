import { useCallback, useEffect, useState } from "react";
import { CoreService, ProfileService, ToolService } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import type { EasyConnectTunnel, Status } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";
import { nonNull } from "./format";
import { usePoll } from "./hooks";
import PasswordInput from "./PasswordInput";

// A VPN is one node plus the rules that send traffic into it: for EasyConnect the gateway's own
// route list (the EASYCONNECT rule), for every type the addresses listed here. Networks named
// under 直连 get a subnet group in between, which picks DIRECT there and the node elsewhere.
const defaultName = "VPN";
const types: Record<string, string> = { easyconnect: "EasyConnect", wireguard: "WireGuard", openvpn: "OpenVPN", tailscale: "Tailscale" };
const noServer = new Set(["openvpn", "tailscale"]); // the address comes from the file, or the control plane

const stateLabel: Record<string, string> = { up: "已连接", connecting: "连接中", down: "未连接" };
const stateClass: Record<string, string> = { up: "delay-good", connecting: "delay-warn", down: "delay-bad" };

type Form = Record<string, string>;
const empty: Form = { type: "easyconnect", port: "443" };

// parseOvpn pulls what the core's openvpn node needs out of an .ovpn file.
function parseOvpn(text: string): Record<string, any> {
  const out: Record<string, any> = {};
  const block = (tag: string) => text.match(new RegExp(`<${tag}>\\s*([\\s\\S]*?)\\s*</${tag}>`))?.[1];
  const line = (key: string) => text.match(new RegExp(`^\\s*${key}\\s+(.+?)\\s*$`, "m"))?.[1];
  const remote = line("remote")?.split(/\s+/);
  if (remote) { out.server = remote[0]; out.port = Number(remote[1]) || 1194; if (remote[2]) out.proto = remote[2]; }
  for (const k of ["proto", "cipher", "auth", "dev", "key-direction"]) { const v = line(k); if (v) out[k] = v; }
  const dc = line("data-ciphers"); if (dc) out["data-ciphers"] = dc.split(":");
  if (/^\s*comp-lzo/m.test(text)) out["comp-lzo"] = line("comp-lzo") ?? "yes";
  for (const tag of ["ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2"]) {
    const v = block(tag); if (v) out[tag] = v;
  }
  return out;
}

const isCIDR = (s: string) => /^\d+\.\d+\.\d+\.\d+(\/\d+)?$/.test(s) || /^[0-9a-f:]+(\/\d+)?$/i.test(s);
// ruleOf turns one 走 VPN 的地址 line into a rule aimed at target.
const ruleOf = (addr: string, target: string) => (isCIDR(addr) ? `IP-CIDR,${addr.includes("/") ? addr : addr + "/32"},${target},no-resolve` : `DOMAIN-SUFFIX,${addr},${target}`);
// addrOf reads it back.
const addrOf = (rule: string) => { const f = rule.split(",").map((x) => x.trim()); return f[0] === "IP-CIDR" ? f[1].replace(/\/32$/, "") : f[0] === "DOMAIN-SUFFIX" ? f[1] : ""; };
const cleanBypass = (b: string[]) => b.map((x) => x.trim()).filter((x) => x && !x.endsWith(":"));

// ListRow is an input with an 添加 button, as the rows above it, with what was added as chips
// below; clicking a chip removes it.
function ListRow({ items, onChange, placeholder, extra }: { items: string[]; onChange: (v: string[]) => void; placeholder: string; extra?: React.ReactNode }) {
  const [text, setText] = useState("");
  const add = () => { const v = text.trim(); if (v && !items.includes(v)) onChange([...items, v]); setText(""); };
  return (
    <>
      <div className="form">
        <input className="grow" placeholder={placeholder} value={text} onChange={(e) => setText(e.target.value)} onKeyDown={(e) => e.key === "Enter" && add()} />
        <button className="btn" disabled={!text.trim()} onClick={add}>添加</button>
        {extra}
      </div>
      {items.length > 0 && (
        <div className="form" style={{ flexWrap: "wrap", marginTop: 8 }}>
          {items.map((x) => <button key={x} className="btn small-btn" title="点击移除" onClick={() => onChange(items.filter((y) => y !== x))}>{x} ×</button>)}
        </div>
      )}
    </>
  );
}

export default function Vpn({ status }: { status: Status }) {
  const [form, setForm] = useState<Form>(empty);
  const [addrs, setAddrs] = useState<string[]>([]);
  const [bypass, setBypass] = useState<string[]>([]);
  const [name, setName] = useState(defaultName);
  const [saved, setSaved] = useState(false);
  const [tunnel, setTunnel] = useState<EasyConnectTunnel | null>(null);
  const [delay, setDelay] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [target, setTarget] = useState("");
  const [testing, setTesting] = useState(false);
  const [probe, setProbe] = useState<string[]>([]);
  const [probing, setProbing] = useState(false);
  const [result, setResult] = useState<string[]>([]);

  const load = useCallback(() => {
    CoreService.EasyConnects().then((t) => setTunnel(nonNull(t).find((x) => x.name === name) ?? null)).catch(() => setTunnel(null));
  }, [name]);
  usePoll(load, 5000, status.running && saved && form.type === "easyconnect");

  useEffect(() => {
    ProfileService.VPN().then((v) => {
      setSaved(!!v?.active);
      if (!v) return;
      const p = v.proxy ?? {};
      setName(String(p.name ?? defaultName));
      const f: Form = { type: String(p.type ?? "easyconnect") };
      for (const [k, x] of Object.entries(p)) if (k !== "name" && k !== "type" && (typeof x === "string" || typeof x === "number" || typeof x === "boolean")) f[k] = String(x);
      if (Array.isArray(p["allowed-ips"])) f["allowed-ips"] = p["allowed-ips"].join(", ");
      setForm(f);
      setAddrs(nonNull(v.rules).map(addrOf).filter(Boolean));
      setBypass(Object.keys(v.group?.rules ?? {}));
    });
  }, []);

  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => setForm({ ...form, [k]: e.target.value });
  const text = (k: string, placeholder: string, extra: React.InputHTMLAttributes<HTMLInputElement> = {}) => <input className="grow" placeholder={placeholder} value={form[k] ?? ""} onChange={set(k)} {...extra} />;
  const secret = (k: string, placeholder: string) => <PasswordInput className="grow" placeholder={placeholder} value={form[k] ?? ""} onChange={set(k)} />;

  const ready = (() => {
    switch (form.type) {
      case "easyconnect": return form.server?.trim() && form.username?.trim() && form.password;
      case "wireguard": return form.server?.trim() && form["private-key"]?.trim() && form["public-key"]?.trim() && form.ip?.trim();
      case "openvpn": return form["ovpn-config"]?.trim();
      case "tailscale": return true; // without an auth key the core logs a login link
    }
    return false;
  })();

  const conds = cleanBypass(bypass);
  const listedAddrs = addrs;

  // build is the override's view of the form: the node, its rules and the bypass group.
  const build = () => {
    const proxy: Record<string, any> = { name, type: form.type, udp: true };
    const str = (k: string) => { const v = form[k]?.trim(); if (v) proxy[k] = v; };
    const num = (k: string, def?: number) => { const v = Number(form[k]) || def; if (v) proxy[k] = v; };
    let auto: string[] = [];
    switch (form.type) {
      case "easyconnect":
        str("server"); num("port", 443); str("username"); str("password"); str("totp-secret");
        break;
      case "wireguard":
        str("server"); num("port"); str("private-key"); str("public-key"); str("pre-shared-key"); str("ip"); str("ipv6"); num("mtu");
        auto = (form["allowed-ips"] ?? "").split(/[,\s]+/).filter(Boolean);
        proxy["allowed-ips"] = auto.length ? auto : ["0.0.0.0/0"];
        break;
      case "openvpn":
        Object.assign(proxy, parseOvpn(form["ovpn-config"] ?? ""));
        proxy["ovpn-config"] = form["ovpn-config"];
        str("username"); str("password");
        break;
      case "tailscale":
        str("auth-key"); str("hostname"); str("control-url"); str("exit-node");
        if (form["accept-routes"] === "true") proxy["accept-routes"] = true;
        auto = ["100.64.0.0/10"]; // the tailnet's own addresses
        break;
    }
    const policy = conds.length ? `${name} 路由` : name;
    const rules = form.type === "easyconnect" ? [`EASYCONNECT,${name},${policy}`] : [];
    [...new Set(listedAddrs.length ? listedAddrs : auto)].filter((a) => a !== "0.0.0.0/0" && a !== "::/0").forEach((a) => rules.push(ruleOf(a, policy)));
    const group = conds.length ? { name: policy, type: "subnet", default: name, rules: Object.fromEntries(conds.map((b) => [b, "DIRECT"])) } : null;
    return { proxy, rules, group };
  };

  const run = (p: Promise<unknown>) => {
    setBusy(true);
    setError("");
    p.catch((e) => setError(String(e))).finally(() => { setBusy(false); load(); });
  };
  const save = () => run(ProfileService.SetVPN(build() as any).then(() => setSaved(true)));
  const disconnect = () => run(ProfileService.RemoveVPN().then(() => { setSaved(false); setTunnel(null); }));
  // probeGateway logs in with the form as it is, without saving: the core reports what the gateway handed out.
  const probeGateway = () => {
    setProbing(true);
    setProbe(["登录中…"]);
    ToolService.VPNProbe(build().proxy).then((l) => setProbe(nonNull(l))).catch((e) => setProbe([String(e)])).finally(() => setProbing(false));
  };
  const measure = () => { setDelay("测量中…"); CoreService.TestProxy(name).then((ms) => setDelay(`${ms} ms`)).catch((e) => setDelay(String(e))); };
  const useCurrent = () => CoreService.CurrentNetwork().then((n) => {
    const cond = n.ssid ? `SSID:${n.ssid}` : n.router ? `ROUTER:${n.router}` : "";
    if (cond && !conds.includes(cond)) setBypass([...conds, cond]);
  });

  const routing = tunnel?.routing;
  const test = () => {
    setTesting(true);
    setResult([]);
    ToolService.VPNTest(target.trim()).then((l) => setResult(nonNull(l))).catch((e) => setResult([String(e)])).finally(() => setTesting(false));
  };

  return (
    <>
      <div className="hint">VPN 那边的网段和域名走 VPN，其余流量不受影响。</div>
      <div className="card" style={{ marginTop: 16 }}>
        <div className="label">VPN</div>
        <div className="form">
          <select value={form.type} disabled={saved} onChange={(e) => setForm({ ...empty, type: e.target.value, port: e.target.value === "easyconnect" ? "443" : "" })}>
            {Object.entries(types).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </select>
          {!noServer.has(form.type) && text("server", form.type === "easyconnect" ? "网关地址，如 vpn.example.edu.cn" : "服务器地址")}
          {!noServer.has(form.type) && <input style={{ width: 80 }} placeholder={form.type === "easyconnect" ? "443" : "端口"} value={form.port ?? ""} onChange={set("port")} />}
        </div>
        {form.type === "easyconnect" && <>
          <div className="form">{text("username", "用户名")}{secret("password", "密码")}</div>
          <div className="form">{secret("totp-secret", "TOTP 密钥，可选")}</div>
        </>}
        {form.type === "wireguard" && <>
          <div className="form">{secret("private-key", "本机私钥")}{text("public-key", "对端公钥")}</div>
          <div className="form">{secret("pre-shared-key", "预共享密钥，可选")}{text("ip", "本机地址，如 10.0.0.2")}{text("mtu", "MTU，可选", { style: { width: 90 } })}</div>
          <div className="form">{text("allowed-ips", "对端网段（allowed-ips），逗号分隔，如 10.0.0.0/8")}</div>
        </>}
        {form.type === "openvpn" && <>
          <div className="form"><textarea className="grow" rows={5} placeholder="粘贴 .ovpn 文件内容（含 remote、<ca>、<cert>、<key>）" value={form["ovpn-config"] ?? ""} onChange={set("ovpn-config")} /></div>
          <div className="form">{text("username", "用户名，可选")}{secret("password", "密码，可选")}</div>
        </>}
        {form.type === "tailscale" && <>
          <div className="form">{secret("auth-key", "Auth key，可选；不填则在日志里找登录链接")}{text("hostname", "本机在 tailnet 里的名字，可选")}</div>
          <div className="form">{text("control-url", "控制服务器，Headscale 时填，可选")}{text("exit-node", "出口节点，可选")}
            <label className="radio"><input type="checkbox" checked={form["accept-routes"] === "true"} onChange={(e) => setForm({ ...form, "accept-routes": String(e.target.checked) })} /> 接受子网路由</label></div>
        </>}
        <ListRow items={addrs} onChange={setAddrs} placeholder={form.type === "easyconnect" ? "额外经 VPN 的网段或域名，如 10.0.0.0/8、example.com（可选）" : "经 VPN 的网段或域名，如 10.0.0.0/8、example.com"} />
        <ListRow items={bypass} onChange={setBypass} placeholder="直连网络：SSID:我的 Wi-Fi 或 ROUTER:192.168.1.1（可选）"
          extra={<button className="btn" disabled={!status.running} onClick={useCurrent}>添加当前网络</button>} />
        <div className="form">
          <button className="btn primary" disabled={busy || !ready || !status.running} onClick={save}>{saved ? "保存并重连" : "连接"}</button>
          {saved && <button className="btn" disabled={busy} onClick={disconnect}>断开</button>}
          {form.type === "easyconnect" && <button className="btn" disabled={probing || !ready || !status.running} onClick={probeGateway}>{probing ? "登录中…" : "测试登录"}</button>}
        </div>
        {error && <div className="error small" style={{ marginTop: 8 }}>{error}</div>}
        {probe.map((l, i) => <div key={i} className={`detail-row ${/失败|错误|refused|rejected/i.test(l) ? "error" : ""}`}>{l}</div>)}
      </div>

      {saved && (
        <div className="card" style={{ marginTop: 16 }}>
          <div className="label">状态</div>
          {form.type === "easyconnect" ? (
            <>
              <div className="detail-row">
                <span className={stateClass[tunnel?.state ?? "down"]}>{stateLabel[tunnel?.state ?? "down"]}</span>
                {tunnel?.address && <span className="muted">隧道地址 {tunnel.address}</span>}
                {routing?.dns?.length ? <span className="muted">DNS {routing.dns.join(", ")}</span> : null}
              </div>
              {routing && <div className="detail-row"><span className="muted">网关下发：{routing.ip?.length ?? 0} 个网段、{routing.domains?.length ?? 0} 个域名</span></div>}
              {nonNull(routing?.ip).map((r, i) => (
                <div key={i} className="detail-row"><span>{r.from === r.to ? r.from : `${r.from} ~ ${r.to}`}</span><span className="muted">{r.portMin === r.portMax ? r.portMin : `${r.portMin}-${r.portMax}`} {r.protocol}</span></div>
              ))}
              {nonNull(routing?.domains).map((d) => <div key={d} className="detail-row"><span>{d}</span></div>)}
            </>
          ) : (
            <div className="detail-row">
              <button className="btn small-btn" disabled={!status.running} onClick={measure}>测延迟</button>
              {delay && <span className="muted">{delay}</span>}
            </div>
          )}
        </div>
      )}

      {saved && (
        <div className="card" style={{ marginTop: 16 }}>
          <div className="label">测试</div>
          <div className="form">
            <input className="grow" placeholder="目标地址，如 10.1.2.3:22 或 git.example.com:443" value={target} onChange={(e) => setTarget(e.target.value)} onKeyDown={(e) => e.key === "Enter" && target && !testing && test()} />
            <button className="btn primary" disabled={testing || !target.trim() || !status.running} onClick={test}>{testing ? "测试中…" : "测试"}</button>
          </div>
          {result.map((l, i) => <div key={i} className={`detail-row ${/失败|超时|拒绝|应答|连不上/.test(l) ? "error" : ""}`}>{l}</div>)}
        </div>
      )}
    </>
  );
}
