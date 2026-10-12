import { Component, type ReactNode } from "react";
import { CoreService } from "../bindings/github.com/ryanzhangtianran/Connaio/internal/app";

const report = (msg: string) => { CoreService.ReportError(msg).catch(() => {}); };

// errors outside React's render (handlers, effects' promises) land in the log too
window.addEventListener("error", (e) => report(`error: ${e.message}\n${e.error?.stack ?? ""}`));
window.addEventListener("unhandledrejection", (e) => report(`unhandled rejection: ${e.reason?.stack ?? e.reason}`));

// ErrorBoundary shows a page's crash instead of blanking the window, and logs it.
export default class ErrorBoundary extends Component<{ children: ReactNode; onReset?: () => void }, { error: Error | null }> {
  state = { error: null as Error | null };
  static getDerivedStateFromError(error: Error) { return { error }; }
  componentDidCatch(error: Error, info: { componentStack?: string | null }) {
    report(`render: ${error.name}: ${error.message}\n${error.stack ?? ""}\n${info.componentStack ?? ""}`);
  }
  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="card error" style={{ whiteSpace: "pre-wrap" }}>
        页面出错了（已记录到 gui-errors.log）：{"\n"}{String(this.state.error)}
        <div style={{ marginTop: 10 }}>
          <button className="btn" onClick={() => { this.setState({ error: null }); this.props.onReset?.(); }}>返回</button>
        </div>
      </div>
    );
  }
}
