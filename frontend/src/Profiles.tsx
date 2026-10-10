import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { ProfileService } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import type { Profiles as ProfilesData } from "../bindings/github.com/ryanzhangtianran/Clash-One";
import { sizeText } from "./format";

const host = (url: string) => { try { return new URL(url).host; } catch { return url; } };
const date = (unix: number) => new Date(unix * 1000).toLocaleDateString();

export default function Profiles() {
  const [data, setData] = useState<ProfilesData>({ current: "", profiles: [] });
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [link, setLink] = useState("");
  const [local, setLocal] = useState("");
  const [override, setOverride] = useState("");
  const [savedOverride, setSavedOverride] = useState("");

  const load = useCallback(() => ProfileService.Profiles().then(setData), []);
  useEffect(() => {
    load();
    ProfileService.Override().then((o) => { setOverride(o); setSavedOverride(o); });
    return Events.On("profiles", (e) => setData(e.data));
  }, [load]);

  // run wraps an action: one at a time, errors shown inline
  const run = (key: string, action: () => Promise<unknown>) => {
    setBusy(key);
    setError("");
    action().catch((e) => setError(String(e))).finally(() => setBusy(""));
  };

  return (
    <>
      <h1 className="page-title">配置</h1>
      {error && <div className="card error" style={{ marginBottom: 16 }}>{error}</div>}

      <div className="card" style={{ marginBottom: 16 }}>
        <div className="label">订阅</div>
        {data.profiles?.length ? null : <div className="muted" style={{ marginTop: 8 }}>还没有配置，添加一个订阅开始使用。</div>}
        <div className="rank" style={{ marginTop: 10 }}>
          {(data.profiles ?? []).map((p) => {
            const used = p.usage ? p.usage.upload + p.usage.download : 0;
            return (
              <div key={p.id} className="profile-row">
                <div>
                  <div className="name">{p.name} {p.id === data.current && <span className="tag">当前</span>}</div>
                  <div className="muted small">
                    {p.url ? host(p.url) : "本地"} · 更新于 {new Date(p.updatedAt).toLocaleString()}
                    {p.interval > 0 && ` · 每 ${p.interval} 小时自动更新`}
                    {p.usage && p.usage.total > 0 && ` · 已用 ${sizeText(used)} / ${sizeText(p.usage.total)}`}
                    {p.usage && p.usage.expire > 0 && ` · ${date(p.usage.expire)} 到期`}
                  </div>
                </div>
                <div className="actions">
                  {p.id !== data.current && <button className="btn" disabled={!!busy} onClick={() => run(p.id, () => ProfileService.Select(p.id))}>使用</button>}
                  {p.url && <button className="btn" disabled={!!busy} onClick={() => run(p.id, () => ProfileService.Update(p.id))}>{busy === p.id ? "更新中…" : "更新"}</button>}
                  <button className="btn" disabled={!!busy} onClick={() => run(p.id, () => ProfileService.Remove(p.id))}>删除</button>
                </div>
              </div>
            );
          })}
        </div>
        <div className="form">
          <input placeholder="名称" value={name} onChange={(e) => setName(e.target.value)} style={{ width: 140 }} />
          <input placeholder="订阅链接 https://…" value={link} onChange={(e) => setLink(e.target.value)} style={{ flex: 1 }} />
          <button className="btn" disabled={!link || !!busy} onClick={() => run("add", () => ProfileService.Add(name, link).then(() => { setName(""); setLink(""); }))}>
            {busy === "add" ? "下载中…" : "添加订阅"}
          </button>
        </div>
        <details style={{ marginTop: 10 }}>
          <summary className="muted">粘贴本地配置</summary>
          <textarea className="code" rows={6} placeholder="Clash YAML（需包含 proxies 或 proxy-providers）" value={local} onChange={(e) => setLocal(e.target.value)} />
          <button className="btn" disabled={!local || !!busy} onClick={() => run("local", () => ProfileService.AddLocal(name || "本地配置", local).then(() => setLocal("")))}>添加</button>
        </details>
      </div>

      <div className="card">
        <div className="head">
          <div className="label">覆写（合并到当前配置之上）</div>
          <button className="btn" disabled={override === savedOverride || !!busy} onClick={() => run("override", () => ProfileService.SetOverride(override).then(() => setSavedOverride(override)))}>
            {busy === "override" ? "应用中…" : "保存并应用"}
          </button>
        </div>
        <div className="muted small" style={{ margin: "6px 0" }}>
          字典深度合并、其他值替换；prepend-rules / append-rules、prepend-proxies、prepend-proxy-groups 在订阅列表前后追加。内核校验不通过时不会生效。
        </div>
        <textarea className="code" rows={12} spellCheck={false} value={override} onChange={(e) => setOverride(e.target.value)}
          placeholder={"mode: rule\nprepend-rules:\n  - DOMAIN-SUFFIX,corp.example.com,DIRECT"} />
      </div>
    </>
  );
}
