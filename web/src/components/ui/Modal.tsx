import { useEffect, useId, useRef, type ReactNode } from 'react';
import { Icon } from './Icon';

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'full';
  /** Allow click-outside / Esc to dismiss. Default true. */
  dismissible?: boolean;
}

const widths: Record<NonNullable<ModalProps['size']>, string> = {
  sm: 'max-w-sm',
  md: 'max-w-lg',
  lg: 'max-w-3xl',
  full: 'max-w-[min(96vw,1100px)] h-[92vh]',
};

/**
 * Native <dialog> modal: the browser provides focus containment, top-layer,
 * Esc and inert page. No manual focus trap or role/aria-modal is added.
 */
export function Modal({ open, onClose, title, children, footer, size = 'md', dismissible = true }: ModalProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (open && !dialog.open) dialog.showModal();
    if (!open && dialog.open) dialog.close();
  }, [open]);

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    const onCancel = (e: Event) => {
      if (!dismissible) {
        e.preventDefault();
        return;
      }
      onClose();
    };
    dialog.addEventListener('cancel', onCancel);
    return () => dialog.removeEventListener('cancel', onCancel);
  }, [onClose, dismissible]);

  // Safari lacks `closedby`; reproduce click-outside (pointerdown + pointerup
  // both outside) as progressive enhancement. Esc already works natively.
  useEffect(() => {
    const dialog = ref.current;
    if (!dialog || !open || !dismissible) return;
    if ('closedBy' in HTMLDialogElement.prototype || typeof HTMLDialogElement === 'undefined') return;
    let pressedOutside = false;
    const outside = (e: PointerEvent) => {
      const r = dialog.getBoundingClientRect();
      return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom;
    };
    const down = (e: PointerEvent) => {
      pressedOutside = outside(e);
    };
    const up = (e: PointerEvent) => {
      if (pressedOutside && outside(e)) onClose();
    };
    dialog.addEventListener('pointerdown', down);
    dialog.addEventListener('pointerup', up);
    return () => {
      dialog.removeEventListener('pointerdown', down);
      dialog.removeEventListener('pointerup', up);
    };
  }, [open, dismissible, onClose]);

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      {...({ closedby: dismissible ? 'any' : 'none' } as Record<string, string>)}
      className={`w-[calc(100%-1.5rem)] ${widths[size]} rounded-2xl border border-slate-200 bg-white p-0 text-slate-900 shadow-2xl dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100`}
    >
      <div className="flex max-h-[92vh] flex-col">
        <header className="flex items-center justify-between gap-3 border-b border-slate-200 px-5 py-3.5 dark:border-slate-700">
          <h2 id={titleId} className="truncate text-base font-semibold">{title}</h2>
          {dismissible && (
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg p-1.5 text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:hover:bg-slate-800 dark:hover:text-white"
              aria-label="Close"
            >
              <Icon name="x" size={18} />
            </button>
          )}
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">{children}</div>
        {footer && (
          <footer className="flex justify-end gap-2 border-t border-slate-200 px-5 py-3 dark:border-slate-700">
            {footer}
          </footer>
        )}
      </div>
    </dialog>
  );
}
