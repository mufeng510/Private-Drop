import { request } from './client';
import type { Paginated, RecycleItem } from './types';

/** Backend returns {messages, files}; the UI consumes one flat item list. */
interface RawRecycle {
  messages: Array<{
    id: string;
    text?: string;
    createdAt?: string;
    expiresAt?: string;
    deletedAt?: string;
    attachments?: unknown[];
  }>;
  files: Array<{
    id: string;
    name?: string;
    size?: number;
    uploadedAt?: string;
    deletedAt?: string;
  }>;
}

function normalize(raw: RawRecycle): Paginated<RecycleItem> {
  const items: RecycleItem[] = [
    ...raw.messages.map((m) => ({
      id: m.id,
      kind: 'message' as const,
      text: m.text,
      deletedAt: m.deletedAt ?? '',
      originalCreatedAt: m.createdAt,
      expiresAt: m.expiresAt,
      attachmentCount: m.attachments?.length ?? 0,
    })),
    ...raw.files.map((f) => ({
      id: f.id,
      kind: 'file' as const,
      name: f.name,
      size: f.size,
      deletedAt: f.deletedAt ?? '',
      originalCreatedAt: f.uploadedAt,
    })),
  ];
  items.sort((a, b) => String(b.deletedAt).localeCompare(String(a.deletedAt)));
  return { items, nextCursor: null };
}

export const recycleApi = {
  list: async (_params: { cursor?: string; limit?: number } = {}): Promise<Paginated<RecycleItem>> =>
    normalize(await request<RawRecycle>('/api/recycle')),
  restore: (id: string, mode: 'reset' | 'original') =>
    request<Record<string, never>>(`/api/recycle/${encodeURIComponent(id)}/restore`, {
      method: 'POST',
      body: { mode },
    }),
  remove: (id: string) => request<Record<string, never>>(`/api/recycle/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  empty: () => request<Record<string, never>>('/api/recycle', { method: 'DELETE' }),
};
