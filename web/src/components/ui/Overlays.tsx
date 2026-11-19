import { useTranslation } from 'react-i18next';
import { useUiStore } from '../../store/ui';
import { Modal } from './Modal';
import { Button } from './primitives';
import { Icon } from './Icon';

export function ConfirmDialog() {
  const { t } = useTranslation();
  const confirm = useUiStore((s) => s.confirm);
  const resolveConfirm = useUiStore((s) => s.resolveConfirm);

  return (
    <Modal
      open={Boolean(confirm)}
      onClose={() => resolveConfirm(false)}
      title={confirm?.title ?? ''}
      size="sm"
      footer={
        <>
          <Button variant="secondary" onClick={() => resolveConfirm(false)} autoFocus>
            {confirm?.cancelLabel ?? t('common.cancel')}
          </Button>
          <Button variant={confirm?.danger ? 'danger' : 'primary'} onClick={() => resolveConfirm(true)}>
            {confirm?.confirmLabel ?? t('common.confirm')}
          </Button>
        </>
      }
    >
      <p className="text-sm text-slate-600 dark:text-slate-300">{confirm?.message}</p>
    </Modal>
  );
}

export function Toasts() {
  const { t } = useTranslation();
  const toasts = useUiStore((s) => s.toasts);
  const dismiss = useUiStore((s) => s.dismissToast);
  return (
    <div className="pointer-events-none fixed bottom-20 right-4 z-[100] flex w-[min(92vw,360px)] flex-col gap-2 md:bottom-5" role="status" aria-live="polite">
      {toasts.map((toast) => {
        const icon = toast.tone === 'error' ? 'alert' : toast.tone === 'success' ? 'success' : 'info';
        const color = toast.tone === 'error'
          ? 'text-red-600 dark:text-red-400'
          : toast.tone === 'success'
            ? 'text-emerald-600 dark:text-emerald-400'
            : 'text-blue-600 dark:text-blue-400';
        return (
          <div key={toast.id} className="pointer-events-auto flex items-start gap-2 rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm shadow-lg dark:border-slate-700 dark:bg-slate-800">
            <Icon name={icon} size={18} className={`mt-0.5 shrink-0 ${color}`} />
            <span className="flex-1 break-words text-slate-700 dark:text-slate-200">{t(toast.message) || toast.message}</span>
            <button type="button" onClick={() => dismiss(toast.id)} aria-label="Dismiss" className="text-slate-400 hover:text-slate-700">
              <Icon name="x" size={16} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
