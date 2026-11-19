import { create } from 'zustand';

export type Theme = 'light' | 'dark' | 'system';
export type Language = 'zh-CN' | 'en';
export type ToastTone = 'info' | 'success' | 'error';

export interface Toast {
  id: number;
  message: string;
  tone: ToastTone;
}

export interface LightboxImage {
  url: string;
  name?: string;
}

export interface ConfirmOptions {
  title: string;
  message?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}

interface ConfirmState extends ConfirmOptions {
  resolve: (value: boolean) => void;
}

interface UiState {
  theme: Theme;
  language: Language;
  sidebarOpen: boolean;
  uploadPanelOpen: boolean;
  lightbox: { images: LightboxImage[]; index: number } | null;
  confirm: ConfirmState | null;
  toasts: Toast[];
  wsStatus: string;
  /** Bumped on realtime changes so pages can refetch. */
  dataVersion: number;

  setTheme: (theme: Theme) => void;
  setLanguage: (language: Language) => void;
  setSidebarOpen: (open: boolean) => void;
  setUploadPanelOpen: (open: boolean) => void;
  bumpData: () => void;
  setWsStatus: (status: string) => void;

  openLightbox: (images: LightboxImage[], index?: number) => void;
  closeLightbox: () => void;
  lightboxStep: (delta: number) => void;

  requestConfirm: (options: ConfirmOptions) => Promise<boolean>;
  resolveConfirm: (value: boolean) => void;

  toast: (message: string, tone?: ToastTone) => void;
  dismissToast: (id: number) => void;
}

function readStored<T extends string>(key: string, fallback: T): T {
  try {
    return (globalThis.localStorage?.getItem(key) as T) || fallback;
  } catch {
    return fallback;
  }
}

let toastSeq = 0;

export const useUiStore = create<UiState>()((set, get) => ({
  theme: readStored<Theme>('pd.theme', 'system'),
  language: readStored<Language>('pd.lang', 'zh-CN'),
  sidebarOpen: false,
  uploadPanelOpen: false,
  lightbox: null,
  confirm: null,
  toasts: [],
  wsStatus: 'closed',
  dataVersion: 0,

  setTheme: (theme) => {
    try {
      globalThis.localStorage?.setItem('pd.theme', theme);
    } catch {
      /* ignore */
    }
    set({ theme });
  },
  setLanguage: (language) => {
    try {
      globalThis.localStorage?.setItem('pd.lang', language);
    } catch {
      /* ignore */
    }
    set({ language });
  },
  setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
  setUploadPanelOpen: (uploadPanelOpen) => set({ uploadPanelOpen }),
  bumpData: () => set((s) => ({ dataVersion: s.dataVersion + 1 })),
  setWsStatus: (wsStatus) => set({ wsStatus }),

  openLightbox: (images, index = 0) => set({ lightbox: { images, index } }),
  closeLightbox: () => set({ lightbox: null }),
  lightboxStep: (delta) =>
    set((s) => {
      if (!s.lightbox || s.lightbox.images.length === 0) return s;
      const count = s.lightbox.images.length;
      const index = (s.lightbox.index + delta + count) % count;
      return { lightbox: { ...s.lightbox, index } };
    }),

  requestConfirm: (options) =>
    new Promise<boolean>((resolve) => set({ confirm: { ...options, resolve } })),
  resolveConfirm: (value) => {
    const confirm = get().confirm;
    set({ confirm: null });
    confirm?.resolve(value);
  },

  toast: (message, tone = 'info') => {
    const id = ++toastSeq;
    set((s) => ({ toasts: [...s.toasts, { id, message, tone }] }));
    setTimeout(() => get().dismissToast(id), 4500);
  },
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}));
