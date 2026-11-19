import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useUiStore } from '../../store/ui';
import { useUploadsStore } from '../../store/uploads';
import { useAuthStore } from '../../store/auth';
import { Icon } from '../ui/Icon';
import { IconButton } from '../ui/primitives';

const titleMap: Record<string, string> = {
  '/': 'messages.title',
  '/files': 'files.title',
  '/search': 'search.title',
  '/favorites': 'nav.favorites',
  '/tags': 'tags.title',
  '/expiring': 'expiring.title',
  '/recycle': 'recycle.title',
  '/notifications': 'notifications.title',
  '/settings': 'admin.title',
  '/admin': 'admin.title',
};

export function TopBar() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const theme = useUiStore((s) => s.theme);
  const setTheme = useUiStore((s) => s.setTheme);
  const language = useUiStore((s) => s.language);
  const setLanguageAction = useUiStore((s) => s.setLanguage);
  const setUploadPanelOpen = useUiStore((s) => s.setUploadPanelOpen);
  const setSidebarOpen = useUiStore((s) => s.setSidebarOpen);
  const activeUploads = useUploadsStore((s) => s.tasks.filter((x) => ['uploading', 'hashing', 'initializing', 'completing'].includes(x.status)).length);
  const adminUnlocked = useAuthStore((s) => s.adminUnlocked);
  const title = titleMap[location.pathname] ?? 'nav.home';

  const toggleTheme = () => setTheme(theme === 'dark' ? 'light' : 'dark');

  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-slate-200 bg-white/90 px-3 backdrop-blur dark:border-slate-800 dark:bg-slate-900/90 md:px-5">
      <IconButton icon="menu" label="Menu" className="md:hidden" onClick={() => setSidebarOpen(true)} />
      <h1 className="flex-1 truncate text-base font-semibold">{t(title)}</h1>

      <IconButton icon="search" label={t('search.title')} onClick={() => navigate('/search')} />
      <div className="relative">
        <IconButton icon="upload" label={t('upload.panel')} onClick={() => setUploadPanelOpen(true)} />
        {activeUploads > 0 && (
          <span className="pointer-events-none absolute right-0.5 top-0.5 min-w-4 rounded-full bg-blue-600 px-1 text-center text-[9px] font-semibold text-white">{activeUploads}</span>
        )}
      </div>
      <IconButton
        icon={theme === 'dark' ? 'sun' : 'moon'}
        label={t('common.theme')}
        onClick={toggleTheme}
      />
      <button
        type="button"
        onClick={() => setLanguageAction(language === 'zh-CN' ? 'en' : 'zh-CN')}
        className="hidden rounded-lg px-2 py-1 text-xs font-medium text-slate-500 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800 sm:block"
        aria-label={t('common.language')}
      >
        {language === 'zh-CN' ? '中文' : 'EN'}
      </button>
      <button
        type="button"
        onClick={() => navigate(location.pathname.startsWith('/admin') ? '/' : '/admin')}
        className={`ml-1 hidden items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-medium sm:inline-flex ${
          adminUnlocked
            ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
            : 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'
        }`}
      >
        <Icon name="shield" size={15} />
        {location.pathname.startsWith('/admin') ? t('nav.backToApp') : t('nav.openAdmin')}
      </button>
    </header>
  );
}
