import { request } from './client';
import type { Tag } from './types';

export const tagsApi = {
  list: async () => (await request<{ items: Tag[] }>('/api/tags')).items,
  create: (data: { name: string; color?: string; icon?: string; sort?: number }) =>
    request<Tag>('/api/tags', { method: 'POST', body: data }),
  update: (id: string, data: Partial<{ name: string; color: string; icon: string; sort: number }>) =>
    request<Tag>(`/api/tags/${encodeURIComponent(id)}`, { method: 'PATCH', body: data }),
  remove: (id: string) => request<Record<string, never>>(`/api/tags/${encodeURIComponent(id)}`, { method: 'DELETE' }),
};
