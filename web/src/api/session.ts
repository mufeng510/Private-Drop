// Holds auth material outside React/Zustand so the API client never needs to
// import a store (avoids circular dependencies), and so downloads/WS can read
// the current token synchronously.

const TOKEN_KEY = 'pd.token';
const MFA_KEY = 'pd.mfaToken';
const DEVICE_KEY = 'pd.deviceKey';

function read(key: string): string | null {
  try {
    return globalThis.localStorage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}
function write(key: string, value: string | null): void {
  try {
    if (value) globalThis.localStorage?.setItem(key, value);
    else globalThis.localStorage?.removeItem(key);
  } catch {
    /* storage unavailable (private mode / SSR) */
  }
}

let token: string | null = read(TOKEN_KEY);
let mfaToken: string | null = read(MFA_KEY);

// Notified when the server rejects a stored MFA token (expired or revoked).
// The auth store uses this to drop back to the verification gate. Registered
// from the store so this module stays free of store/API imports (no cycles).
let onMfaInvalidated: (() => void) | null = null;
export function setMfaInvalidatedHandler(fn: () => void): void {
  onMfaInvalidated = fn;
}

/** Stable device identity. Never use the editable display name for this. */
export function getDeviceKey(): string {
  let key = read(DEVICE_KEY);
  if (!key) {
    const c = globalThis.crypto;
    if (c?.randomUUID) {
      key = c.randomUUID();
    } else if (c?.getRandomValues) {
      const b = c.getRandomValues(new Uint8Array(16));
      key = Array.from(b, (n) => n.toString(16).padStart(2, '0')).join('');
    } else {
      key = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
    }
    write(DEVICE_KEY, key);
  }
  return key;
}

export const session = {
  getToken: () => token,
  setToken: (value: string | null) => {
    token = value;
    write(TOKEN_KEY, value);
  },
  getMfaToken: () => mfaToken,
  setMfaToken: (value: string | null) => {
    mfaToken = value;
    write(MFA_KEY, value);
  },
  /** Discard a token the server no longer accepts and notify the UI. */
  invalidateMfa: () => {
    mfaToken = null;
    write(MFA_KEY, null);
    onMfaInvalidated?.();
  },
  clear: () => {
    token = null;
    mfaToken = null;
    write(TOKEN_KEY, null);
    write(MFA_KEY, null);
  },
};

export function authHeaders(opts: { auth?: boolean; mfa?: boolean } = {}): HeadersInit {
  const headers: Record<string, string> = {};
  // Sent on every request (including login/init) so the server can recognize
  // this device without relying on the user-editable device name.
  headers['X-Device-Key'] = getDeviceKey();
  if (opts.auth !== false) {
    const t = session.getToken();
    if (t) headers.Authorization = `Bearer ${t}`;
  }
  if (opts.mfa) {
    const m = session.getMfaToken();
    if (m) headers['X-MFA-Token'] = m;
  }
  return headers;
}
