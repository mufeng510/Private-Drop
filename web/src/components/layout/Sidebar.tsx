import { NavLink, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { primaryNav } from '../../nav';
import { useAuthStore } from '../../store/auth';
import { useNotificationsStore } from '../../store/notifications';
import { useUploadsStore } from '../../store/uploads';
import { useUiStore } from '../../store/ui';
import { Icon } from '../ui/Icon';

export function Sidebar() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const unread = useNotificationsStore((s) => s.unread);
  const activeUploads = useUploadsStore((s) => s.tasks.filter((x) => ['uploading', 'hashing', 'initializing', 'completing'].includes(x.status)).length);
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const bumpData = useUiStore((s) => s.bumpData);

  return (
    <nav aria-label={t('nav.home')} className="flex h-full w-60 shrink-0 flex-col border-r border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
      <div className="flex items-center gap-2.5 px-5 py-5">
        <span className="flex size-9 items-center justify-center rounded-xl bg-blue-600 text-white"><Icon name="sparkles" size={20} /></span>
        <div className="leading-tight">
          <p className="text-sm font-semibold">Private Drop</p>
          <p className="text-[11px] text-slate-400">NAS message hub</p>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-2.5 pb-4">
        {primaryNav.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) =>
              `mb-0.5 flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors ${
                isActive
                  ? 'bg-blue-50 font-medium text-blue-700 dark:bg-blue-950/50 dark:text-blue-300'
                  : 'text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'
              }`
            }
          >
            <Icon name={item.icon} size={19} />
            <span className="flex-1">{t(item.labelKey)}</span>
            {item.to === '/notifications' && unread > 0 && (
              <span className="rounded-full bg-red-500 px-1.5 py-0.5 text-[10px] font-semibold text-white">{unread}</span>
            )}
          </NavLink>
        ))}
      </div>

      <div className="border-t border-slate-200 p-2.5 dark:border-slate-800">
        <button
          type="button"
          onClick={() => navigate('/admin')}
          className="mb-0.5 flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
        >
          <Icon name="shield" size={19} />
          <span className="flex-1">{t('nav.adminArea')}</span>
          <Icon name="chevronRight" size={15} />
        </button>
        <div className="flex items-center gap-2 rounded-lg px-3 py-2">
          <span className="flex size-8 items-center justify-center rounded-full bg-slate-100 text-slate-500 dark:bg-slate-800"><Icon name="user" size={17} /></span>
          <div className="min-w-0 flex-1 leading-tight">
            <p className="truncate text-xs font-medium">{user?.username ?? '—'}</p>
            <p className="truncate text-[10px] text-slate-400">{activeUploads > 0 ? `${activeUploads} ${t('upload.title').toLowerCase()}` : t('nav.home')}</p>
          </div>
          <button
            type="button"
            aria-label={t('nav.logout')}
            title={t('nav.logout')}
            onClick={async () => {
              await logout();
              bumpData();
              navigate('/login', { replace: true });
            }}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-red-600 dark:hover:bg-slate-800"
          >
            <Icon name="logout" size={17} />
          </button>
        </div>
      </div>
    </nav>
  );
}
