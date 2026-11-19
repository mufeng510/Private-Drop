import { request } from './client';
import type { Paginated, SearchResult } from './types';

export interface SearchParams {
  q: string;
  type?: string;
  ext?: string;
  minSize?: number;
  maxSize?: number;
  from?: string;
  to?: string;
  tag?: string;
  favorite?: boolean;
  pinned?: boolean;
  device?: string;
  cursor?: string;
  limit?: number;
}

export const searchApi = {
  search: (params: SearchParams) => request<Paginated<SearchResult>>('/api/search', { query: { ...params } }),
};
