import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { authApi } from '../../api/auth';
import { devicesApi } from '../../api/devices';
import { settingsApi } from '../../api/settings';
import { errorMessage } from '../../api/client';
import type { Device, DeviceRequest, DevicePolicy, Settings, TotpSetup } from '../../api/types';
import { useAdminSettings } from '../../store/adminSettings';
import { useAuthStore } from '../../store/auth';
import { useUploadsStore } from '../../store/uploads';
import { useUiStore, type Language, type Theme } from '../../store/ui';
import { setLanguage as setI18nLanguage } from '../../i18n';
import { fromBase64Url, getAssertion, toBase64Url } from '../../lib/webauthn';
import { Button, Field, IconButton, TextInput, Toggle, inputClass } from '../../components/ui/primitives';
import { Icon } from '../../components/ui/Icon';

function Card({ title, description, children }: { title: string; description?: string; children: ReactNode }) {
  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
      <h2 className="text-base font-semibold">{title}</h2>
      {description && <p className="mt-1 text-xs text-slate-400">{description}</p>}
      <div className="mt-4 flex flex-col gap-4">{children}</div>
    </section>
  );
}

function num(values: Settings, key: string, fallback: number): number {
  const value = values[key];
  const parsed = Number(value ?? fallback);
  return Number.isFinite(parsed) ? parsed : fallback;
}
function bool(values: Settings, key: string, fallback = false): boolean {
  const value = values[key];
  if (value === undefined || value === null) return fallback;
  return value === true || value === 1 || value === '1' || value === 'true';
}

function useSettingsForm() {
  const values = useAdminSettings((s) => s.values);
  const save = useAdminSettings((s) => s.save);
  return { values, save };
}

