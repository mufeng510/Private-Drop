import { fetchBinary, request } from './client';
import type { Attachment, Paginated } from './types';

export interface ListFilesParams {
  cursor?: string;
  limit?: number;
  type?: string;
  tag?: string;
  favorite?: boolean;
  from?: string;
  to?: string;
  sort?: string;
}

/** Contract v2: POST /api/files/bulk. */
export type FilesBulkAction = 'favorite' | 'unfavorite' | 'delete' | 'restore' | 'tag' | 'untag';

export interface FilesBulkPayload {
  ids: string[];
  action: FilesBulkAction;
  tagId?: string;
}

export const filesApi = {
  list: (params: ListFilesParams = {}) => request<Paginated<Attachment>>('/api/files', { query: { ...params } }),
  get: (id: string) => request<Attachment>(`/api/files/${encodeURIComponent(id)}`),
  update: (id: string, data: { favorited?: boolean; name?: string; tagIds?: string[] }) =>
    request<Attachment>(`/api/files/${encodeURIComponent(id)}`, { method: 'PATCH', body: data }),
  remove: (id: string) => request<Record<string, never>>(`/api/files/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  bulk: (payload: FilesBulkPayload) =>
    request<Record<string, never>>('/api/files/bulk', { method: 'POST', body: payload }),
  download: (id: string, signal?: AbortSignal) => fetchBinary(`/api/files/${encodeURIComponent(id)}/download`, { signal }),
  zip: (ids: string[], keepStructure: boolean, signal?: AbortSignal) =>
    fetchBinary('/api/files/zip', { method: 'POST', body: { ids, keepStructure }, signal }),
  thumbnail: (id: string, signal?: AbortSignal) => fetchBinary(`/api/files/${encodeURIComponent(id)}/thumbnail`, { signal }),
  preview: (id: string, signal?: AbortSignal) => fetchBinary(`/api/files/${encodeURIComponent(id)}/preview`, { signal }),
};
