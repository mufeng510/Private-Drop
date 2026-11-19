import { request } from './client';
import type { Device, DeviceRequest } from './types';

// Device administration is an admin action and requires secondary verification.
// approveByCode is intentionally exempt: it is the new-device bootstrap flow
// used from the login screen before any admin grant exists.
interface ItemList<T> { items: T[] }

export const devicesApi = {
  list: async () => (await request<ItemList<Device>>('/api/devices', { mfa: true })).items,
  rename: (id: string, name: string) => request<Device>(`/api/devices/${encodeURIComponent(id)}`, { method: 'PATCH', body: { name }, mfa: true }),
  revoke: (id: string) => request<Record<string, never>>(`/api/devices/${encodeURIComponent(id)}/revoke`, { method: 'POST', mfa: true }),
  requests: async () => (await request<ItemList<DeviceRequest>>('/api/devices/requests', { mfa: true })).items,
  approve: (id: string) => request<Record<string, never>>(`/api/devices/requests/${encodeURIComponent(id)}/approve`, { method: 'POST', body: {}, mfa: true }),
  reject: (id: string) => request<Record<string, never>>(`/api/devices/requests/${encodeURIComponent(id)}/reject`, { method: 'POST', mfa: true }),
  approveByCode: (code: string) => request<Record<string, never>>('/api/devices/requests/code', { method: 'POST', body: { code } }),
};
