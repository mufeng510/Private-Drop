import { request } from './client';
import type { LinkPreviewResponse } from './types';

export const linkPreviewApi = {
  fetch: (url: string) => request<LinkPreviewResponse>('/api/link-preview', { method: 'POST', body: { url } }),
};
