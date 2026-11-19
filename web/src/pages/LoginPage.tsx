import { useEffect, useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { authApi } from '../api/auth';
import { devicesApi } from '../api/devices';
import { ApiError, errorMessage } from '../api/client';
import type { MfaType } from '../api/types';
import { useAuthStore } from '../store/auth';
import { useUiStore } from '../store/ui';
import { finishLogin } from '../authFlow';
import { Button, Field, TextInput } from '../components/ui/primitives';
import { Icon } from '../components/ui/Icon';
import { getAssertion } from '../lib/webauthn';

export function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const phase = useAuthStore((s) => s.phase);
  const token = useAuthStore((s) => s.token);
  const setStatus = useAuthStore((s) => s.setStatus);
  const toast = useUiStore((s) => s.toast);
  const [checking, setChecking] = useState(true);
  const [initialized, setInitialized] = useState(true);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [deviceName, setDeviceName] = useState('');
  const [ttlDays, setTtlDays] = useState(0);
  const [totp, setTotp] = useState('');
  const [mfa, setMfa] = useState<MfaType | null>(null);
  const [code, setCode] = useState('');
  const [pending, setPending] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    authApi.status()
      .then((res) => {
        if (!alive) return;
        setInitialized(res.initialized);
        setStatus(res);
        if (!res.initialized) navigate('/init', { replace: true });
      })
      .catch((e) => alive && toast(errorMessage(e), 'error'))
      .finally(() => alive && setChecking(false));
    return () => { alive = false; };
  }, [navigate, setStatus, toast]);

  if (checking) return <div className="flex min-h-dvh items-center justify-center text-slate-400">{t('common.loading')}</div>;
  if (phase === 'authenticated' && token) return <Navigate to="/" replace />;
  if (!initialized) return <Navigate to="/init" replace />;

  const complete = async (res: { token?: string; user: import('../api/types').User; device: import('../api/types').Device; mfaToken?: string }) => {
    if (!res.token) return;
    await finishLogin(res.token, res.user, res.device, res.mfaToken);
    navigate('/', { replace: true });
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setPending(false);
    try {
      const res = await authApi.login({
        username: username.trim(),
        password,
        deviceName: deviceName.trim() || undefined,
        totp: totp.trim() || undefined,
        ttlDays,
      });
      if (res.mfaRequired === 'totp') {
        setMfa('totp');
        toast(t('auth.mfaRequired'), 'info');
      } else if (res.mfaRequired === 'passkey') {
        setMfa('passkey');
      } else {
        await complete(res);
      }
    } catch (err) {
      if (err instanceof ApiError && err.code === 'DEVICE_PENDING') setPending(true);
      else if (err instanceof ApiError && err.code === 'MFA_REQUIRED') setMfa('totp');
      else toast(err instanceof ApiError && err.code === 'INVALID_CREDENTIALS' ? t('auth.invalidCredentials') : errorMessage(err), 'error');
    } finally {
      setBusy(false);
    }
  };

  const authorizeDevice = async () => {
    try {
      await devicesApi.approveByCode(code.trim());
      setPending(false);
      toast(t('common.saved'), 'success');
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const passkeyLogin = async () => {
    setBusy(true);
    try {
      const options = await authApi.passkeyLoginBegin(username.trim() || undefined);
      const pub = options.publicKey as PublicKeyCredentialRequestOptions;
      if (!pub) throw new Error('No WebAuthn options');
      const assertion = await getAssertion(pub);
      await complete(await authApi.passkeyLoginFinish(assertion));
    } catch (e) {
      toast(errorMessage(e), 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="flex min-h-dvh items-center justify-center bg-slate-50 px-4 py-10 dark:bg-slate-950">
      <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">
        <div className="mb-6 flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-xl bg-blue-600 text-white"><Icon name="lock" size={22} /></span>
          <div>
            <h1 className="text-lg font-semibold">{t('auth.loginTitle')}</h1>
            <p className="text-xs text-slate-400">{t('auth.welcome')}</p>
          </div>
        </div>

        {pending ? (
          <div className="flex flex-col gap-3 rounded-lg bg-amber-50 p-4 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
            <p>{t('auth.approvalPending')}</p>
            <TextInput value={code} onChange={(e) => setCode(e.target.value)} placeholder={t('auth.approvalCode')} />
            <Button variant="primary" onClick={() => void authorizeDevice()} disabled={!code.trim()}>{t('admin.authorize')}</Button>
            <Button variant="ghost" onClick={() => setPending(false)}>{t('common.back')}</Button>
          </div>
        ) : (
          <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-4">
            <Field label={t('auth.username')}>
              <TextInput value={username} onChange={(e) => setUsername(e.target.value)} required autoComplete="username" />
            </Field>
            <Field label={t('auth.password')}>
              <TextInput type="password" value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
            </Field>
            <Field label={t('auth.deviceName')}>
              <TextInput value={deviceName} onChange={(e) => setDeviceName(e.target.value)} placeholder={t('auth.deviceNamePlaceholder')} />
            </Field>
            <Field label={t('auth.loginDuration')} hint={t('auth.ttlHint')}>
              <select value={ttlDays} onChange={(e) => setTtlDays(Number(e.target.value))} className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100" aria-label={t('auth.loginDuration')}>
                <option value={0}>{t('auth.ttlDefault')}</option>
                <option value={7}>{t('auth.ttl7')}</option>
                <option value={30}>{t('auth.ttl30')}</option>
                <option value={90}>{t('auth.ttl90')}</option>
                <option value={-1}>{t('auth.ttlForever')}</option>
              </select>
            </Field>
            {mfa === 'totp' && (
              <Field label={t('auth.totpCode')}>
                <TextInput value={totp} onChange={(e) => setTotp(e.target.value)} inputMode="numeric" autoComplete="one-time-code" placeholder="000000" />
              </Field>
            )}
            <Button type="submit" variant="primary" loading={busy} className="w-full">{t('auth.signIn')}</Button>
            <button type="button" onClick={() => void passkeyLogin()} disabled={busy} className="inline-flex w-full items-center justify-center gap-2 rounded-lg border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 disabled:opacity-60 dark:border-slate-600 dark:hover:bg-slate-800">
              <Icon name="keyboard" size={16} /> {t('auth.usePasskey')}
            </button>
            <p className="text-center text-xs text-slate-400">{t('auth.newDeviceNotice')}</p>
          </form>
        )}
      </div>
    </main>
  );
}
