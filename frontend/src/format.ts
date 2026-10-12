import { useEffect, useState } from "react";

// [value, unit] for big-number displays, e.g. 813 KB/s
export function speed(bytes: number): [string, string] {
  if (bytes < 1000) return [String(bytes), "B/s"];
  if (bytes < 1000 * 1000) return [(bytes / 1000).toFixed(0), "KB/s"];
  return [(bytes / 1000 / 1000).toFixed(1), "MB/s"];
}

export function size(bytes: number): [string, string] {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = bytes, i = 0;
  while (v >= 1000 && i < units.length - 1) { v /= 1000; i++; }
  return [i === 0 ? String(v) : v.toFixed(v < 10 ? 2 : 1), units[i]];
}

export const sizeText = (bytes: number) => size(bytes).join(" ");

// nonNull drops the nulls Go's bindings allow in slices of maps.
export const nonNull = <T,>(a: (T | null)[] | null | undefined): T[] => (a ?? []).filter((x): x is T => x != null);

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
