import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { RecycleItem } from '../api/types';
import { recycleApi } from '../api/recycle';
import { errorMessage } from '../api/client';
import { ESC_EVENT } from '../hooks/useKeyboardShortcuts';
import { useUiStore } from '../store/ui';
import { Modal } from '../components/ui/Modal';
import { Button, EmptyState, IconButton } from '../components/ui/primitives';
import { Icon } from '../components/ui/Icon';
import { formatDateTime, formatRelativeTime } from '../lib/format';

export function RecyclePage() {
  const { t, i18n } = useTranslation();
  const [items, setItems] = useState<RecycleItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [selectMode, setSelectMode] = useState(false);
  /** Ids awaiting a restore-mode choice; works for one item or a batch. */
  const [restoreIds, setRestoreIds] = useState<string[] | null>(null);
  const confirm = useUiStore((s) => s.requestConfirm);
  const toast = useUiStore((s) => s.toast);
  const dataVersion = useUiStore((s) => s.dataVersion);
  const bumpData = useUiStore((s) => s.bumpData);

  const load = async () => {
    setLoading(true);
    try {
      const res = await recycleApi.list({ limit: 200 });
      setItems(res.items);
    } catch (e) {
      toast(errorMessage(e), 'error');
    } finally {
      setLoading(false);
    }
  };

  // Reload when realtime changes bump the data version.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void load(); }, [dataVersion]);
  useEffect(() => {
    const onEscape = () => { setSelectMode(false); setSelected(new Set()); };
    window.addEventListener(ESC_EVENT, onEscape);
    return () => window.removeEventListener(ESC_EVENT, onEscape);
  }, []);

  const restore = async (ids: string[], mode: 'reset' | 'original') => {
    try {
      await Promise.all(ids.map((id) => recycleApi.restore(id, mode)));
      setRestoreIds(null);
      setSelected(new Set());
      setSelectMode(false);
      toast(t('common.saved'), 'success');
      bumpData();
      void load();
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const permanentDelete = async (ids: string[]) => {
    const ok = await confirm({
      title: t('recycle.permanentDelete'),
      message: ids.length === 1 ? (items.find((x) => x.id === ids[0])?.name || items.find((x) => x.id === ids[0])?.text || ids[0]) : t('recycle.confirmPermanent'),
      danger: true,
      confirmLabel: t('common.delete'),
    });
    if (!ok) return;
    try {
      await Promise.all(ids.map((id) => recycleApi.remove(id)));
      setSelected(new Set());
      setSelectMode(false);
      void load();
      bumpData();
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const empty = async () => {
    const ok = await confirm({ title: t('recycle.emptyBin'), message: t('recycle.confirmEmpty'), danger: true, confirmLabel: t('common.delete') });
    if (!ok) return;
    try {
      await recycleApi.empty();
      void load();
      bumpData();
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const allSelected = items.length > 0 && selected.size === items.length;

  return (
    <div className="mx-auto w-full max-w-3xl px-3 py-4 sm:px-5">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-slate-400">{t('recycle.subtitle')}</p>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant={selectMode ? 'primary' : 'secondary'} icon="check" onClick={() => { setSelectMode((v) => !v); setSelected(new Set()); }}>
            {selectMode ? t('messages.exitSelectMode') : t('messages.selectMode')}
          </Button>
          <Button variant="danger" icon="trash" onClick={() => void empty()} disabled={items.length === 0}>{t('recycle.emptyBin')}</Button>
        </div>
      </div>

      {selectMode && items.length > 0 && (
        <div className="mb-3 flex flex-wrap items-center gap-2 rounded-xl border border-blue-200 bg-blue-50 p-2 dark:border-blue-900 dark:bg-blue-950/30">
          <span className="px-1 text-xs font-medium text-blue-700 dark:text-blue-300">{t('common.selected', { count: selected.size })}</span>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => setSelected(allSelected ? new Set() : new Set(items.map((x) => x.id)))}>
            {allSelected ? t('common.clearSelection') : t('common.selectAll')}
          </Button>
          <div className="flex-1" />
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" icon="restore" onClick={() => selected.size && setRestoreIds([...selected])} disabled={selected.size === 0}>{t('recycle.restore')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs hover:!text-red-600" icon="trash" onClick={() => void permanentDelete([...selected])} disabled={selected.size === 0}>{t('recycle.permanentDelete')}</Button>
        </div>
      )}

      {items.length === 0 && !loading ? (
        <EmptyState icon="archive" title={t('recycle.noItems')} />
      ) : (
        <ul className="flex flex-col gap-2">
          {items.map((item) => (
            <li key={item.id} className="flex items-center gap-3 rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
              {selectMode && (
                <input type="checkbox" checked={selected.has(item.id)} onChange={() => toggle(item.id)} aria-label={t('messages.selectMode')} className="size-4 shrink-0 accent-blue-600" />
              )}
              <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-slate-800">
                <Icon name={item.kind === 'message' ? 'message' : 'file'} size={18} />
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{item.name || item.text || t('recycle.items', { count: item.attachmentCount ?? 0 })}</p>
                <p className="text-xs text-slate-400">{t('recycle.deletedAt', { time: formatRelativeTime(item.deletedAt, i18n.language) })} · {formatDateTime(item.deletedAt, i18n.language)}</p>
              </div>
              <Button variant="secondary" icon="restore" onClick={() => setRestoreIds([item.id])}>{t('recycle.restore')}</Button>
              <IconButton icon="trash" label={t('recycle.permanentDelete')} className="hover:text-red-600" onClick={() => void permanentDelete([item.id])} />
            </li>
          ))}
        </ul>
      )}

      <Modal open={restoreIds !== null} onClose={() => setRestoreIds(null)} title={t('recycle.modeTitle')} size="sm">
        <div className="flex flex-col gap-2">
          <Button variant="secondary" onClick={() => restoreIds && void restore(restoreIds, 'reset')}>{t('recycle.restoreReset')}</Button>
          <Button variant="secondary" onClick={() => restoreIds && void restore(restoreIds, 'original')}>{t('recycle.restoreOriginal')}</Button>
        </div>
      </Modal>
      <div className="h-16 md:hidden" />
    </div>
  );
}
