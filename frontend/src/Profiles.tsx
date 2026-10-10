import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { ProfileService, SettingsService } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import type { Profiles as ProfilesData } from "../bindings/github.com/ryanzhangtianran/Clash-One/internal/app";
import { sizeText } from "./format";

const host = (url: string) => { try { return new URL(url).host; } catch { return url; } };

// ago is a short relative time, e.g. 刚刚, 5 分钟前, 3 天前.
function ago(iso: string) {
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (!(s >= 0)) return "—";
  if (s < 60) return "刚刚";
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`;
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`;
  return `${Math.floor(s / 86400)} 天前`;
}

// Profiles lists subscriptions and local configs (click to select, double-click to use), the
// override merged on top, and where it is all stored.
export default function Profiles() {
  const [data, setData] = useState<ProfilesData>({ current: "", profiles: [] });
  const [sel, setSel] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [link, setLink] = useState("");
  const [override, setOverride] = useState("");
  const [savedOverride, setSavedOverride] = useState("");
  const [dataDir, setDataDir] = useState("");

  const load = useCallback(() => ProfileService.Profiles().then(setData), []);
  useEffect(() => {
    load();
    ProfileService.Override().then((o) => { setOverride(o); setSavedOverride(o); });
    SettingsService.About().then((a) => setDataDir(a.dataDir));
    return Events.On("profiles", (e) => setData(e.data));
  }, [load]);

  const run = (key: string, action: () => Promise<unknown>) => {
    setBusy(key);
    setError("");
    action().catch((e) => setError(String(e))).finally(() => setBusy(""));
  };
  const list = data.profiles ?? [];
  const selected = list.find((p) => p.id === sel);
  const use = (id: string) => id !== data.current && run("use", () => ProfileService.Select(id));
  const add = () => run("add", () => ProfileService.Add(name, link).then(() => { setName(""); setLink(""); setAdding(false); }));

  return (
    <>
      {error && <div className="card error" style={{ marginTop: 16 }}>{error}</div>}

      <div className="group-title">配置列表</div>
      <div className="plist">
        {list.length === 0 && !adding && <div className="plist-empty muted">还没有配置，从 URL 添加订阅或导入文件。</div>}
        {list.map((p) => {
          const used = p.usage ? p.usage.upload + p.usage.download : 0;
          const total = p.usage?.total ?? 0;
          return (
            <div key={p.id} className={`plist-row ${p.id === sel ? "selected" : ""}`} onClick={() => setSel(p.id)} onDoubleClick={() => use(p.id)}>
              <span className="plist-check">{p.id === data.current ? "✓" : ""}</span>
              <div className="plist-main">
                <div className="plist-name">{p.name}</div>
                <div className="muted small">
                  {p.url ? host(p.url) : "本地"} · 更新于 {ago(p.updatedAt)}{p.interval > 0 && ` · 每 ${p.interval} 小时自动更新`}
                </div>
              </div>
              {total > 0 && (
                <div className="plist-usage">
                  <div className="small">{sizeText(used)} / {sizeText(total)}</div>
                  <div className="plist-meter"><i style={{ width: `${Math.min(100, (used / total) * 100)}%` }} /></div>
                  {(p.usage?.expire ?? 0) > 0 && <div className="muted small">{new Date(p.usage!.expire * 1000).toLocaleDateString()} 到期</div>}
                </div>
              )}
            </div>
          );
        })}
        {adding && (
          <div className="plist-add">
            <input placeholder="名称（可选）" value={name} onChange={(e) => setName(e.target.value)} style={{ width: 130 }} />
            <input placeholder="订阅链接 https://…" value={link} autoFocus onChange={(e) => setLink(e.target.value)} onKeyDown={(e) => e.key === "Enter" && link && add()} style={{ flex: 1 }} />
            <button className="btn primary" disabled={!link || !!busy} onClick={add}>{busy === "add" ? "下载中…" : "添加"}</button>
            <button className="btn" onClick={() => setAdding(false)}>取消</button>
          </div>
        )}
        <div className="plist-tools">
          <button className="btn" title="从 URL 添加订阅" onClick={() => setAdding(true)}>＋</button>
          <button className="btn" title="删除所选配置" disabled={!selected || !!busy} onClick={() => selected && run("remove", () => ProfileService.Remove(selected.id).then(() => setSel("")))}>－</button>
          <button className="btn" disabled={!selected?.url || !!busy} onClick={() => selected && run("update", () => ProfileService.Update(selected.id))}>{busy === "update" ? "更新中…" : "更新"}</button>
          <button className="btn" disabled={!!busy} onClick={() => run("import", () => ProfileService.ImportFile())}>导入…</button>
          <span className="grow" />
          <button className="btn" disabled={!selected || selected.id === data.current || !!busy} onClick={() => selected && use(selected.id)}>{busy === "use" ? "切换中…" : "使用"}</button>
        </div>
      </div>

      <div className="group-title">覆写</div>
      <div className="plist">
        <textarea className="code override" rows={10} spellCheck={false} value={override} onChange={(e) => setOverride(e.target.value)}
          placeholder={"合并到当前配置之上，例如：\nprepend-rules:\n  - DOMAIN-SUFFIX,corp.example.com,DIRECT\nremove-proxy-groups:\n  - 不需要的策略组"} />
        <div className="plist-tools">
          <span className="muted small">内核校验不通过时不会生效</span>
          <span className="grow" />
          <button className="btn" disabled={override === savedOverride || !!busy} onClick={() => setOverride(savedOverride)}>还原</button>
          <button className="btn primary" disabled={override === savedOverride || !!busy} onClick={() => run("override", () => ProfileService.SetOverride(override).then(() => setSavedOverride(override)))}>
            {busy === "override" ? "应用中…" : "保存并应用"}
          </button>
        </div>
      </div>

      <div className="group-title">配置存储</div>
      <div className="plist">
        <div className="plist-row static">
          <div className="plist-main"><div className="plist-name">数据目录</div><div className="muted small selectable">{dataDir}</div></div>
          <button className="btn" onClick={() => SettingsService.OpenDataDir()}>在 Finder 中显示</button>
        </div>
      </div>
    </>
  );
}
