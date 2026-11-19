import { create } from 'zustand';
import { authApi } from '../api/auth';
import { session } from '../api/session';
import type { AuthStatus, Device, Settings, User } from '../api/types';

type AuthPhase = 'loading' | 'guest' | 'authenticated';

interface AuthState {
  phase: AuthPhase;
  status: AuthStatus | null;
  user: User | null;
  device: Device | null;
  settings: Settings;
  token: string | null;
  mfaToken: string | null;
  /** Admin UI has passed the second-factor gate for this session. */
  adminUnlocked: boolean;
  setStatus: (status: AuthStatus) => void;
  setLogin: (token: string, user: User, device: Device) => void;
  setMe: (user: User, device: Device, settings: Settings) => void;
  setMfaToken: (token: string) => void;
  setAdminUnlocked: (value: boolean) => void;
  setSettings: (settings: Settings) => void;
  setGuest: () => void;
  logout: () => Promise<void>;
}

export const useAuthStore = create<AuthState>()((set, get) => ({
  phase: 'loading',
  status: null,
  user: null,
  device: null,
  settings: {},
  token: session.getToken(),
  mfaToken: session.getMfaToken(),
  // A stored (still valid) bootstrap/verification grant keeps the admin UI
  // unlocked across reloads; the backend re-checks its expiry on every call.
  adminUnlocked: Boolean(session.getMfaToken()),

  setStatus: (status) => set({ status }),
  setLogin: (token, user, device) => {
    session.setToken(token);
    set({ token, user, device, phase: 'authenticated' });
  },
  setMe: (user, device, settings) => set({ user, device, settings }),
  setMfaToken: (token) => {
    session.setMfaToken(token);
    set({ mfaToken: token, adminUnlocked: true });
  },
  setAdminUnlocked: (value) => set({ adminUnlocked: value }),
  setSettings: (settings) => set({ settings }),
  setGuest: () => {
    session.clear();
    set({
      phase: 'guest',
      token: null,
      mfaToken: null,
      user: null,
      device: null,
      settings: {},
      adminUnlocked: false,
    });
  },
  logout: async () => {
    try {
      await authApi.logout();
    } catch {
      /* clear locally regardless */
    }
    get().setGuest();
  },
}));
