import { request } from './client';
import type { Settings } from './types';

const MB = 1024 * 1024;

function toBool(v: unknown, fallback: boolean): boolean {
  if (v === undefined || v === null) return fallback;
  return v === true || v === 1 || v === '1' || v === 'true';
}

function toNum(v: unknown, fallback: number): number {
  const n = Number(v ?? fallback);
  return Number.isFinite(n) ? n : fallback;
}

/**
 * The backend stores settings as snake_case strings (bytes for sizes).
 * The admin UI works with typed camelCase values (MB for sizes). Translate at
 * the API boundary so neither side needs to know about the other's shape.
 */
function fromBackend(raw: Record<string, string>): Settings {
  return {
    retentionDays: toNum(raw.retention_days, 30),
    recycleDays: toNum(raw.recycle_days, 30),
    maxFileSizeMb: Math.round(toNum(raw.max_upload_size, 512 * MB) / MB),
    chunkSizeMb: Math.round(toNum(raw.chunk_size, MB) / MB),
    uploadConcurrency: toNum(raw.upload_concurrency, 3),
    storageWarnPercent: toNum(raw.storage_warning_percent, 90),
    devicePolicy: raw.device_policy ?? 'open',
    linkPreviewEnabled: toBool(raw.link_preview_enabled, true),
    sessionDuration: toNum(raw.session_ttl_days, 30),
    loginAttemptLimit: toNum(raw.login_attempt_limit, 5),
    language: raw.language ?? 'zh',
    notifyNewDevice: toBool(raw.notify_new_device, true),
    notifyUploadDone: toBool(raw.notify_upload_done, true),
    notifyUploadFailed: toBool(raw.notify_upload_failed, true),
    notifyLowStorage: toBool(raw.notify_low_storage, true),
    notifyExpiring: toBool(raw.notify_expiring, true),
  };
}

const KEY_MAP: Record<string, string> = {
  retentionDays: 'retention_days',
  recycleDays: 'recycle_days',
  uploadConcurrency: 'upload_concurrency',
  storageWarnPercent: 'storage_warning_percent',
  devicePolicy: 'device_policy',
  language: 'language',
  sessionDuration: 'session_ttl_days',
  loginAttemptLimit: 'login_attempt_limit',
  notifyNewDevice: 'notify_new_device',
  notifyUploadDone: 'notify_upload_done',
  notifyUploadFailed: 'notify_upload_failed',
  notifyLowStorage: 'notify_low_storage',
  notifyExpiring: 'notify_expiring',
};

function toBackend(values: Settings): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(values)) {
    if (value === undefined || value === null) continue;
    if (key === 'maxFileSizeMb') {
      out.max_upload_size = String(Math.round(toNum(value, 512) * MB));
    } else if (key === 'chunkSizeMb') {
      out.chunk_size = String(Math.round(toNum(value, 1) * MB));
    } else if (key === 'linkPreviewEnabled') {
      out.link_preview_enabled = toBool(value, true) ? 'true' : 'false';
    } else {
      out[KEY_MAP[key] ?? key] = String(value);
    }
  }
  return out;
}

export const settingsApi = {
  // All settings endpoints require the MFA token header.
  list: async (): Promise<Settings> =>
    fromBackend(await request<Record<string, string>>('/api/settings', { mfa: true })),
  update: async (values: Settings): Promise<Settings> => {
    await request<{ changed: string[] }>('/api/settings', {
      method: 'PATCH',
      body: toBackend(values),
      mfa: true,
    });
    // The PATCH response only lists changed keys; re-read for the merged view.
    return settingsApi.list();
  },
  security: () => request<Settings>('/api/settings/security', { mfa: true }),
  password: (oldPassword: string, newPassword: string) =>
    request<Record<string, never>>('/api/settings/password', {
      method: 'POST',
      body: { oldPassword, newPassword },
      mfa: true,
    }),
};
