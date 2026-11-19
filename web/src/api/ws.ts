import type { WsFrame } from './types';

type Handler = (frame: WsFrame) => void;
type StatusListener = (status: WsStatus) => void;
export type WsStatus = 'connecting' | 'open' | 'reconnecting' | 'closed';

const SINCE_KEY = 'pd.ws.since';

function readSince(): string | undefined {
  try {
    return globalThis.localStorage?.getItem(SINCE_KEY) || undefined;
  } catch {
    return undefined;
  }
}
function saveSince(ts: string): void {
  try {
    globalThis.localStorage?.setItem(SINCE_KEY, ts);
  } catch {
    /* ignore */
  }
}

/**
 * Transport-only WebSocket client: reconnect with backoff, persist a
 * "since" cursor and replay it on reconnect so the server can send missed
 * events. Store wiring lives in src/realtime.ts to avoid import cycles.
 */
export class WsClient {
  private socket: WebSocket | null = null;
  private token: string | null = null;
  private since: string | undefined;
  private handlers = new Map<string, Set<Handler>>();
  private statusListeners = new Set<StatusListener>();
  private status: WsStatus = 'closed';
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private attempt = 0;
  private stopped = true;
  private readonly baseDelay = 800;
  private readonly maxDelay = 15_000;

  getStatus = (): WsStatus => this.status;

  onStatus(listener: StatusListener): () => void {
    this.statusListeners.add(listener);
    listener(this.status);
    return () => this.statusListeners.delete(listener);
  }

  on(type: string, handler: Handler): () => void {
    let set = this.handlers.get(type);
    if (!set) {
      set = new Set();
      this.handlers.set(type, set);
    }
    set.add(handler);
    return () => set?.delete(handler);
  }

  private setStatus(status: WsStatus): void {
    this.status = status;
    for (const listener of this.statusListeners) listener(status);
  }

  connect(token: string, since?: string): void {
    this.stopped = false;
    this.token = token;
    this.since = since ?? readSince();
    this.attempt = 0;
    this.openSocket();
  }

  /** Switch user/session without keeping old subscriptions' connection. */
  reconnectWithToken(token: string): void {
    this.stop();
    this.connect(token);
  }

  stop(): void {
    this.stopped = true;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    if (this.heartbeatTimer) clearInterval(this.heartbeatTimer);
    this.reconnectTimer = null;
    this.heartbeatTimer = null;
    if (this.socket) {
      const s = this.socket;
      this.socket = null;
      s.onopen = s.onmessage = s.onerror = s.onclose = null;
      s.close();
    }
    this.setStatus('closed');
  }

  private openSocket(): void {
    if (this.stopped || !this.token || typeof WebSocket === 'undefined') return;
    const base = (import.meta.env?.VITE_API_BASE as string | undefined)?.replace(/\/$/, '') || '';
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = base || location.host;
    const sinceParam = this.since ? `&since=${encodeURIComponent(this.since)}` : '';
    const url = base
      ? `${base.replace(/^http/, 'ws')}/api/ws?token=${encodeURIComponent(this.token)}${sinceParam}`
      : `${proto}//${host}/api/ws?token=${encodeURIComponent(this.token)}${sinceParam}`;
    this.setStatus(this.attempt === 0 ? 'connecting' : 'reconnecting');

    let ws: WebSocket;
    try {
      ws = new WebSocket(url);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.socket = ws;

    ws.onopen = () => {
      this.attempt = 0;
      this.setStatus('open');
      // Ask for a catch-up from our last seen event timestamp.
      ws.send(JSON.stringify({ type: 'subscribe', since: this.since ?? null }));
      if (this.heartbeatTimer) clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'ping' }));
      }, 25_000);
    };

    ws.onmessage = (event) => {
      let frame: WsFrame;
      try {
        frame = JSON.parse(String(event.data)) as WsFrame;
      } catch {
        return;
      }
      if (!frame || typeof frame.type !== 'string') return;
      if (frame.ts) {
        this.since = frame.ts;
        saveSince(frame.ts);
      }
      for (const handler of this.handlers.get(frame.type) ?? []) {
        try {
          handler(frame);
        } catch {
          /* a bad listener must not break the stream */
        }
      }
      for (const handler of this.handlers.get('*') ?? []) {
        try {
          handler(frame);
        } catch {
          /* ignore */
        }
      }
    };

    ws.onerror = () => {
      // onclose will follow and schedule the reconnect.
      try {
        ws.close();
      } catch {
        /* ignore */
      }
    };

    ws.onclose = () => {
      if (this.socket === ws) this.socket = null;
      if (this.heartbeatTimer) clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
      if (!this.stopped) {
        this.setStatus('reconnecting');
        this.scheduleReconnect();
      }
    };
  }

  private scheduleReconnect(): void {
    if (this.stopped || this.reconnectTimer) return;
    const delay = Math.min(this.maxDelay, this.baseDelay * 2 ** this.attempt) + Math.random() * 250;
    this.attempt += 1;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.openSocket();
    }, delay);
  }
}

export const wsClient = new WsClient();
