import { create } from 'zustand';
import { authApi } from '../api/auth';
import { session, setMfaInvalidatedHandler } from '../api/session';
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
  setMe: (user: User, device: Device, settings: Settings, mfaValid: boolean) => void;
  setMfaToken: (token: string | null) => void;
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
  // Not derived from mere token presence: a stored token may have expired or
  // been revoked. /api/auth/me reports mfaValid and this is set from that
  // authoritative answer before the app renders.
  adminUnlocked: false,

  setStatus: (status) => set({ status }),
  setLogin: (token, user, device) => {
    session.setToken(token);
    set({ token, user, device, phase: 'authenticated' });
  },
  setMe: (user, device, settings, mfaValid) => {
    // Drop a token the server no longer accepts so the gate reappears.
    if (!mfaValid && session.getMfaToken()) session.setMfaToken(null);
    set({
      user, device, settings,
      mfaToken: session.getMfaToken(),
      adminUnlocked: Boolean(mfaValid),
    });
  },
  setMfaToken: (token) => {
    session.setMfaToken(token);
    set({ mfaToken: token, adminUnlocked: Boolean(token) });
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

// A 403 MFA_REQUIRED from any API call means the stored grant is stale. Drop
// the unlock flag so AdminPage renders the verification gate again.
setMfaInvalidatedHandler(() => {
  useAuthStore.setState({ mfaToken: null, adminUnlocked: false });
});
