import { authHeaders } from '../api/session';

const BASE = (import.meta.env?.VITE_API_BASE as string | undefined)?.replace(/\/$/, '') || '';
const cache = new Map<string, Promise<string>>();
const created = new Set<string>();

/**
 * Fetch an authenticated binary resource and return an object URL suitable
 * for <img src> / <video src>. <img> cannot send Authorization headers, so a
 * blob URL is required. Promise results are de-duplicated per path.
 */
export function getAuthBlobUrl(path: string, signal?: AbortSignal): Promise<string> {
  const key = path;
  const existing = cache.get(key);
  if (existing) return existing;
  const p = fetch(`${BASE}${path}`, {
    headers: authHeaders(),
    credentials: 'same-origin',
    signal,
  })
    .then(async (res) => {
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      created.add(url);
      return url;
    })
    .catch((e) => {
      cache.delete(key);
      throw e;
    });
  cache.set(key, p);
  return p;
}

export function clearAuthBlobUrl(path: string): void {
  const p = cache.get(path);
  cache.delete(path);
  if (p) void p.then((url) => URL.revokeObjectURL(url)).catch(() => undefined);
}

export function revokeAllAuthBlobUrls(): void {
  for (const url of created) URL.revokeObjectURL(url);
  created.clear();
  cache.clear();
}

export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename || 'download';
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
}

/** Authenticated download of an arbitrary endpoint (single file / ZIP). */
export async function downloadPath(
  path: string,
  filename: string,
  options: { method?: 'GET' | 'POST'; body?: unknown; signal?: AbortSignal } = {},
): Promise<void> {
  const headers = new Headers(authHeaders());
  const body = options.body !== undefined ? JSON.stringify(options.body) : undefined;
  if (body !== undefined) headers.set('Content-Type', 'application/json');
  const res = await fetch(`${BASE}${path}`, {
    method: options.method ?? 'GET',
    headers,
    body,
    credentials: 'same-origin',
    signal: options.signal,
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  downloadBlob(await res.blob(), filename);
}
