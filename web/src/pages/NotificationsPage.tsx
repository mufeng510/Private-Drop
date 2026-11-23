import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { NotificationItem } from '../api/types';
import { errorMessage } from '../api/client';
import { useNotificationsStore } from '../store/notifications';
import { useUiStore } from '../store/ui';
import { Button, EmptyState, IconButton } from '../components/ui/primitives';
import { Icon, type IconName } from '../components/ui/Icon';
import { formatRelativeTime } from '../lib/format';

const routeFor = (item: NotificationItem): string => {
  if (item.link) return item.link;
  const type = item.type || '';
  if (type.includes('upload')) return '/';
  if (type.includes('device') || type.includes('security') || type.includes('login')) return '/admin';
  if (type.includes('expir')) return '/expiring';
  if (type.includes('storage')) return '/admin';
  return '/notifications';
};

const iconFor: Record<string, IconName> = {
  new_device: 'devices', device_request: 'devices', login_failed: 'alert',
  upload_failed: 'alert', upload_completed: 'upload', low_storage: 'database',
  expiring: 'clock', security: 'shield',
};

export function NotificationsPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const items = useNotificationsStore((s) => s.items);
  const unread = useNotificationsStore((s) => s.unread);
  const load = useNotificationsStore((s) => s.load);
  const readAll = useNotificationsStore((s) => s.readAll);
  const remove = useNotificationsStore((s) => s.remove);
  const toast = useUiStore((s) => s.toast);

  useEffect(() => { void load(); }, [load]);

  return (
    <div className="mx-auto w-full max-w-3xl px-3 py-4 sm:px-5">
      <div className="mb-4 flex items-center justify-between gap-2">
        <p className="text-xs text-slate-400">{unread > 0 ? t('notifications.unread', { count: unread }) : t('notifications.noNotifications')}</p>
        <Button variant="secondary" onClick={() => void readAll()} disabled={unread === 0}>{t('notifications.markAllRead')}</Button>
      </div>

      {items.length === 0 ? (
        <EmptyState icon="bell" title={t('notifications.noNotifications')} />
      ) : (
        <ul className="flex flex-col gap-2">
          {items.map((item) => (
            <li
              key={item.id}
              className={`flex items-start gap-3 rounded-xl border p-3 ${item.read ? 'border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900' : 'border-blue-200 bg-blue-50/50 dark:border-blue-900 dark:bg-blue-950/20'}`}
            >
              <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-slate-800">
                <Icon name={iconFor[item.type] || 'bell'} size={18} />
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">
                  {t(`notifications.${item.type}`, { defaultValue: t('notifications.system') })}
                </p>
                {typeof item.payload?.message === 'string' && <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{item.payload.message}</p>}
                <p className="mt-1 text-[11px] text-slate-400">{formatRelativeTime(item.createdAt, i18n.language)}</p>
                <button type="button" onClick={() => navigate(routeFor(item))} className="mt-1 inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:underline dark:text-blue-400">
                  {t('notifications.jump')} <Icon name="chevronRight" size={13} />
                </button>
              </div>
              <IconButton icon="x" label={t('notifications.dismiss')} onClick={() => void remove(item.id).catch((e) => toast(errorMessage(e), 'error'))} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
