import { create } from 'zustand';
import { notificationsApi } from '../api/notifications';
import type { NotificationItem } from '../api/types';

interface NotificationsState {
  items: NotificationItem[];
  unread: number;
  loading: boolean;
  load: () => Promise<void>;
  readAll: () => Promise<void>;
  remove: (id: string) => Promise<void>;
  push: (notification: NotificationItem) => void;
}

export const useNotificationsStore = create<NotificationsState>()((set, _get) => ({
  items: [],
  unread: 0,
  loading: false,

  load: async () => {
    set({ loading: true });
    try {
      const res = await notificationsApi.list();
      set({ items: res.items, unread: res.unread });
    } finally {
      set({ loading: false });
    }
  },
  readAll: async () => {
    await notificationsApi.readAll();
    set((s) => ({ items: s.items.map((n) => ({ ...n, read: true })), unread: 0 }));
  },
  remove: async (id) => {
    await notificationsApi.remove(id);
    set((s) => {
      const items = s.items.filter((n) => n.id !== id);
      return { items, unread: items.filter((n) => !n.read).length };
    });
  },
  push: (notification) =>
    set((s) => {
      if (s.items.some((n) => n.id === notification.id)) return s;
      const items = [notification, ...s.items];
      return { items, unread: items.filter((n) => !n.read).length };
    }),
}));
