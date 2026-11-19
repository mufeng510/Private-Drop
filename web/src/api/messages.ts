import { request } from './client';
import type { Message, MessageSort, Paginated } from './types';

export interface ListMessagesParams {
  cursor?: string;
  limit?: number;
  sort?: MessageSort;
  type?: string;
  favorite?: boolean;
  tag?: string;
  from?: string;
  to?: string;
}

export type BulkAction = 'delete' | 'favorite' | 'unfavorite' | 'pin' | 'unpin' | 'tag' | 'untag' | 'restore';

export interface BulkPayload {
  ids: string[];
  action: BulkAction;
  tagId?: string;
}

export const messagesApi = {
  list: (params: ListMessagesParams = {}) => request<Paginated<Message>>('/api/messages', { query: { ...params } }),
  create: (data: { text?: string; attachmentIds?: string[] }) => request<Message>('/api/messages', { method: 'POST', body: data }),
  get: (id: string) => request<Message>(`/api/messages/${encodeURIComponent(id)}`),
  update: (id: string, data: { text?: string; pinned?: boolean; favorited?: boolean; tagIds?: string[] }) =>
    request<Message>(`/api/messages/${encodeURIComponent(id)}`, { method: 'PATCH', body: data }),
  remove: (id: string) => request<Record<string, never>>(`/api/messages/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  bulk: (payload: BulkPayload) => request<Record<string, never>>('/api/messages/bulk', { method: 'POST', body: payload }),
  expiring: (params: { cursor?: string; limit?: number } = {}) =>
    request<Paginated<Message>>('/api/messages/expiring', { query: { ...params } }),
};
