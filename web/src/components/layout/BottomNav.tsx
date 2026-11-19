import { NavLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { bottomNav } from '../../nav';
import { useNotificationsStore } from '../../store/notifications';
import { Icon } from '../ui/Icon';

export function BottomNav() {
  const { t } = useTranslation();
  const unread = useNotificationsStore((s) => s.unread);
  return (
    <nav
      aria-label={t('nav.home')}
      className="fixed inset-x-0 bottom-0 z-40 flex border-t border-slate-200 bg-white/95 pb-[env(safe-area-inset-bottom)] backdrop-blur dark:border-slate-800 dark:bg-slate-900/95 md:hidden"
    >
      {bottomNav.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.end}
          className={({ isActive }) =>
            `relative flex min-h-14 flex-1 flex-col items-center justify-center gap-0.5 text-[10px] ${
              isActive ? 'text-blue-600 dark:text-blue-400' : 'text-slate-500 dark:text-slate-400'
            }`
          }
        >
          <Icon name={item.icon} size={21} />
          <span>{t(item.labelKey)}</span>
          {item.to === '/notifications' && unread > 0 && (
            <span className="absolute right-[22%] top-1.5 min-w-4 rounded-full bg-red-500 px-1 text-center text-[9px] font-semibold text-white">{unread}</span>
          )}
        </NavLink>
      ))}
    </nav>
  );
}
