import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";

// on subscribes to an event the Go side declares (application.RegisterEvent), so a misspelt name
// fails to compile and the callback gets the typed data; it returns the unsubscribe.
export const on = <K extends keyof Events.CustomEvents>(name: K, cb: (data: Events.CustomEvents[K]) => void) =>
  Events.On(name, (e) => cb(e.data));

// usePoll calls fn now and every ms while enabled (the latest fn, so it may close over state).
export function usePoll(fn: () => void, ms: number, enabled = true) {
  const latest = useRef(fn);
  latest.current = fn;
  useEffect(() => {
    if (!enabled) return;
    latest.current();
    const t = setInterval(() => latest.current(), ms);
    return () => clearInterval(t);
  }, [ms, enabled]);
}

// useCachedURL is fetch(key)'s data URL (an icon from Go), fetched once per key for the app's lifetime.
const urls = new Map<string, string>();
export function useCachedURL(key: string | undefined, fetch: (key: string) => Promise<string>) {
  const [src, setSrc] = useState(key ? urls.get(key) : undefined);
  useEffect(() => {
    if (!key) return;
    if (urls.has(key)) return setSrc(urls.get(key));
    fetch(key).then((u) => { urls.set(key, u); setSrc(u); });
  }, [key]);
  return src;
}
