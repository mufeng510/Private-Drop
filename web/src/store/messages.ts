import { create } from 'zustand';
import { messagesApi } from '../api/messages';
import type { Message, MessageSort } from '../api/types';

interface Filters {
  type?: string;
  favorite?: boolean;
  tag?: string;
  from?: string;
  to?: string;
}

interface MessagesState {
  items: Message[];
  pinned: Message[];
  sort: MessageSort;
  filters: Filters;
  nextCursor: string | null;
  loading: boolean;
  loaded: boolean;

  setSort: (sort: MessageSort) => void;
  setFilters: (filters: Filters) => void;
  load: () => Promise<void>;
  loadMore: () => Promise<void>;
  upsert: (message: Message) => void;
  remove: (id: string) => void;
  patch: (id: string, changes: Partial<Message>) => void;
  reset: () => void;
}

function partition(messages: Message[]): { items: Message[]; pinned: Message[] } {
  return {
    pinned: messages.filter((m) => m.pinned),
    items: messages.filter((m) => !m.pinned),
  };
}

export const useMessagesStore = create<MessagesState>()((set, get) => ({
  items: [],
  pinned: [],
  sort: 'newest',
  filters: {},
  nextCursor: null,
  loading: false,
  loaded: false,

  setSort: (sort) => {
    set({ sort });
    void get().load();
  },
  setFilters: (filters) => {
    set({ filters });
    void get().load();
  },
  load: async () => {
    set({ loading: true });
    try {
      const { sort, filters } = get();
      const res = await messagesApi.list({ sort, limit: 30, ...filters });
      const { items, pinned } = partition(res.items);
      set({ items, pinned, nextCursor: res.nextCursor, loaded: true });
    } finally {
      set({ loading: false });
    }
  },
  loadMore: async () => {
    const { nextCursor, loading, sort, filters } = get();
    if (!nextCursor || loading) return;
    set({ loading: true });
    try {
      const res = await messagesApi.list({ sort, limit: 30, cursor: nextCursor, ...filters });
      set((s) => {
        const { items, pinned } = partition(res.items);
        const seen = new Set([...s.items, ...s.pinned].map((m) => m.id));
        return {
          items: [...s.items, ...items.filter((m) => !seen.has(m.id))],
          pinned: [...s.pinned, ...pinned.filter((m) => !seen.has(m.id))],
          nextCursor: res.nextCursor,
        };
      });
    } finally {
      set({ loading: false });
    }
  },
  upsert: (message) =>
    set((s) => {
      const exists = s.items.some((m) => m.id === message.id) || s.pinned.some((m) => m.id === message.id);
      if (!exists) {
        return message.pinned
          ? { pinned: [message, ...s.pinned] }
          : { items: [message, ...s.items] };
      }
      return {
        items: s.items.map((m) => (m.id === message.id ? message : m)),
        pinned: s.pinned.map((m) => (m.id === message.id ? message : m)),
      };
    }),
  remove: (id) =>
    set((s) => ({
      items: s.items.filter((m) => m.id !== id),
      pinned: s.pinned.filter((m) => m.id !== id),
    })),
  patch: (id, changes) =>
    set((s) => ({
      items: s.items.map((m) => (m.id === id ? { ...m, ...changes } : m)),
      pinned: s.pinned.map((m) => (m.id === id ? { ...m, ...changes } : m)),
    })),
  reset: () => set({ items: [], pinned: [], nextCursor: null, loaded: false, filters: {}, sort: 'newest' }),
}));
