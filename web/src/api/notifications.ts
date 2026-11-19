import { request } from './client';
import type { NotificationsResponse } from './types';

export const notificationsApi = {
  list: () => request<NotificationsResponse>('/api/notifications'),
  readAll: () => request<Record<string, never>>('/api/notifications/read-all', { method: 'POST' }),
  remove: (id: string) => request<Record<string, never>>(`/api/notifications/${encodeURIComponent(id)}`, { method: 'DELETE' }),
};
