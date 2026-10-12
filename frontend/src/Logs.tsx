import { useState } from "react";
import type { LogLine } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";

const levels = ["debug", "info", "warning", "error"];

export default function Logs({ lines, onClear }: { lines: LogLine[]; onClear: () => void }) {
  const [level, setLevel] = useState("info");
  const [filter, setFilter] = useState("");
  const min = levels.indexOf(level);
  const q = filter.toLowerCase();
  const shown = lines.filter((l) => levels.indexOf(l.type) >= min && (!q || l.payload.toLowerCase().includes(q)));
  return (
    <>
      <div className="card">
        <div className="head">
          <div className="seg">
            {levels.slice(1).map((l) => <button key={l} className={l === level ? "on" : ""} onClick={() => setLevel(l)}>{l}</button>)}
          </div>
          <div className="actions">
            <input placeholder="搜索…" value={filter} onChange={(e) => setFilter(e.target.value)} style={{ width: 200 }} />
            <button className="btn" onClick={onClear}>清空</button>
          </div>
        </div>
        <div className="logs">
          {shown.slice().reverse().map((l, i) => (
            <div key={i} className={`log ${l.type}`}><span className="lvl">{l.type}</span>{l.payload}</div>
          ))}
        </div>
      </div>
    </>
  );
}
