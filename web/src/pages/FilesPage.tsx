import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Attachment, AttachmentType } from '../api/types';
import { filesApi, type FilesBulkAction } from '../api/files';
import { errorMessage } from '../api/client';
import { downloadBlob } from '../lib/blob';
import { formatBytes, formatDateTime } from '../lib/format';
import { useDebounce } from '../hooks/useDebounce';
import { ESC_EVENT } from '../hooks/useKeyboardShortcuts';
import { useUiStore } from '../store/ui';
import { useTagsStore } from '../store/tags';
import { FilePreview } from '../components/FilePreview';
import { TagPicker } from '../components/ui/TagPicker';
import { AuthImage } from '../components/ui/AuthImage';
import { Button, EmptyState, IconButton, Spinner, TextInput, Toggle, inputClass } from '../components/ui/primitives';
import { Icon, type IconName } from '../components/ui/Icon';

const types: (AttachmentType | 'all')[] = ['all', 'image', 'video', 'audio', 'pdf', 'office', 'text', 'code', 'other'];
const typeIcon: Record<string, IconName> = {
  image: 'image', video: 'video', audio: 'audio', pdf: 'document', office: 'document', text: 'document', code: 'code', other: 'file',
};

export function FilesPage() {
  const { t, i18n } = useTranslation();
  const [items, setItems] = useState<Attachment[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [type, setType] = useState<string>('all');
  const [query, setQuery] = useState('');
  const [favorite, setFavorite] = useState(false);
  const [tag, setTag] = useState('');
  const [sort, setSort] = useState('newest');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [keepStructure, setKeepStructure] = useState(false);
  const [preview, setPreview] = useState<Attachment | null>(null);
  const debouncedQuery = useDebounce(query, 350);
  const dataVersion = useUiStore((s) => s.dataVersion);
  const toast = useUiStore((s) => s.toast);
  const confirm = useUiStore((s) => s.requestConfirm);
  const bumpData = useUiStore((s) => s.bumpData);
  const tags = useTagsStore((s) => s.items);
  const sentinel = useRef<HTMLDivElement>(null);

  useEffect(() => { void useTagsStore.getState().load(); }, []);

  // Escape exits selection mode.
  useEffect(() => {
    const onEscape = () => setSelected(new Set());
    window.addEventListener(ESC_EVENT, onEscape);
    return () => window.removeEventListener(ESC_EVENT, onEscape);
  }, []);

  const buildParams = useCallback((nextCursor?: string) => ({
    cursor: nextCursor,
    limit: 40,
    type: type === 'all' ? undefined : type,
    tag: tag || undefined,
    favorite: favorite || undefined,
    from: from || undefined,
    to: to || undefined,
    sort,
  }), [type, tag, favorite, from, to, sort]);

  const load = useCallback(async (reset = true) => {
    setLoading(true);
    try {
      const res = await filesApi.list(buildParams(reset ? undefined : cursor ?? undefined));
      setItems((prev) => reset ? res.items : [...prev, ...res.items.filter((f) => !prev.some((p) => p.id === f.id))]);
      setCursor(res.nextCursor);
    } catch (e) {
      toast(errorMessage(e), 'error');
    } finally {
      setLoading(false);
    }
  }, [buildParams, cursor, toast]);

  useEffect(() => {
    setCursor(null);
    void load(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [type, tag, favorite, sort, from, to, dataVersion]);

  useEffect(() => {
    if (!debouncedQuery.trim()) return;
    void load(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedQuery]);

  useEffect(() => {
    const node = sentinel.current;
    if (!node || !cursor) return;
    const observer = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting && !loading) void load(false);
    }, { rootMargin: '200px' });
    observer.observe(node);
    return () => observer.disconnect();
  }, [cursor, loading, load]);

  const toggleFavorite = async (file: Attachment) => {
    try {
      const updated = await filesApi.update(file.id, { favorited: !file.favorited });
      setItems((prev) => prev.map((f) => (f.id === file.id ? updated : f)));
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const download = async (file: Attachment) => {
    try {
      downloadBlob(await filesApi.download(file.id), file.name);
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const downloadSelected = async () => {
    try {
      downloadBlob(await filesApi.zip([...selected], keepStructure), 'private-drop.zip');
      toast(t('common.saved'), 'success');
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const bulk = async (action: FilesBulkAction, tagId?: string) => {
    if (selected.size === 0) return;
    const ids = [...selected];
    if (action === 'delete') {
      const ok = await confirm({ title: t('messages.bulkDelete'), message: t('common.selected', { count: ids.length }), danger: true, confirmLabel: t('common.delete') });
      if (!ok) return;
    }
    try {
      await filesApi.bulk({ ids, action, tagId });
      setSelected(new Set());
      toast(t('common.saved'), 'success');
      bumpData();
      void load(true);
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const shown = debouncedQuery.trim()
    ? items.filter((file) => file.name.toLowerCase().includes(debouncedQuery.trim().toLowerCase()))
    : items;

  return (
    <div className="mx-auto w-full max-w-6xl px-3 py-4 sm:px-5">
      <div className="mb-4 flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <TextInput value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('files.searchPlaceholder')} className="min-w-48 flex-1" />
          <select value={tag} onChange={(e) => setTag(e.target.value)} className={inputClass + ' w-auto'} aria-label={t('tags.title')}>
            <option value="">{t('tags.title')}</option>
            {tags.filter((x) => !x.isSystem).map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
          </select>
          <select value={sort} onChange={(e) => setSort(e.target.value)} className={inputClass + ' w-auto'} aria-label={t('common.sort')}>
            <option value="newest">{t('messages.sortNewest')}</option>
            <option value="oldest">{t('messages.sortOldest')}</option>
            <option value="name">{t('common.name')}</option>
            <option value="size">{t('common.size')}</option>
          </select>
          <Toggle checked={favorite} onChange={setFavorite} label={t('files.favoriteOnly')} />
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {types.map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => setType(item)}
              aria-pressed={type === item}
              className={`rounded-full px-3 py-1.5 text-xs font-medium ${type === item ? 'bg-blue-600 text-white' : 'bg-slate-100 text-slate-600 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-300'}`}
            >
              {t(item === 'all' ? 'files.allTypes' : `files.${item}`)}
            </button>
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500">
          <input type="date" value={from} onChange={(e) => setFrom(e.target.value)} aria-label={t('search.dateFrom')} className={inputClass + ' w-auto !py-1.5 !text-xs'} />
          <span>—</span>
          <input type="date" value={to} onChange={(e) => setTo(e.target.value)} aria-label={t('search.dateTo')} className={inputClass + ' w-auto !py-1.5 !text-xs'} />
          <div className="flex-1" />
          {selected.size > 0 && (
            <>
              <span className="font-medium text-blue-600 dark:text-blue-400">{t('common.selected', { count: selected.size })}</span>
              <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => setSelected(new Set())}>{t('common.clearSelection')}</Button>
              <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('favorite')}>{t('messages.bulkFavorite')}</Button>
              <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('unfavorite')}>{t('messages.bulkUnfavorite')}</Button>
              <TagPicker compact selectedIds={[]} onChange={(ids) => ids[0] && void bulk('tag', ids[0])} />
              <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('untag')}>{t('messages.bulkUntag')}</Button>
              <Toggle checked={keepStructure} onChange={setKeepStructure} label={t('files.keepStructure')} />
              <Button variant="primary" icon="download" onClick={() => void downloadSelected()}>{t('files.multiDownload', { count: selected.size })}</Button>
              <Button variant="ghost" className="!min-h-7 !px-2 !text-xs hover:!text-red-600" icon="trash" onClick={() => void bulk('delete')}>{t('messages.bulkDelete')}</Button>
            </>
          )}
        </div>
      </div>

      {shown.length === 0 && !loading ? (
        <EmptyState icon="folder" title={t('files.noFiles')} />
      ) : (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
          {shown.map((file) => (
            <article key={file.id} className="group overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
              <button type="button" onClick={() => setPreview(file)} className="relative flex aspect-square w-full items-center justify-center bg-slate-100 dark:bg-slate-800" aria-label={`${t('files.preview')} ${file.name}`}>
                {file.type === 'image' ? (
                  <AuthImage path={file.thumbnailUrl || `/api/files/${file.id}/thumbnail`} alt={file.name} className="size-full object-cover" />
                ) : (
                  <Icon name={typeIcon[file.type] || 'file'} size={36} className="text-slate-400" />
                )}
                <input
                  type="checkbox"
                  checked={selected.has(file.id)}
                  onChange={(e) => {
                    e.stopPropagation();
                    setSelected((prev) => {
                      const next = new Set(prev);
                      if (next.has(file.id)) next.delete(file.id);
                      else next.add(file.id);
                      return next;
                    });
                  }}
                  onClick={(e) => e.stopPropagation()}
                  aria-label={t('common.selectAll')}
                  className="absolute left-2 top-2 size-4 accent-blue-600"
                />
              </button>
              <div className="p-2.5">
                <p className="truncate text-sm font-medium" title={file.name}>{file.name}</p>
                <p className="text-xs text-slate-400">{formatBytes(file.size)} · {formatDateTime(file.uploadedAt, i18n.language)}</p>
                <div className="mt-1.5 flex items-center gap-0.5">
                  <IconButton icon="eye" label={t('files.preview')} onClick={() => setPreview(file)} />
                  <IconButton icon="star" label={file.favorited ? t('files.unfavorite') : t('files.favorite')} className={file.favorited ? 'text-yellow-500' : ''} onClick={() => void toggleFavorite(file)} />
                  <IconButton icon="download" label={t('common.download')} onClick={() => void download(file)} />
                </div>
              </div>
            </article>
          ))}
        </div>
      )}
      {loading && <div className="flex justify-center py-6"><Spinner /></div>}
      {cursor && !loading && <div ref={sentinel} className="h-10" />}
      <FilePreview file={preview} onClose={() => setPreview(null)} />
    </div>
  );
}
