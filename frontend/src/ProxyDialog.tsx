import { useEffect, useState } from "react";
import { CoreService, ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";

type Field = {
  key: string; // mihomo key; dots nest: "ws-opts.path"
  label: string;
  kind?: "text" | "number" | "password" | "bool" | "select";
  options?: string[];
  placeholder?: string;
  when?: [string, string[]]; // show only when another field has one of these values
  def?: string | boolean;
};

const tlsFields: Field[] = [
  { key: "sni", label: "SNI", placeholder: "默认为服务器地址" },
  { key: "skip-cert-verify", label: "跳过证书验证", kind: "bool" },
];
const transport: Field[] = [
  { key: "network", label: "传输方式", kind: "select", options: ["tcp", "ws", "grpc", "h2", "http"], def: "tcp" },
  { key: "ws-opts.path", label: "WS 路径", placeholder: "/", when: ["network", ["ws"]] },
  { key: "ws-opts.headers.Host", label: "WS Host", when: ["network", ["ws"]] },
  { key: "grpc-opts.grpc-service-name", label: "gRPC 服务名", when: ["network", ["grpc"]] },
];
const ssCiphers = ["aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305", "none"];

const protocols: Record<string, { label: string; fields: Field[] }> = {
  http: { label: "HTTP", fields: [{ key: "username", label: "用户名" }, { key: "password", label: "密码", kind: "password" }, { key: "tls", label: "TLS", kind: "bool" }, ...tlsFields] },
  socks5: { label: "SOCKS5", fields: [{ key: "username", label: "用户名" }, { key: "password", label: "密码", kind: "password" }, { key: "tls", label: "TLS", kind: "bool" }, { key: "skip-cert-verify", label: "跳过证书验证", kind: "bool" }] },
  ss: { label: "Shadowsocks", fields: [{ key: "cipher", label: "加密方式", kind: "select", options: ssCiphers, def: "aes-128-gcm" }, { key: "password", label: "密码", kind: "password" }] },
  vmess: { label: "VMess", fields: [{ key: "uuid", label: "UUID" }, { key: "alterId", label: "alterId", kind: "number", def: "0" }, { key: "cipher", label: "加密", kind: "select", options: ["auto", "aes-128-gcm", "chacha20-poly1305", "none"], def: "auto" }, { key: "tls", label: "TLS", kind: "bool" }, { key: "servername", label: "SNI" }, ...transport] },
  vless: { label: "VLESS", fields: [{ key: "uuid", label: "UUID" }, { key: "flow", label: "Flow", kind: "select", options: ["", "xtls-rprx-vision"], def: "" }, { key: "tls", label: "TLS", kind: "bool", def: true }, { key: "servername", label: "SNI" }, { key: "client-fingerprint", label: "指纹", kind: "select", options: ["", "chrome", "firefox", "safari", "ios", "random"], def: "" }, { key: "reality-opts.public-key", label: "Reality 公钥" }, { key: "reality-opts.short-id", label: "Reality short-id" }, ...transport] },
  trojan: { label: "Trojan", fields: [{ key: "password", label: "密码", kind: "password" }, ...tlsFields, ...transport] },
  hysteria2: { label: "Hysteria2", fields: [{ key: "password", label: "密码", kind: "password" }, ...tlsFields, { key: "up", label: "上行", placeholder: "如 50 Mbps" }, { key: "down", label: "下行", placeholder: "如 200 Mbps" }, { key: "obfs", label: "混淆", kind: "select", options: ["", "salamander"], def: "" }, { key: "obfs-password", label: "混淆密码", kind: "password", when: ["obfs", ["salamander"]] }] },
  tuic: { label: "TUIC", fields: [{ key: "uuid", label: "UUID" }, { key: "password", label: "密码", kind: "password" }, ...tlsFields, { key: "congestion-controller", label: "拥塞控制", kind: "select", options: ["bbr", "cubic", "new_reno"], def: "bbr" }] },
  anytls: { label: "AnyTLS", fields: [{ key: "password", label: "密码", kind: "password" }, ...tlsFields] },
  ssh: { label: "SSH", fields: [{ key: "username", label: "用户名" }, { key: "password", label: "密码", kind: "password" }, { key: "private-key", label: "私钥（内容）", placeholder: "-----BEGIN OPENSSH PRIVATE KEY-----…" }] },
  wireguard: { label: "WireGuard", fields: [{ key: "private-key", label: "私钥" }, { key: "public-key", label: "对端公钥" }, { key: "ip", label: "IPv4 地址", placeholder: "10.0.0.2" }, { key: "ipv6", label: "IPv6 地址" }, { key: "mtu", label: "MTU", kind: "number" }] },
};

// set writes value at a dotted key, creating nested maps.
function set(obj: Record<string, any>, key: string, value: unknown) {
  const parts = key.split(".");
  let cur = obj;
  for (const p of parts.slice(0, -1)) cur = cur[p] ??= {};
  cur[parts[parts.length - 1]] = value;
}

export default function ProxyDialog({ groups, onClose }: { groups: string[]; onClose: () => void }) {
  const [type, setType] = useState("ss");
  const [v, setV] = useState<Record<string, any>>({});
  const [names, setNames] = useState<string[]>([]);
  const [join, setJoin] = useState<string[]>(groups.slice(0, 1));
  const [extra, setExtra] = useState("");
  const [error, setError] = useState("");
  const [result, setResult] = useState("");
  useEffect(() => { CoreService.ProxyNames().then((n) => setNames(n ?? [])); }, []);

  const fields = protocols[type].fields;
  const val = (f: Field) => v[f.key] ?? f.def ?? (f.kind === "bool" ? false : "");
  const shown = (f: Field) => !f.when || f.when[1].includes(String(v[f.when[0]] ?? fields.find((x) => x.key === f.when![0])?.def ?? ""));

  const build = () => {
    const proxy: Record<string, any> = { name: v.name, type, server: v.server, port: Number(v.port) };
    for (const f of fields) {
      if (!shown(f)) continue;
      const x = val(f);
      if (x === "" || (f.key === "network" && x === "tcp")) continue;
      set(proxy, f.key, f.kind === "number" ? Number(x) : x);
    }
    for (const k of ["udp", "tfo"]) if (v[k]) proxy[k] = true;
    for (const k of ["ip-version", "interface-name", "dialer-proxy"]) if (v[k]) proxy[k] = v[k];
    return proxy;
  };

  const add = async (test: boolean) => {
    setError(""); setResult("");
    try {
      await ProfileService.AddProxy(build(), extra, join);
      if (!test) return onClose();
      const ms = await CoreService.TestProxy(v.name);
      setResult(`已添加，延迟 ${ms} ms`);
    } catch (e) {
      setError(String(e));
    }
  };

  const input = (f: Field) => {
    const x = val(f);
    const on = (value: unknown) => setV({ ...v, [f.key]: value });
    if (f.kind === "bool") return <input type="checkbox" checked={!!x} onChange={(e) => on(e.target.checked)} />;
    if (f.kind === "select") return <select value={x} onChange={(e) => on(e.target.value)}>{f.options!.map((o) => <option key={o} value={o}>{o || "无"}</option>)}</select>;
    return <input type={f.kind === "password" ? "password" : "text"} placeholder={f.placeholder ?? "可选"} value={x} onChange={(e) => on(e.target.value)} />;
  };

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal card wide" onClick={(e) => e.stopPropagation()}>
        <h2 className="modal-title">增加代理</h2>
        <div className="two-col">
          <div>
            <div className="section-title">基本信息</div>
            <div className="form-grid">
              <label>名称</label><input autoFocus placeholder="必填" value={v.name ?? ""} onChange={(e) => setV({ ...v, name: e.target.value })} />
              <label>协议</label>
              <select value={type} onChange={(e) => setType(e.target.value)}>
                {Object.entries(protocols).map(([k, p]) => <option key={k} value={k}>{p.label}</option>)}
              </select>
            </div>
            <div className="section-title">服务器信息</div>
            <div className="form-grid">
              <label>地址</label>
              <div className="inline"><input placeholder="地址" value={v.server ?? ""} onChange={(e) => setV({ ...v, server: e.target.value })} style={{ flex: 1 }} /> : <input placeholder="端口" value={v.port ?? ""} onChange={(e) => setV({ ...v, port: e.target.value })} style={{ width: 80 }} /></div>
            </div>
            <div className="section-title">{protocols[type].label}</div>
            <div className="form-grid">
              {fields.filter(shown).map((f) => [<label key={f.key + "l"}>{f.label}</label>, <span key={f.key}>{input(f)}</span>])}
            </div>
            <div className="section-title">代理链</div>
            <div className="form-grid">
              <label>跳板代理</label>
              <select value={v["dialer-proxy"] ?? ""} onChange={(e) => setV({ ...v, "dialer-proxy": e.target.value })}>
                <option value="">不使用</option>
                {names.map((n) => <option key={n}>{n}</option>)}
              </select>
            </div>
          </div>
          <div>
            <div className="section-title">加入策略组</div>
            <div className="checks">
              {groups.map((g) => (
                <label key={g} className="radio"><input type="checkbox" checked={join.includes(g)} onChange={(e) => setJoin(e.target.checked ? [...join, g] : join.filter((x) => x !== g))} /> {g}</label>
              ))}
            </div>
            <div className="section-title">出站设置</div>
            <div className="form-grid">
              <label>网络接口</label><input placeholder="默认" value={v["interface-name"] ?? ""} onChange={(e) => setV({ ...v, "interface-name": e.target.value })} />
              <label>IP 版本</label>
              <select value={v["ip-version"] ?? ""} onChange={(e) => setV({ ...v, "ip-version": e.target.value })}>
                {["", "ipv4", "ipv6", "ipv4-prefer", "ipv6-prefer"].map((o) => <option key={o} value={o}>{o || "双栈"}</option>)}
              </select>
            </div>
            <div className="section-title">选项</div>
            <label className="radio"><input type="checkbox" checked={!!v.udp} onChange={(e) => setV({ ...v, udp: e.target.checked })} /> 允许 UDP</label>
            <label className="radio"><input type="checkbox" checked={!!v.tfo} onChange={(e) => setV({ ...v, tfo: e.target.checked })} /> TCP Fast Open（实验性）</label>
            <div className="section-title">更多设置（YAML，合并到上面的字段）</div>
            <textarea className="code" rows={6} spellCheck={false} placeholder={"plugin: obfs\nplugin-opts:\n  mode: tls"} value={extra} onChange={(e) => setExtra(e.target.value)} />
          </div>
        </div>
        {error && <div className="error">{error}</div>}
        {result && <div className="muted">{result}</div>}
        <div className="actions" style={{ justifyContent: "flex-end", marginTop: 12 }}>
          <button className="btn" onClick={onClose}>{result ? "关闭" : "取消"}</button>
          {!result && <button className="btn" disabled={!v.name || !v.server || !v.port} onClick={() => add(true)}>完成并测试</button>}
          {!result && <button className="btn primary" disabled={!v.name || !v.server || !v.port} onClick={() => add(false)}>完成</button>}
        </div>
      </div>
    </div>
  );
}
