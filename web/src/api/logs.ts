import { request } from './client';
import type { AuditEntry, LogEntry, Paginated } from './types';

export type LogType = 'app' | 'upload' | 'clean' | 'error' | 'security';

export const logsApi = {
  list: (type?: LogType, params: { cursor?: string; limit?: number } = {}) =>
    request<Paginated<LogEntry>>('/api/logs', { query: { type, ...params }, mfa: true }),
  audit: (params: { cursor?: string; limit?: number } = {}) =>
    request<Paginated<AuditEntry>>('/api/audit', { query: { ...params }, mfa: true }),
};
