import { useEffect, useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { authApi } from '../api/auth';
import { errorMessage, ApiError } from '../api/client';
import type { AuthStatus, DevicePolicy } from '../api/types';
import { useAuthStore } from '../store/auth';
import { useUiStore } from '../store/ui';
import { finishLogin } from '../authFlow';
import { Button, Field, TextInput, inputClass } from '../components/ui/primitives';
import { Icon } from '../components/ui/Icon';

export function InitPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const phase = useAuthStore((s) => s.phase);
  const setStatus = useAuthStore((s) => s.setStatus);
  const toast = useUiStore((s) => s.toast);
  const [checking, setChecking] = useState(true);
  const [status, setLocalStatus] = useState<AuthStatus | null>(null);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [deviceName, setDeviceName] = useState('');
  const [policy, setPolicy] = useState<DevicePolicy>('approval');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    authApi.status()
      .then((res) => {
        if (!alive) return;
        setLocalStatus(res);
        setStatus(res);
      })
      .catch((e) => alive && toast(errorMessage(e), 'error'))
      .finally(() => alive && setChecking(false));
    return () => { alive = false; };
  }, [setStatus, toast]);

  if (checking) return <div className="flex min-h-dvh items-center justify-center text-slate-400">{t('common.loading')}</div>;
  if (status?.initialized) return <Navigate to="/login" replace />;
  if (phase === 'authenticated') return <Navigate to="/" replace />;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await authApi.init({ username: username.trim(), password, deviceName: deviceName.trim() || undefined, policy });
      await finishLogin(res.token, res.user, res.device, res.mfaToken);
      navigate('/', { replace: true });
    } catch (err) {
      const msg = err instanceof ApiError && err.code === 'FORBIDDEN'
        ? t('auth.lanOnlyNote')
        : errorMessage(err);
      toast(msg, 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="flex min-h-dvh items-center justify-center bg-slate-50 px-4 py-10 dark:bg-slate-950">
      <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">
        <div className="mb-6 flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-xl bg-blue-600 text-white"><Icon name="sparkles" size={22} /></span>
          <div>
            <h1 className="text-lg font-semibold">{t('auth.initTitle')}</h1>
            <p className="text-xs text-slate-400">Private Drop</p>
          </div>
        </div>
        <p className="mb-5 text-sm text-slate-500 dark:text-slate-400">{t('auth.initSubtitle')}</p>
        <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-4">
          <Field label={t('auth.username')}>
            <TextInput value={username} onChange={(e) => setUsername(e.target.value)} required autoComplete="username" />
          </Field>
          <Field label={t('auth.password')}>
            <TextInput type="password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} autoComplete="new-password" />
          </Field>
          <Field label={t('auth.deviceName')}>
            <TextInput value={deviceName} onChange={(e) => setDeviceName(e.target.value)} placeholder={t('auth.deviceNamePlaceholder')} />
          </Field>
          <Field label={t('auth.initPolicy')}>
            <select value={policy} onChange={(e) => setPolicy(e.target.value as DevicePolicy)} className={inputClass}>
              <option value="open">{t('admin.policyOpen')}</option>
              <option value="whitelist">{t('admin.policyWhitelist')}</option>
              <option value="approval">{t('admin.policyApproval')}</option>
            </select>
          </Field>
          <p className="rounded-lg bg-amber-50 p-3 text-xs text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">{t('auth.lanOnlyNote')}</p>
          <Button type="submit" variant="primary" loading={busy} className="w-full">{t('auth.initButton')}</Button>
        </form>
      </div>
    </main>
  );
}
