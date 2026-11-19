import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { authApi } from '../api/auth';
import { errorMessage } from '../api/client';
import { useAuthStore } from '../store/auth';
import { useUiStore } from '../store/ui';
import { getAssertion } from '../lib/webauthn';
import { Button, Field, TextInput } from '../components/ui/primitives';
import { Icon } from '../components/ui/Icon';

export function AdminGate() {
  const { t } = useTranslation();
  const setMfaToken = useAuthStore((s) => s.setMfaToken);
  const toast = useUiStore((s) => s.toast);
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);

  const verifyTotp = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await authApi.mfaVerify({ type: 'totp', code: code.trim() });
      setMfaToken(res.mfaToken);
      toast(t('admin.unlocked'), 'success');
    } catch (err) {
      toast(errorMessage(err), 'error');
    } finally {
      setBusy(false);
    }
  };

  const verifyPasskey = async () => {
    setBusy(true);
    try {
      const options = await authApi.passkeyLoginBegin();
      const pub = options.publicKey as PublicKeyCredentialRequestOptions;
      if (!pub) throw new Error('No WebAuthn options');
      const assertion = await getAssertion(pub);
      const res = await authApi.mfaVerify({ type: 'passkey', assertion });
      setMfaToken(res.mfaToken);
      toast(t('admin.unlocked'), 'success');
    } catch (e) {
      toast(errorMessage(e), 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="flex min-h-dvh items-center justify-center bg-slate-50 px-4 py-10 dark:bg-slate-950">
      <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">
        <div className="mb-4 flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-xl bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300"><Icon name="shield" size={22} /></span>
          <h1 className="text-lg font-semibold">{t('auth.adminGateTitle')}</h1>
        </div>
        <p className="mb-6 text-sm text-slate-500 dark:text-slate-400">{t('auth.adminGateDesc')}</p>
        <form onSubmit={(e) => void verifyTotp(e)} className="flex flex-col gap-4">
          <Field label={t('auth.totpCode')}>
            <TextInput value={code} onChange={(e) => setCode(e.target.value)} inputMode="numeric" autoComplete="one-time-code" placeholder="000000" autoFocus />
          </Field>
          <Button type="submit" variant="primary" loading={busy} disabled={!code.trim()}>{t('auth.adminGateButton')}</Button>
          <Button type="button" variant="secondary" onClick={() => void verifyPasskey()} disabled={busy} icon="keyboard">{t('auth.mfaPasskey')}</Button>
        </form>
      </div>
    </main>
  );
}
