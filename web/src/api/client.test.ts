import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, errorMessage, request } from './client';

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

describe('ApiError / errorMessage', () => {
  it('normalizes errors to readable messages', () => {
    expect(errorMessage(new ApiError('bad', 'BAD', 400))).toBe('bad');
    expect(errorMessage(new Error('boom'))).toBe('boom');
    expect(errorMessage('oops')).toBe('oops');
    expect(errorMessage(42)).toBe('Unknown error');
  });
});

describe('request', () => {
  it('parses the contract error envelope', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ error: { code: 'UNAUTHORIZED', message: '请先登录' } }, 401)),
    );
    await expect(request('/api/messages')).rejects.toMatchObject({
      code: 'UNAUTHORIZED',
      message: '请先登录',
      status: 401,
    });
  });

  it('returns parsed JSON on success', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ items: [], nextCursor: null }));
    vi.stubGlobal('fetch', fetchMock);
    const res = await request<{ items: unknown[] }>('/api/messages');
    expect(res.items).toEqual([]);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/messages');
    expect(init.method).toBe('GET');
  });

  it('serializes query params and skips nullish values', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}));
    vi.stubGlobal('fetch', fetchMock);
    await request('/api/messages', {
      query: { sort: 'newest', limit: 30, favorite: false, tag: undefined, from: null },
    });
    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('sort=newest');
    expect(url).toContain('limit=30');
    expect(url).toContain('favorite=false');
    expect(url).not.toContain('tag=');
    expect(url).not.toContain('from=');
  });

  it('maps network failures to NETWORK errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('failed to fetch')));
    await expect(request('/api/auth/me')).rejects.toMatchObject({ code: 'NETWORK' });
  });

  it('handles 204 no-content', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })));
    await expect(request('/api/auth/logout', { method: 'POST' })).resolves.toBeUndefined();
  });
});
