import { createHmac } from 'node:crypto';

export const BASE = process.env.REAL_BASE_URL || 'http://localhost:18081';
export const USERNAME = 'admin';
export const PASSWORD = 'StrongPass123!';

export interface Session { token: string; mfaToken: string; deviceKey: string }
interface ApiClient {
  addInitScript: (script: (arg: Session) => void, arg: Session) => Promise<unknown>;
}

const newKey = () => 'e2e-device-' + Math.random().toString(36).slice(2);

export async function api(path: string, init: RequestInit = {}): Promise<Record<string, unknown>> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init.headers || {}) },
  });
  const text = await res.text();
  let body: Record<string, unknown> = {};
  try { body = text ? JSON.parse(text) as Record<string, unknown> : {}; } catch { /* non-JSON */ }
  if (!res.ok) throw new Error(`${path} -> ${res.status}: ${text.slice(0, 200)}`);
  return body;
}

let ready: Session | undefined;

/** Idempotent first-run setup: initialize once, then log in with a fresh device. */
export async function ensureAdmin(): Promise<Session> {
  if (ready) return ready;
  const status = await api('/api/auth/status');
  if (!status.initialized) {
    await api('/api/auth/init', {
      method: 'POST',
      headers: { 'X-Device-Key': newKey() },
      body: JSON.stringify({ username: USERNAME, password: PASSWORD, deviceName: 'E2E Admin', policy: 'open' }),
    });
  }
  const deviceKey = newKey();
  const res = await api('/api/auth/login', {
    method: 'POST',
    headers: { 'X-Device-Key': deviceKey },
    body: JSON.stringify({ username: USERNAME, password: PASSWORD, deviceName: 'E2E Admin Browser' }),
  });
  // Once a second factor exists, login withholds the bootstrap MFA grant and
  // returns mfaRequired. Tests that need admin access verify via TOTP/passkey.
  ready = { token: String(res.token || ''), mfaToken: String(res.mfaToken || ''), deviceKey };
  return ready;
}

/** Each login must carry its own device key: device identity is never the display name. */
export async function login(deviceName: string): Promise<Session> {
  const deviceKey = newKey();
  const res = await api('/api/auth/login', {
    method: 'POST',
    headers: { 'X-Device-Key': deviceKey },
    body: JSON.stringify({ username: USERNAME, password: PASSWORD, deviceName }),
  });
  return { token: String(res.token || ''), mfaToken: String(res.mfaToken || ''), deviceKey };
}

export async function injectSession(context: ApiClient, session: Session): Promise<void> {
  // The sessionStorage flag lets a test deliberately drop the session before
  // re-navigating (init scripts run on every navigation).
  await context.addInitScript((s) => {
    if (sessionStorage.getItem('pd.no-restore')) return;
    localStorage.setItem('pd.token', s.token);
    localStorage.setItem('pd.mfaToken', s.mfaToken);
    localStorage.setItem('pd.deviceKey', s.deviceKey);
  }, session);
}

const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

function base32Decode(input: string): Buffer {
  const s = input.toUpperCase().replace(/=+$/, '').replace(/[^A-Z2-7]/g, '');
  let bits = '';
  for (const ch of s) bits += B32.indexOf(ch).toString(2).padStart(5, '0');
  const bytes: number[] = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) bytes.push(parseInt(bits.slice(i, i + 8), 2));
  return Buffer.from(bytes);
}

/** RFC 6238 TOTP (SHA-1, 6 digits, 30-second step). */
export function totpCode(secret: string): string {
  const counter = Math.floor(Date.now() / 30_000);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(counter));
  const h = createHmac('sha1', base32Decode(secret)).update(buf).digest();
  const off = h[h.length - 1] & 0x0f;
  const n = ((h[off] & 0x7f) << 24) | ((h[off + 1] & 0xff) << 16) | ((h[off + 2] & 0xff) << 8) | (h[off + 3] & 0xff);
  return String(n % 1_000_000).padStart(6, '0');
}
