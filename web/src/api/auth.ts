import { request } from './client';
import type {
  AuthStatus,
  InitRequest,
  InitResponse,
  LoginRequest,
  LoginResponse,
  MeResponse,
  MfaVerifyRequest,
  MfaVerifyResponse,
  TotpSetup,
} from './types';

export const authApi = {
  status: () => request<AuthStatus>('/api/auth/status', { auth: false }),
  init: (data: InitRequest) => request<InitResponse>('/api/auth/init', { method: 'POST', body: data, auth: false }),
  login: (data: LoginRequest) => request<LoginResponse>('/api/auth/login', { method: 'POST', body: data, auth: false }),
  logout: () => request<Record<string, never>>('/api/auth/logout', { method: 'POST' }),
  // Include the MFA header so the server can report its validity in mfaValid.
  me: () => request<MeResponse>('/api/auth/me', { mfa: true }),
  refresh: () => request<{ token: string }>('/api/auth/refresh', { method: 'POST' }),

  totpSetup: () => request<TotpSetup>('/api/auth/totp/setup', { method: 'POST', mfa: true }),
  totpEnable: (code: string) => request<Record<string, never>>('/api/auth/totp/enable', { method: 'POST', body: { code }, mfa: true }),

  passkeyRegisterBegin: () => request<Record<string, unknown>>('/api/auth/passkey/register/begin', { method: 'POST', mfa: true }),
  passkeyRegisterFinish: (credential: unknown) =>
    request<Record<string, never>>('/api/auth/passkey/register/finish', { method: 'POST', body: credential, mfa: true }),
  passkeyLoginBegin: (username?: string) =>
    request<Record<string, unknown>>('/api/auth/passkey/login/begin', { method: 'POST', body: { username }, auth: false }),
  passkeyLoginFinish: (assertion: unknown) =>
    request<LoginResponse>('/api/auth/passkey/login/finish', { method: 'POST', body: assertion, auth: false }),

  mfaVerify: (data: MfaVerifyRequest) => request<MfaVerifyResponse>('/api/auth/mfa/verify', { method: 'POST', body: data }),
};
