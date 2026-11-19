import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useAuthStore } from '../store/auth';
import { useAdminSettings } from '../store/adminSettings';
import { useUiStore } from '../store/ui';
import { AdminGate } from './AdminGate';
import {
  AccountSecurity, DevicesAdmin, LanguageSettings, LinkParsingSettings, LoginPolicySettings,
  NotificationSettings, RetentionSettings, ShortcutsSettings, StorageWarningSettings, UploadSettings,
} from './admin/AdminSettings';
import { DiagnosticsView, LogsAndAudit, StorageStatsView, SystemStatusView } from './admin/AdminSystem';
import { Icon, type IconName } from '../components/ui/Icon';
import { Button } from '../components/ui/primitives';

type SectionId =
  | 'account' | 'policy' | 'devices' | 'retention' | 'uploads' | 'warning' | 'links'
  | 'language' | 'shortcuts' | 'notifications' | 'logs' | 'system' | 'storage' | 'diagnose';

const sections: Array<{ id: SectionId; labelKey: string; icon: IconName }> = [
  { id: 'account', labelKey: 'admin.account', icon: 'user' },
  { id: 'policy', labelKey: 'admin.loginPolicy', icon: 'lock' },
  { id: 'devices', labelKey: 'admin.devices', icon: 'devices' },
  { id: 'retention', labelKey: 'admin.retention', icon: 'clock' },
  { id: 'uploads', labelKey: 'admin.uploadLimits', icon: 'upload' },
  { id: 'warning', labelKey: 'admin.storageWarning', icon: 'database' },
  { id: 'links', labelKey: 'admin.linkParsing', icon: 'link' },
  { id: 'language', labelKey: 'admin.language', icon: 'globe' },
  { id: 'shortcuts', labelKey: 'admin.shortcuts', icon: 'keyboard' },
  { id: 'notifications', labelKey: 'admin.notifications', icon: 'bell' },
  { id: 'logs', labelKey: 'admin.logs', icon: 'list' },
  { id: 'system', labelKey: 'admin.system', icon: 'activity' },
  { id: 'storage', labelKey: 'admin.storage', icon: 'chart' },
  { id: 'diagnose', labelKey: 'admin.diagnostics', icon: 'shield' },
];

export function AdminPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const unlocked = useAuthStore((s) => s.adminUnlocked);
  const setAdminUnlocked = useAuthStore((s) => s.setAdminUnlocked);
  const loadSettings = useAdminSettings((s) => s.load);
  const bumpData = useUiStore((s) => s.bumpData);
  const [section, setSection] = useState<SectionId>('account');

  useEffect(() => {
    if (unlocked) void loadSettings();
  }, [unlocked, loadSettings]);

  if (!unlocked) {
    return (
      <div>
        <div className="mx-auto max-w-3xl px-3 py-3"><Button variant="ghost" icon="arrowLeft" onClick={() => navigate('/')}>{t('nav.backToApp')}</Button></div>
        <AdminGate />
      </div>
    );
  }

  const render = () => {
    switch (section) {
      case 'account': return <AccountSecurity />;
      case 'policy': return <LoginPolicySettings />;
      case 'devices': return <DevicesAdmin />;
      case 'retention': return <RetentionSettings />;
      case 'uploads': return <UploadSettings />;
      case 'warning': return <StorageWarningSettings />;
      case 'links': return <LinkParsingSettings />;
      case 'language': return <LanguageSettings />;
      case 'shortcuts': return <ShortcutsSettings />;
      case 'notifications': return <NotificationSettings />;
      case 'logs': return <LogsAndAudit />;
      case 'system': return <SystemStatusView />;
      case 'storage': return <StorageStatsView />;
      case 'diagnose': return <DiagnosticsView />;
    }
  };

  return (
    <div className="mx-auto flex w-full max-w-6xl gap-5 px-3 py-4 sm:px-5">
      <nav aria-label={t('admin.title')} className="hidden w-52 shrink-0 self-start rounded-2xl border border-slate-200 bg-white p-2 dark:border-slate-800 dark:bg-slate-900 md:block">
        {sections.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => setSection(item.id)}
            aria-pressed={section === item.id}
            className={`mb-0.5 flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm ${section === item.id ? 'bg-blue-50 font-medium text-blue-700 dark:bg-blue-950/50 dark:text-blue-300' : 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'}`}
          >
            <Icon name={item.icon} size={17} /> {t(item.labelKey)}
          </button>
        ))}
        <div className="mt-2 border-t border-slate-200 pt-2 dark:border-slate-700">
          <Button variant="ghost" className="w-full justify-start" icon="logout" onClick={() => { setAdminUnlocked(false); bumpData(); navigate('/'); }}>{t('nav.backToApp')}</Button>
        </div>
      </nav>

      <div className="min-w-0 flex-1">
        <div className="mb-4 flex items-center justify-between gap-2">
          <div>
            <h2 className="text-lg font-semibold">{t(sections.find((s) => s.id === section)?.labelKey || 'admin.title')}</h2>
            <p className="text-xs text-slate-400">{t('admin.subtitle')}</p>
          </div>
          <select
            value={section}
            onChange={(e) => setSection(e.target.value as SectionId)}
            className="rounded-lg border border-slate-300 bg-white px-2 py-1.5 text-sm dark:border-slate-600 dark:bg-slate-800 md:hidden"
            aria-label={t('admin.title')}
          >
            {sections.map((item) => <option key={item.id} value={item.id}>{t(item.labelKey)}</option>)}
          </select>
        </div>
        {render()}
        <div className="h-16 md:hidden" />
      </div>
    </div>
  );
}
