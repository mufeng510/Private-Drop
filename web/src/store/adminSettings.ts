import { create } from 'zustand';
import { settingsApi } from '../api/settings';
import type { Settings } from '../api/types';
import { useUiStore } from './ui';
import { useAuthStore } from './auth';

interface AdminSettingsState {
  values: Settings;
  loading: boolean;
  load: () => Promise<void>;
  save: (values: Settings) => Promise<void>;
}

export const useAdminSettings = create<AdminSettingsState>()((set, _get) => ({
  values: {},
  loading: false,
  load: async () => {
    set({ loading: true });
    try {
      const values = await settingsApi.list();
      set({ values });
    } catch (e) {
      useUiStore.getState().toast(String(e), 'error');
    } finally {
      set({ loading: false });
    }
  },
  save: async (values) => {
    try {
      const updated = await settingsApi.update(values);
      set((s) => ({ values: { ...s.values, ...updated } }));
      useAuthStore.getState().setSettings({ ...useAuthStore.getState().settings, ...updated });
      useUiStore.getState().toast('common.saved', 'success');
      useUiStore.getState().bumpData();
    } catch (e) {
      useUiStore.getState().toast(String(e), 'error');
      throw e;
    }
  },
}));
