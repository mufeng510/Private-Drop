import { authApi } from './api/auth';
import type { Device, User } from './api/types';
import { useAuthStore } from './store/auth';
import { useUploadsStore } from './store/uploads';
import { useNotificationsStore } from './store/notifications';
import { useTagsStore } from './store/tags';
import { connectRealtime, disconnectRealtime } from './realtime';

/** Shared post-login initialization used by init, password and passkey flows. */
export async function finishLogin(token: string, user: User, device: Device, mfaToken?: string): Promise<void> {
  const auth = useAuthStore.getState();
  auth.setLogin(token, user, device);
  // A fresh login either carries a new grant or none at all. Always sync this
  // state so a stale token from a previous session cannot leave the admin UI
  // unlocked without a valid second-factor grant.
  auth.setMfaToken(mfaToken ?? null);
  connectRealtime(token);
  await Promise.allSettled([
    useUploadsStore.getState().init(),
    useNotificationsStore.getState().load(),
    useTagsStore.getState().load(),
    authApi.me().then((me) => useAuthStore.getState().setMe(me.user, me.device, me.settings, me.mfaValid)),
  ]);
}

export async function finishLogout(): Promise<void> {
  disconnectRealtime();
  await useAuthStore.getState().logout();
}

export async function refreshMe(): Promise<void> {
  try {
    const me = await authApi.me();
    useAuthStore.getState().setMe(me.user, me.device, me.settings, me.mfaValid);
  } catch {
    /* handled by callers */
  }
}
