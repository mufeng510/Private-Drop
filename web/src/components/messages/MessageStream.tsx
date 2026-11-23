import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Attachment, Message, MessageSort } from '../../api/types';
import { messagesApi } from '../../api/messages';
import { filesApi } from '../../api/files';
import { errorMessage } from '../../api/client';
import { downloadBlob } from '../../lib/blob';
import { ESC_EVENT } from '../../hooks/useKeyboardShortcuts';
import { useUiStore } from '../../store/ui';
import { useTagsStore } from '../../store/tags';
import { MessageCard } from './MessageCard';
import { FilePreview } from '../FilePreview';
import { TagPicker } from '../ui/TagPicker';
import { Button, EmptyState, Spinner } from '../ui/primitives';
import { Icon } from '../ui/Icon';

const sorts: MessageSort[] = ['newest', 'oldest', 'favorite', 'pinned'];

export function MessageStream({
  messages,
  pinned = [],
  loading = false,
  hasMore = false,
  onLoadMore,
  onChanged,
  showSort = true,
  sort = 'newest',
  onSortChange,
  emptyTitle,
  chatLayout = false,
  controlsAtBottom = false,
}: {
  messages: Message[];
  pinned?: Message[];
  loading?: boolean;
  hasMore?: boolean;
  onLoadMore?: () => void;
  onChanged: () => void;
  showSort?: boolean;
  sort?: MessageSort;
  onSortChange?: (sort: MessageSort) => void;
  emptyTitle?: string;
  /** Show messages oldest → newest (newest at the bottom), chat-app style. */
  chatLayout?: boolean;
  /** Render the sort / select controls below the stream instead of above it. */
  controlsAtBottom?: boolean;
}) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [selectMode, setSelectMode] = useState(false);
  const [preview, setPreview] = useState<Attachment | null>(null);
  const [bulkTagOpen, setBulkTagOpen] = useState(false);
  const confirm = useUiStore((s) => s.requestConfirm);
  const toast = useUiStore((s) => s.toast);
  const tags = useTagsStore((s) => s.items);
  const sentinel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setSelected(new Set());
    setSelectMode(false);
  }, [sort]);

  // Escape exits the current operation (taskbook §20).
  useEffect(() => {
    const onEscape = () => {
      setSelected(new Set());
      setSelectMode(false);
      setBulkTagOpen(false);
    };
    window.addEventListener(ESC_EVENT, onEscape);
    return () => window.removeEventListener(ESC_EVENT, onEscape);
  }, []);

  useEffect(() => {
    const node = sentinel.current;
    if (!node || !hasMore || !onLoadMore) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) onLoadMore();
      },
      { rootMargin: '300px' },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasMore, onLoadMore]);

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const bulk = async (action: 'delete' | 'favorite' | 'unfavorite' | 'pin' | 'unpin' | 'tag' | 'untag', tagId?: string) => {
    const ids = [...selected];
    if (ids.length === 0) return;
    if (action === 'delete') {
      const ok = await confirm({ title: t('messages.bulkDelete'), message: t('common.selected', { count: ids.length }), danger: true, confirmLabel: t('common.delete') });
      if (!ok) return;
    }
    try {
      await messagesApi.bulk({ ids, action, tagId });
      setSelected(new Set());
      setSelectMode(false);
      setBulkTagOpen(false);
      toast(t('common.saved'), 'success');
      onChanged();
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const downloadSelectedAttachments = async () => {
    const ids = [...pinned, ...messages]
      .filter((m) => selected.has(m.id))
      .flatMap((m) => m.attachments.map((a) => a.id))
      .filter((id, i, arr) => arr.indexOf(id) === i);
    if (ids.length === 0) {
      toast(t('files.noFiles'), 'info');
      return;
    }
    try {
      downloadBlob(await filesApi.zip(ids, false), 'private-drop.zip');
      toast(t('common.saved'), 'success');
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const all = [...pinned, ...messages];
  const allSelected = all.length > 0 && selected.size === all.length;

  // In chat layout the store stays in API order (newest first), but we paint it
  // oldest → newest so the newest message sits at the bottom of the stream.
  const shownMessages = chatLayout ? [...messages].reverse() : messages;
  const shownPinned = chatLayout ? [...pinned].reverse() : pinned;

  const loadMoreBlock = hasMore && !loading ? (
    <div ref={sentinel} className="flex justify-center py-2">
      <Button variant="secondary" onClick={() => onLoadMore?.()}>{t('messages.loadMore')}</Button>
    </div>
  ) : null;

  const loadingBlock = loading ? (
    <div className="flex justify-center py-6 text-slate-400"><Spinner /></div>
  ) : null;

  const controls = (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {showSort && (
          <div className="inline-flex rounded-lg border border-slate-200 bg-white p-0.5 dark:border-slate-700 dark:bg-slate-800">
            {sorts.map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => onSortChange?.(s)}
                aria-pressed={sort === s}
                className={`rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors ${
                  sort === s ? 'bg-blue-600 text-white' : 'text-slate-500 hover:text-slate-800 dark:hover:text-white'
                }`}
              >
                {t(`messages.sort${s[0].toUpperCase()}${s.slice(1)}`)}
              </button>
            ))}
          </div>
        )}
        <div className="flex-1" />
        <Button
          variant={selectMode ? 'primary' : 'secondary'}
          icon="check"
          onClick={() => {
            setSelectMode((v) => !v);
            setSelected(new Set());
          }}
        >
          {selectMode ? t('messages.exitSelectMode') : t('messages.selectMode')}
        </Button>
      </div>

      {selectMode && all.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 rounded-xl border border-blue-200 bg-blue-50 p-2 dark:border-blue-900 dark:bg-blue-950/30">
          <span className="px-1 text-xs font-medium text-blue-700 dark:text-blue-300">{t('common.selected', { count: selected.size })}</span>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => setSelected(allSelected ? new Set() : new Set(all.map((m) => m.id)))}>
            {allSelected ? t('common.clearSelection') : t('common.selectAll')}
          </Button>
          <div className="flex-1" />
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('favorite')} disabled={selected.size === 0}>{t('messages.bulkFavorite')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('unfavorite')} disabled={selected.size === 0}>{t('messages.bulkUnfavorite')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('pin')} disabled={selected.size === 0}>{t('messages.bulkPin')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('unpin')} disabled={selected.size === 0}>{t('messages.bulkUnpin')}</Button>
          <div className="relative">
            <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => setBulkTagOpen((v) => !v)} disabled={selected.size === 0} icon="tag">{t('messages.bulkTag')}</Button>
            {bulkTagOpen && (
              <div className="absolute bottom-full right-0 z-20 mb-1 flex w-56 flex-col gap-1 rounded-xl border border-slate-200 bg-white p-2 shadow-xl dark:border-slate-700 dark:bg-slate-800">
                {tags.filter((tag) => !tag.isSystem).map((tag) => (
                  <button key={tag.id} type="button" onClick={() => void bulk('tag', tag.id)} className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700">
                    <span className="size-2.5 rounded-full" style={{ background: tag.color || '#64748b' }} />{tag.name}
                  </button>
                ))}
                <TagPicker compact selectedIds={[]} onChange={(ids) => ids[0] && void bulk('tag', ids[0])} />
              </div>
            )}
          </div>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('untag')} disabled={selected.size === 0}>{t('messages.bulkUntag')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void downloadSelectedAttachments()} disabled={selected.size === 0} icon="download">{t('files.downloadZip')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs hover:!text-red-600" onClick={() => void bulk('delete')} disabled={selected.size === 0} icon="trash">{t('messages.bulkDelete')}</Button>
        </div>
      )}
    </>
  );

  const stream = (
    <>
      {chatLayout && loadMoreBlock}
      {chatLayout && loadingBlock}

      {pinned.length > 0 && (
        <section aria-label={t('messages.pinned')}>
          <h2 className="mb-2 flex items-center gap-1.5 px-1 text-xs font-semibold uppercase tracking-wide text-amber-600 dark:text-amber-400"><Icon name="pin" size={14} /> {t('messages.pinned')}</h2>
          <div className="flex flex-col gap-3">
            {shownPinned.map((message) => (
              <MessageCard key={message.id} message={message} selected={selected.has(message.id)} selectMode={selectMode} onToggleSelect={() => toggle(message.id)} onPreview={setPreview} onChanged={onChanged} />
            ))}
          </div>
          <div className="my-3 border-t border-dashed border-slate-200 dark:border-slate-700" />
        </section>
      )}

      {messages.length === 0 && !loading ? (
        <EmptyState icon="message" title={emptyTitle || t('messages.noMessages')} />
      ) : (
        <div className="flex flex-col gap-3">
          {shownMessages.map((message) => (
            <MessageCard key={message.id} message={message} selected={selected.has(message.id)} selectMode={selectMode} onToggleSelect={() => toggle(message.id)} onPreview={setPreview} onChanged={onChanged} />
          ))}
        </div>
      )}

      {!chatLayout && loadingBlock}
      {!chatLayout && loadMoreBlock}
      {!hasMore && messages.length > 0 && <p className="py-6 text-center text-xs text-slate-300">— {t('common.none')} —</p>}
    </>
  );

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-3 px-3 py-4 sm:px-5">
      {!controlsAtBottom && controls}
      {stream}
      {controlsAtBottom && controls}
      <FilePreview file={preview} onClose={() => setPreview(null)} />
    </div>
  );
}
