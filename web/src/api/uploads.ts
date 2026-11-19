import { request } from './client';
import type {
  InitUploadRequest,
  InitUploadResponse,
  UploadedChunkStatus,
  UploadListResponse,
} from './types';

export const uploadsApi = {
  init: (data: InitUploadRequest) => request<InitUploadResponse>('/api/uploads/init', { method: 'POST', body: data }),
  putChunk: (id: string, index: number, chunk: Blob, signal?: AbortSignal) =>
    request<{ received: boolean }>(`/api/uploads/${encodeURIComponent(id)}/chunk/${index}`, {
      method: 'PUT',
      rawBody: chunk,
      signal,
    }),
  complete: (id: string) => request<{ attachmentId: string }>(`/api/uploads/${encodeURIComponent(id)}/complete`, { method: 'POST', body: {} }),
  status: (id: string) => request<UploadedChunkStatus>(`/api/uploads/${encodeURIComponent(id)}/status`),
  list: () => request<UploadListResponse>('/api/uploads'),
  cancel: (id: string) => request<Record<string, never>>(`/api/uploads/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  retry: (id: string) => request<Record<string, never>>(`/api/uploads/${encodeURIComponent(id)}/retry`, { method: 'POST', body: {} }),
};
