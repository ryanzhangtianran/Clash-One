import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";

// on subscribes to an event the Go side declares (application.RegisterEvent), so a misspelt name
// fails to compile and the callback gets the typed data; it returns the unsubscribe.
export const on = <K extends keyof Events.CustomEvents>(name: K, cb: (data: Events.CustomEvents[K]) => void) =>
  Events.On(name, (e) => cb(e.data));

// usePoll calls fn now and again ms after each call settles while enabled (the latest fn, so it may
// close over state). Runs never overlap, and none start while the document is hidden; the first
// one after it becomes visible again runs at once.
export function usePoll(fn: () => void | Promise<unknown>, ms: number, enabled = true) {
  const latest = useRef(fn);
  latest.current = fn;
  useEffect(() => {
    if (!enabled) return;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;
    const run = () => {
      if (stopped || document.hidden) return;
      Promise.resolve(latest.current()).finally(() => { if (!stopped) timer = setTimeout(run, ms); });
    };
    const visible = () => { if (!document.hidden) { clearTimeout(timer); run(); } };
    document.addEventListener("visibilitychange", visible);
    run();
    return () => { stopped = true; clearTimeout(timer); document.removeEventListener("visibilitychange", visible); };
  }, [ms, enabled]);
}

// useClickAway calls close on any document click while open: menus and popovers that dismiss on
// a click elsewhere; the opener stops its own click's propagation.
export function useClickAway(open: boolean, close: () => void) {
  useEffect(() => {
    if (!open) return;
    document.addEventListener("click", close);
    return () => document.removeEventListener("click", close);
  }, [open]);
}

// useCachedURL is fetch(key)'s data URL (an icon from Go), fetched once per key for the app's lifetime.
const urls = new Map<string, Promise<string>>();
export function useCachedURL(key: string | undefined, fetch: (key: string) => Promise<string>) {
  const [src, setSrc] = useState<string>();
  useEffect(() => {
    if (!key) return;
    let alive = true;
    if (!urls.has(key)) urls.set(key, fetch(key).catch(() => ""));
    urls.get(key)!.then((u) => alive && setSrc(u));
    return () => { alive = false; };
  }, [key]);
  return src;
}

// useDirty is true once any of deps has changed since mount: a form with edits to protect.
export function useDirty(deps: unknown[]) {
  const initial = useRef(deps); // compared, not counted: StrictMode runs the mount effect twice
  return deps.some((d, i) => !Object.is(d, initial.current[i]));
}