export function AccountSecurity() {
  const { t } = useTranslation();
  const toast = useUiStore((s) => s.toast);
  const user = useAuthStore((s) => s.user);
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [totpSetup, setTotpSetup] = useState<TotpSetup | null>(null);
  const [totpCode, setTotpCode] = useState('');

  const changePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await settingsApi.password(oldPassword, newPassword);
      toast(t('common.saved'), 'success');
      setOldPassword('');
      setNewPassword('');
    } catch (err) {
      toast(errorMessage(err), 'error');
    }
  };

  const setupTotp = async () => {
    try {
      setTotpSetup(await authApi.totpSetup());
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const enableTotp = async () => {
    try {
      const code = totpCode.trim();
      await authApi.totpEnable(code);
      // Enrolling a factor retires the bootstrap grant; re-verify so the admin
      // session stays unlocked with a real second-factor token.
      const verified = await authApi.mfaVerify({ type: 'totp', code });
      useAuthStore.getState().setMfaToken(verified.mfaToken);
      setTotpSetup(null);
      toast(t('common.saved'), 'success');
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const registerPasskey = async () => {
    try {
      const options = await authApi.passkeyRegisterBegin();
      const pub = options.publicKey as PublicKeyCredentialCreationOptions;
      if (!pub) throw new Error('No WebAuthn options');
      const credential = (await navigator.credentials.create({
        publicKey: {
          ...pub,
          challenge: fromBase64Url(pub.challenge as unknown as string),
          user: { ...pub.user, id: fromBase64Url(pub.user.id as unknown as string) },
          excludeCredentials: pub.excludeCredentials?.map((c) => ({ ...c, id: fromBase64Url(c.id as unknown as string) })),
        },
      })) as PublicKeyCredential | null;
      if (!credential) throw new Error('No credential returned');
      const response = credential.response as AuthenticatorAttestationResponse;
      await authApi.passkeyRegisterFinish({
        id: credential.id,
        rawId: toBase64Url(credential.rawId),
        type: credential.type,
        response: {
          clientDataJSON: toBase64Url(response.clientDataJSON),
          attestationObject: toBase64Url(response.attestationObject),
        },
      });
      // Registering a factor retires the bootstrap grant server-side. Obtain a
      // fresh real grant so the admin session stays unlocked (mirrors TOTP).
      const loginOptions = await authApi.passkeyLoginBegin();
      const requestPub = loginOptions.publicKey as PublicKeyCredentialRequestOptions;
      if (!requestPub) throw new Error('No WebAuthn options');
      const assertion = await getAssertion(requestPub);
      const verified = await authApi.mfaVerify({ type: 'passkey', assertion });
      useAuthStore.getState().setMfaToken(verified.mfaToken);
      toast(t('common.saved'), 'success');
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <Card title={t('admin.account')} description={user?.username}>
        <form onSubmit={(e) => void changePassword(e)} className="flex flex-col gap-3">
          <Field label={t('admin.oldPassword')}><TextInput type="password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} autoComplete="current-password" required /></Field>
          <Field label={t('admin.newPassword')}><TextInput type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} autoComplete="new-password" minLength={8} required /></Field>
          <Button type="submit" variant="primary" className="self-start">{t('admin.changePassword')}</Button>
        </form>
      </Card>
      <Card title={t('admin.totp')}>
        <p className="text-sm text-slate-500 dark:text-slate-400">{user?.hasTotp || user?.totpEnabled ? t('common.on') : t('common.off')}</p>
        {!totpSetup ? (
          <Button variant="secondary" className="self-start" onClick={() => void setupTotp()}>{t('auth.totpSetup')}</Button>
        ) : (
          <div className="flex flex-col gap-3 rounded-xl bg-slate-50 p-3 dark:bg-slate-800">
            <p className="text-xs text-slate-500">{t('auth.totpSecret')}: <code className="break-all font-mono text-sm text-slate-800 dark:text-slate-100">{totpSetup.secret}</code></p>
            {totpSetup.qr && <img src={totpSetup.qr} alt="TOTP QR" className="size-40" />}
            <TextInput value={totpCode} onChange={(e) => setTotpCode(e.target.value)} placeholder="000000" inputMode="numeric" />
            <Button variant="primary" className="self-start" onClick={() => void enableTotp()}>{t('auth.totpEnable')}</Button>
          </div>
        )}
      </Card>
      <Card title={t('admin.passkey')} description={user?.hasPasskey || user?.passkeyEnabled ? t('common.on') : t('common.off')}>
        <Button variant="secondary" className="self-start" icon="keyboard" onClick={() => void registerPasskey()}>{t('admin.registerPasskey')}</Button>
      </Card>
    </div>
  );
}

export function DevicesAdmin() {
  const { t, i18n } = useTranslation();
  const toast = useUiStore((s) => s.toast);
  const [devices, setDevices] = useState<Device[]>([]);
  const [requests, setRequests] = useState<DeviceRequest[]>([]);
  const [code, setCode] = useState('');
  const [editing, setEditing] = useState<string | null>(null);
  const [name, setName] = useState('');

  const load = async () => {
    try {
      const [d, r] = await Promise.all([devicesApi.list(), devicesApi.requests()]);
      setDevices(d);
      setRequests(r);
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };
  // eslint-disable-next-line react-hooks/exhaustive-deps -- load is recreated each render; run on mount/filter change only
  useEffect(() => { void load(); }, []);

  const act = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      toast(t('common.saved'), 'success');
      void load();
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <Card title={t('admin.devices')}>
        <ul className="flex flex-col gap-2">
          {devices.map((device) => (
            <li key={device.id} className="flex flex-wrap items-center gap-2 rounded-xl border border-slate-200 p-3 dark:border-slate-700">
              <span className="flex size-9 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-slate-800"><Icon name="devices" size={18} /></span>
              <div className="min-w-0 flex-1">
                {editing === device.id ? (
                  <TextInput value={name} onChange={(e) => setName(e.target.value)} autoFocus />
                ) : (
                  <p className="truncate text-sm font-medium">{device.name} {device.current && <span className="ml-1 text-xs text-blue-600">({t('admin.current')})</span>}</p>
                )}
                <p className="text-xs text-slate-400">{device.type} · {device.lastActive ? `${t('admin.lastActive')} ${new Date(device.lastActive).toLocaleString(i18n.language)}` : '—'} · {device.ip}</p>
              </div>
              <span className={`rounded-full px-2 py-0.5 text-[10px] ${device.authorized ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-slate-100 text-slate-500'}`}>
                {device.authorized ? t('admin.authorized') : t('admin.revoked')}
              </span>
              {editing === device.id ? (
                <Button variant="primary" onClick={() => void act(async () => { await devicesApi.rename(device.id, name); setEditing(null); })}>{t('common.save')}</Button>
              ) : (
                <IconButton icon="edit" label={t('admin.renameDevice')} onClick={() => { setEditing(device.id); setName(device.name); }} />
              )}
              {!device.current && <IconButton icon="logout" label={t('admin.revoke')} className="hover:text-red-600" onClick={() => void act(() => devicesApi.revoke(device.id))} />}
            </li>
          ))}
        </ul>
      </Card>
      <Card title={t('admin.pendingRequests')}>
        {requests.length === 0 ? <p className="text-sm text-slate-400">{t('admin.noRequests')}</p> : (
          <ul className="flex flex-col gap-2">
            {requests.map((request) => (
              <li key={request.id} className="flex items-center gap-2 rounded-xl border border-slate-200 p-3 dark:border-slate-700">
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium">{request.name}</p>
                  <p className="text-xs text-slate-400">{request.type} · {request.ip} · {request.code}</p>
                </div>
                <Button variant="primary" onClick={() => void act(() => devicesApi.approve(request.id))}>{t('admin.approve')}</Button>
                <Button variant="ghost" onClick={() => void act(() => devicesApi.reject(request.id))}>{t('admin.reject')}</Button>
              </li>
            ))}
          </ul>
        )}
        <div className="flex gap-2">
          <TextInput value={code} onChange={(e) => setCode(e.target.value)} placeholder={t('admin.oneTimeCode')} />
          <Button variant="secondary" disabled={!code.trim()} onClick={() => void act(() => devicesApi.approveByCode(code.trim()))}>{t('admin.authorize')}</Button>
        </div>
      </Card>
    </div>
  );
}

export function LoginPolicySettings() {
  const { values, save } = useSettingsForm();
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Settings>({});
  useEffect(() => setDraft({}), [values]);
  const field = (key: string, value: string | number) => setDraft((d) => ({ ...d, [key]: value }));
  return (
    <Card title={t('admin.loginPolicy')} description={t('admin.policyNote')}>
      <Field label={t('admin.devicePolicy')}>
        <select className={inputClass} value={String(draft.devicePolicy ?? values.devicePolicy ?? 'approval')} onChange={(e) => field('devicePolicy', e.target.value as DevicePolicy)}>
          <option value="open">{t('admin.policyOpen')}</option>
          <option value="whitelist">{t('admin.policyWhitelist')}</option>
          <option value="approval">{t('admin.policyApproval')}</option>
        </select>
      </Field>
      <Field label={t('admin.sessionDuration')}>
        <TextInput type="number" min={0} value={String(draft.sessionDuration ?? num(values, 'sessionDuration', 30))} onChange={(e) => field('sessionDuration', Number(e.target.value))} />
      </Field>
      <Field label={t('admin.loginAttemptLimit')}>
        <TextInput type="number" min={1} value={String(draft.loginAttemptLimit ?? num(values, 'loginAttemptLimit', 5))} onChange={(e) => field('loginAttemptLimit', Number(e.target.value))} />
      </Field>
      <Button variant="primary" className="self-start" onClick={() => void save(draft)} disabled={Object.keys(draft).length === 0}>{t('common.save')}</Button>
    </Card>
  );
}

export function RetentionSettings() {
  const { values, save } = useSettingsForm();
  const { t } = useTranslation();
  const [retentionDays, setRetentionDays] = useState(num(values, 'retentionDays', 30));
  const [recycleDays, setRecycleDays] = useState(num(values, 'recycleDays', 30));
  useEffect(() => {
    setRetentionDays(num(values, 'retentionDays', 30));
    setRecycleDays(num(values, 'recycleDays', 30));
  }, [values]);
  return (
    <Card title={t('admin.retention')}>
      <Field label={t('admin.retentionDays')}>
        <TextInput type="number" min={1} value={retentionDays} onChange={(e) => setRetentionDays(Number(e.target.value))} />
      </Field>
      <Field label={t('admin.recycleDays')} hint={t('recycle.title')}>
        <TextInput type="number" min={1} value={recycleDays} onChange={(e) => setRecycleDays(Number(e.target.value))} />
      </Field>
      <Button variant="primary" className="self-start" onClick={() => void save({ retentionDays, recycleDays })}>{t('common.save')}</Button>
    </Card>
  );
}

export function UploadSettings() {
  const { values, save } = useSettingsForm();
  const { t } = useTranslation();
  const manager = useUploadsStore((s) => s.manager);
  const [maxFileSizeMb, setMaxFileSizeMb] = useState(num(values, 'maxFileSizeMb', 512));
  const [concurrency, setConcurrency] = useState(num(values, 'uploadConcurrency', 3));
  useEffect(() => {
    setMaxFileSizeMb(num(values, 'maxFileSizeMb', 512));
    setConcurrency(num(values, 'uploadConcurrency', 3));
  }, [values]);
  return (
    <Card title={t('admin.uploadLimits')}>
      <Field label={t('admin.maxFileSize')}>
        <TextInput type="number" min={1} value={maxFileSizeMb} onChange={(e) => setMaxFileSizeMb(Number(e.target.value))} />
      </Field>
      <Field label={t('admin.uploadConcurrency')}>
        <TextInput type="number" min={1} max={10} value={concurrency} onChange={(e) => setConcurrency(Number(e.target.value))} />
      </Field>
      <Button variant="primary" className="self-start" onClick={() => void save({ maxFileSizeMb, uploadConcurrency: concurrency }).then(() => manager?.setConcurrency(concurrency))}>{t('common.save')}</Button>
    </Card>
  );
}

export function StorageWarningSettings() {
  const { values, save } = useSettingsForm();
  const { t } = useTranslation();
  const [percent, setPercent] = useState(num(values, 'storageWarnPercent', 85));
  useEffect(() => setPercent(num(values, 'storageWarnPercent', 85)), [values]);
  return (
    <Card title={t('admin.storageWarning')} description={t('admin.warnNote')}>
      <Field label={t('admin.storageWarnPercent')}>
        <TextInput type="number" min={1} max={99} value={percent} onChange={(e) => setPercent(Number(e.target.value))} />
      </Field>
      <Button variant="primary" className="self-start" onClick={() => void save({ storageWarnPercent: percent })}>{t('common.save')}</Button>
    </Card>
  );
}

export function LinkParsingSettings() {
  const { values, save } = useSettingsForm();
  const { t } = useTranslation();
  return (
    <Card title={t('admin.linkParsing')} description={t('admin.linkNote')}>
      <Toggle checked={bool(values, 'linkPreviewEnabled', true)} onChange={(v) => void save({ linkPreviewEnabled: v })} label={t('admin.linkPreviewEnabled')} />
    </Card>
  );
}

export function LanguageSettings() {
  const { t } = useTranslation();
  const language = useUiStore((s) => s.language);
  const setLanguage = useUiStore((s) => s.setLanguage);
  const theme = useUiStore((s) => s.theme);
  const setTheme = useUiStore((s) => s.setTheme);
  return (
    <Card title={t('admin.language')}>
      <Field label={t('common.language')}>
        <select className={inputClass} value={language} onChange={(e) => {
          const next = e.target.value as Language;
          setLanguage(next);
          setI18nLanguage(next);
        }}>
          <option value="zh-CN">简体中文</option>
          <option value="en">English</option>
        </select>
      </Field>
      <Field label={t('common.theme')}>
        <select className={inputClass} value={theme} onChange={(e) => setTheme(e.target.value as Theme)}>
          <option value="light">{t('common.light')}</option>
          <option value="dark">{t('common.dark')}</option>
          <option value="system">{t('common.system')}</option>
        </select>
      </Field>
    </Card>
  );
}

export function ShortcutsSettings() {
  const { t } = useTranslation();
  const rows: Array<[string, string]> = [
    ['Ctrl + V', t('shortcuts.paste')],
    ['Ctrl + Enter', t('shortcuts.send')],
    ['Ctrl + K', t('shortcuts.search')],
    ['Esc', t('shortcuts.escape')],
  ];
  return (
    <Card title={t('admin.shortcuts')}>
      <ul className="flex flex-col gap-2">
        {rows.map(([key, label]) => (
          <li key={key} className="flex items-center justify-between rounded-lg border border-slate-200 px-3 py-2 text-sm dark:border-slate-700">
            <span>{label}</span>
            <kbd className="rounded bg-slate-100 px-2 py-0.5 font-mono text-xs dark:bg-slate-800">{key}</kbd>
          </li>
        ))}
      </ul>
    </Card>
  );
}

export function NotificationSettings() {
  const { values, save } = useSettingsForm();
  const { t } = useTranslation();
  const keys = ['notifyNewDevice', 'notifyUploadDone', 'notifyUploadFailed', 'notifyLowStorage', 'notifyExpiring'];
  const labels: Record<string, string> = {
    notifyNewDevice: t('notifications.newDevice'),
    notifyUploadDone: t('notifications.uploadDone'),
    notifyUploadFailed: t('notifications.uploadFailed'),
    notifyLowStorage: t('notifications.lowStorage'),
    notifyExpiring: t('notifications.expiring'),
  };
  const [draft, setDraft] = useState<Settings>({});
  return (
    <Card title={t('admin.notifications')}>
      {keys.map((key) => (
        <Toggle key={key} checked={bool({ ...values, ...draft }, key, true)} onChange={(v) => setDraft((d) => ({ ...d, [key]: v }))} label={labels[key]} />
      ))}
      <Button variant="primary" className="self-start" onClick={() => void save(draft)} disabled={Object.keys(draft).length === 0}>{t('common.save')}</Button>
    </Card>
  );
}
