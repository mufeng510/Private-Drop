import type { Page, Request } from '@playwright/test';

export const user = {
  id: 'u1',
  username: 'tester',
  createdAt: '2026-01-01T00:00:00.000Z',
};

export const device = {
  id: 'd1',
  name: 'Main phone',
  type: 'phone',
  createdAt: '2026-01-01T00:00:00.000Z',
  authorized: true,
};

/** Settings are stored snake_case by the backend and translated in the API layer. */
export const rawSettings: Record<string, string> = {
  retention_days: '30',
  recycle_days: '30',
  max_upload_size: String(512 * 1024 * 1024),
  chunk_size: String(1024 * 1024),
  upload_concurrency: '3',
  session_ttl_days: '30',
  language: 'en',
  link_preview_enabled: 'true',
};

export function attachment(overrides: Record<string, unknown> = {}) {
  return {
    id: 'a1',
    name: 'photo.png',
    mime: 'image/png',
    size: 1024,
    sha256: 'abc',
    favorited: false,
    uploadedAt: '2026-06-01T10:00:00.000Z',
    type: 'image',
    messageIds: ['m1'],
    ...overrides,
  };
}

export function message(overrides: Record<string, unknown> = {}) {
  return {
    id: 'm1',
    text: 'hello from phone',
    pinned: false,
    favorited: false,
    createdAt: '2026-06-01T10:00:00.000Z',
    updatedAt: '2026-06-01T10:00:00.000Z',
    expiresAt: null,
    device,
    attachments: [],
    tags: [],
    links: [],
    ...overrides,
  };
}

export function file(overrides: Record<string, unknown> = {}) {
  return {
    id: 'f1',
    name: 'report.pdf',
    mime: 'application/pdf',
    size: 4096,
    sha256: 'def',
    favorited: false,
    uploadedAt: '2026-06-01T10:00:00.000Z',
    type: 'pdf',
    messageIds: ['m1'],
    ...overrides,
  };
}

/** Seed a signed-in session and force English UI strings. */
export async function seedSession(page: Page, opts: { mfa?: boolean } = {}) {
  await page.addInitScript(({ mfa }) => {
    localStorage.setItem('pd.token', 'e2e-token');
    localStorage.setItem('pd.lang', 'en');
    if (mfa) localStorage.setItem('pd.mfaToken', 'e2e-mfa-token');
  }, { mfa: Boolean(opts.mfa) });
}

export interface MockRoute {
  method: string;
  path: RegExp;
  body?: unknown | ((request: Request, url: URL) => unknown | Promise<unknown>);
  status?: number;
  contentType?: string;
}

/** 1x1 transparent PNG, used for authenticated thumbnail/preview endpoints. */
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/kq8AAAAASUVORK5CYII=',
  'base64',
);

export async function mockApi(page: Page, routes: MockRoute[] = []) {
  const all = [
    ...routes,
    { method: 'GET', path: /^\/api\/auth\/status$/, body: { initialized: true, lanOnly: false } },
    { method: 'GET', path: /^\/api\/auth\/me$/, body: { user, device, settings: rawSettings } },
    { method: 'GET', path: /^\/api\/notifications$/, body: { items: [], unread: 0 } },
    { method: 'GET', path: /^\/api\/tags$/, body: { items: [] } },
    { method: 'GET', path: /^\/api\/messages$/, body: { items: [], nextCursor: null } },
    { method: 'GET', path: /^\/api\/files$/, body: { items: [], nextCursor: null } },
    { method: 'GET', path: /^\/api\/recycle$/, body: { messages: [], files: [] } },
    { method: 'GET', path: /^\/api\/search$/, body: { items: [], nextCursor: null } },
    { method: 'GET', path: /^\/api\/uploads$/, body: { items: [] } },
    { method: 'GET', path: /^\/api\/settings$/, body: rawSettings },
    { method: 'GET', path: /^\/api\/files\/[^/]+\/(thumbnail|preview)$/, body: PNG, contentType: 'image/png' },
  ];
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const match = all.find((r) => r.method === request.method() && r.path.test(url.pathname));
    if (!match) {
      await route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'NOT_MOCKED', message: `${request.method()} ${url.pathname}` } }),
      });
      return;
    }
    const raw = typeof match.body === 'function' ? await match.body(request, url) : match.body;
    await route.fulfill({
      status: match.status ?? 200,
      contentType: match.contentType ?? 'application/json',
      body: Buffer.isBuffer(raw) ? raw : JSON.stringify(raw ?? {}),
    });
  });
}

export function jsonBody(request: Request): Record<string, unknown> {
  try {
    return JSON.parse(request.postData() || '{}') as Record<string, unknown>;
  } catch {
    return {};
  }
}
