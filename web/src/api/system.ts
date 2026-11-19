import { request } from './client';
import type { DiagnoseResult, StorageStats, SystemInfo } from './types';

export const systemApi = {
  storage: () => request<StorageStats>('/api/storage/stats', { mfa: true }),
  info: () => request<SystemInfo>('/api/system/info', { mfa: true }),
  // Backend returns a flat {check: result} map; normalize it for the UI list.
  diagnose: async (): Promise<DiagnoseResult> => {
    const raw = await request<Record<string, unknown>>('/api/system/diagnose', { mfa: true });
    const checks = Object.entries(raw).map(([name, value]) => {
      const ok = typeof value !== 'string' || value === 'ok';
      return {
        name,
        ok,
        message: typeof value === 'string' && value !== 'ok' ? value : undefined,
      };
    });
    return {
      ok: checks.every((c) => c.ok),
      checks,
      checkedAt: new Date().toISOString(),
    };
  },
};
