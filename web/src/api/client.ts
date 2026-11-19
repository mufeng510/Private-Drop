import type { ApiErrorBody } from './types';
import { authHeaders } from './session';

const BASE = (import.meta.env?.VITE_API_BASE as string | undefined)?.replace(/\/$/, '') || '';

export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  constructor(message: string, code = 'UNKNOWN', status = 0) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

/** Normalize any thrown value into a user-presentable message. */
export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  if (typeof e === 'string') return e;
  return 'Unknown error';
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  /** Raw binary body for chunk uploads. */
  rawBody?: Blob | ArrayBuffer | Uint8Array;
  query?: Record<string, string | number | boolean | null | undefined>;
  signal?: AbortSignal;
  /** Skip the Bearer header (login/init/status). */
  auth?: boolean;
  /** Include the MFA token header (admin settings). */
  mfa?: boolean;
  headers?: Record<string, string>;
}

function buildUrl(path: string, query?: RequestOptions['query']): string {
  const url = `${BASE}${path.startsWith('/') ? path : `/${path}`}`;
  if (!query) return url;
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === null || value === undefined || value === '') continue;
    params.set(key, String(value));
  }
  const qs = params.toString();
  return qs ? `${url}?${qs}` : url;
}

async function parseError(res: Response): Promise<ApiError> {
  let code = 'HTTP_ERROR';
  let message = `${res.status} ${res.statusText}`;
  try {
    const text = await res.text();
    if (text) {
      const parsed = JSON.parse(text) as ApiErrorBody;
      if (parsed?.error) {
        code = parsed.error.code || code;
        message = parsed.error.message || message;
      }
    }
  } catch {
    /* non-JSON error body */
  }
  return new ApiError(message, code, res.status);
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, rawBody, query, signal, auth = true, mfa = false, headers } = options;
  const finalHeaders = new Headers(authHeaders({ auth, mfa }));
  if (headers) for (const [k, v] of Object.entries(headers)) finalHeaders.set(k, v);
  if (body !== undefined && !finalHeaders.has('Content-Type')) {
    finalHeaders.set('Content-Type', 'application/json');
  }
  if (rawBody !== undefined && !finalHeaders.has('Content-Type')) {
    finalHeaders.set('Content-Type', 'application/octet-stream');
  }

  let res: Response;
  try {
    res = await fetch(buildUrl(path, query), {
      method,
      headers: finalHeaders,
      body: rawBody !== undefined ? (rawBody as BodyInit) : body !== undefined ? JSON.stringify(body) : undefined,
      signal,
      credentials: 'same-origin',
    });
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') throw e;
    throw new ApiError(`Network error: ${errorMessage(e)}`, 'NETWORK', 0);
  }

  if (!res.ok) throw await parseError(res);
  if (res.status === 204) return undefined as T;

  const contentType = res.headers.get('content-type') || '';
  if (!contentType.includes('application/json')) {
    return (await res.text()) as T;
  }
  return (await res.json()) as T;
}

/** Binary fetch used for authenticated images, previews and downloads. */
export async function fetchBinary(path: string, options: RequestOptions = {}): Promise<Blob> {
  const headers = new Headers(authHeaders({ auth: options.auth, mfa: options.mfa }));
  if (options.headers) for (const [k, v] of Object.entries(options.headers)) headers.set(k, v);
  let res: Response;
  try {
    res = await fetch(buildUrl(path, options.query), {
      method: options.method ?? 'GET',
      headers,
      body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
      signal: options.signal,
      credentials: 'same-origin',
    });
  } catch (e) {
    throw new ApiError(`Network error: ${errorMessage(e)}`, 'NETWORK', 0);
  }
  if (!res.ok) throw await parseError(res);
  return res.blob();
}

export const api = { request, fetchBinary, ApiError, errorMessage };
