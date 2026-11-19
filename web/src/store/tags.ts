import { create } from 'zustand';
import { tagsApi } from '../api/tags';
import type { Tag } from '../api/types';
import { useUiStore } from './ui';

interface TagsState {
  items: Tag[];
  load: () => Promise<void>;
  create: (data: { name: string; color?: string; icon?: string; sort?: number }) => Promise<Tag>;
  update: (id: string, data: Partial<Tag>) => Promise<void>;
  remove: (id: string) => Promise<void>;
}

export const useTagsStore = create<TagsState>()((set, get) => ({
  items: [],
  load: async () => {
    try {
      set({ items: await tagsApi.list() });
    } catch (e) {
      useUiStore.getState().toast(String(e), 'error');
    }
  },
  create: async (data) => {
    const tag = await tagsApi.create(data);
    set((s) => ({ items: [...s.items, tag] }));
    return tag;
  },
  update: async (id, data) => {
    const tag = await tagsApi.update(id, data);
    set((s) => ({ items: s.items.map((t) => (t.id === id ? tag : t)) }));
  },
  remove: async (id) => {
    await tagsApi.remove(id);
    set((s) => ({ items: s.items.filter((t) => t.id !== id) }));
    void get().load();
  },
}));
