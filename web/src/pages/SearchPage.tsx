import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { Attachment, Device, Message, SearchResult } from '../api/types';
import { searchApi } from '../api/search';
import { messagesApi, type BulkAction } from '../api/messages';
import { filesApi, type FilesBulkAction } from '../api/files';
import { errorMessage } from '../api/client';
import { downloadBlob } from '../lib/blob';
import { useDebounce } from '../hooks/useDebounce';
import { ESC_EVENT } from '../hooks/useKeyboardShortcuts';
import { useUiStore } from '../store/ui';
import { useTagsStore } from '../store/tags';
import { FilePreview } from '../components/FilePreview';
import { TagPicker } from '../components/ui/TagPicker';
import { Button, EmptyState, IconButton, Spinner, TextInput, inputClass } from '../components/ui/primitives';
import { Icon } from '../components/ui/Icon';
import { formatBytes, formatDateTime } from '../lib/format';

const MB = 1024 * 1024;
type ResultKey = string; // `${kind}:${id}`
type DeviceOption = Pick<Device, 'id' | 'name' | 'type'>;
const keyOf = (r: SearchResult): ResultKey => `${r.kind}:${r.id}`;

export function SearchPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [query, setQuery] = useState('');
  const debounced = useDebounce(query, 350);
  const [type, setType] = useState('');
  const [favorite, setFavorite] = useState(false);
  const [pinned, setPinned] = useState(false);
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [ext, setExt] = useState('');
  const [minSizeMb, setMinSizeMb] = useState('');
  const [maxSizeMb, setMaxSizeMb] = useState('');
  const [tag, setTag] = useState('');
  const [device, setDevice] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [devices, setDevices] = useState<DeviceOption[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<Attachment | null>(null);
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<ResultKey>>(new Set());
  const [bulkTagOpen, setBulkTagOpen] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const toast = useUiStore((s) => s.toast);
  const confirm = useUiStore((s) => s.requestConfirm);
  const tags = useTagsStore((s) => s.items);
  const inputRef = useRef<HTMLInputElement>(null);
  const refresh = () => setRefreshKey((k) => k + 1);

  useEffect(() => { inputRef.current?.focus(); }, []);
  useEffect(() => { void useTagsStore.getState().load(); }, []);

  // Escape exits selection / closes popovers.
  useEffect(() => {
    const onEscape = () => {
      setSelectMode(false);
      setSelected(new Set());
      setBulkTagOpen(false);
    };
    window.addEventListener(ESC_EVENT, onEscape);
    return () => window.removeEventListener(ESC_EVENT, onEscape);
  }, []);

  const collectDevices = (items: SearchResult[]) => {
    setDevices((prev) => {
      const map = new Map(prev.map((d) => [d.id, d]));
      for (const item of items) {
        const d = item.message?.device;
        if (d?.id && d.name && !map.has(d.id)) map.set(d.id, d);
      }
      return [...map.values()];
    });
  };

  const params = useMemo(() => ({
    q: debounced.trim(),
    type: type || undefined,
    favorite: favorite || undefined,
    pinned: pinned || undefined,
    from: from || undefined,
    to: to || undefined,
    ext: ext || undefined,
    minSize: minSizeMb ? Math.round(Number(minSizeMb) * MB) : undefined,
    maxSize: maxSizeMb ? Math.round(Number(maxSizeMb) * MB) : undefined,
    tag: tag || undefined,
    device: device || undefined,
  }), [debounced, type, favorite, pinned, from, to, ext, minSizeMb, maxSizeMb, tag, device]);

  useEffect(() => {
    if (!params.q) {
      setResults([]);
      setCursor(null);
      return;
    }
    let alive = true;
    setLoading(true);
    searchApi.search({ ...params, limit: 40 }).then((res) => {
      if (!alive) return;
      setResults(res.items);
      setCursor(res.nextCursor);
      collectDevices(res.items);
    }).catch((e) => alive && toast(errorMessage(e), 'error'))
      .finally(() => alive && setLoading(false));
    return () => { alive = false; };
  }, [params, refreshKey, toast]);

  const loadMore = async () => {
    if (!cursor || !params.q) return;
    setLoading(true);
    try {
      const res = await searchApi.search({ ...params, cursor, limit: 40 });
      setResults((prev) => [...prev, ...res.items]);
      setCursor(res.nextCursor);
      collectDevices(res.items);
    } catch (e) {
      toast(errorMessage(e), 'error');
    } finally {
      setLoading(false);
    }
  };

  const openResult = (result: SearchResult) => {
    if (result.kind === 'message' && result.message) {
      navigate(`/?msg=${encodeURIComponent(result.message.id)}`);
    } else if (result.file) {
      setPreview(result.file);
    }
  };

  const toggle = (key: ResultKey) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });

  const splitSelected = () => {
    const msgIds: string[] = [];
    const fileIds: string[] = [];
    const attachmentIds: string[] = [];
    for (const item of results) {
      if (!selected.has(keyOf(item))) continue;
      if (item.kind === 'message' && item.message) {
        msgIds.push(item.message.id);
        for (const a of item.message.attachments) attachmentIds.push(a.id);
      } else if (item.file) {
        fileIds.push(item.file.id);
      }
    }
    return { msgIds, fileIds, attachmentIds };
  };

  const bulk = async (action: BulkAction | FilesBulkAction, tagId?: string) => {
    const { msgIds, fileIds } = splitSelected();
    if (msgIds.length + fileIds.length === 0) return;
    if (action === 'delete') {
      const ok = await confirm({ title: t('messages.bulkDelete'), message: t('common.selected', { count: msgIds.length + fileIds.length }), danger: true, confirmLabel: t('common.delete') });
      if (!ok) return;
    }
    try {
      const jobs: Promise<unknown>[] = [];
      if (msgIds.length) jobs.push(messagesApi.bulk({ ids: msgIds, action: action as BulkAction, tagId }));
      // Pin/unpin are message-only; files have no pin action.
      if (fileIds.length && action !== 'pin' && action !== 'unpin') {
        jobs.push(filesApi.bulk({ ids: fileIds, action: action as FilesBulkAction, tagId }));
      }
      await Promise.all(jobs);
      setSelected(new Set());
      setSelectMode(false);
      setBulkTagOpen(false);
      toast(t('common.saved'), 'success');
      refresh();
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const downloadFiles = async (ids: string[]) => {
    if (ids.length === 0) {
      toast(t('files.noFiles'), 'info');
      return;
    }
    try {
      downloadBlob(ids.length === 1 ? await filesApi.download(ids[0]) : await filesApi.zip(ids, false), 'private-drop.zip');
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const downloadSelected = async () => {
    const { fileIds, attachmentIds } = splitSelected();
    await downloadFiles([...fileIds, ...attachmentIds].filter((id, i, arr) => arr.indexOf(id) === i));
  };

  const perResult = async (result: SearchResult, action: 'favorite' | 'unfavorite' | 'delete' | 'download' | 'tag', tagIds?: string[]) => {
    const message = result.message;
    const file = result.file;
    try {
      if (action === 'download') {
        if (file) await downloadFiles([file.id]);
        else if (message) await downloadFiles(message.attachments.map((a) => a.id));
        return;
      }
      if (action === 'delete') {
        const ok = await confirm({ title: t('messages.delete'), message: file?.name || message?.text || result.id, danger: true, confirmLabel: t('common.delete') });
        if (!ok) return;
      }
      if (message) {
        if (action === 'favorite' || action === 'unfavorite') {
          await messagesApi.update(message.id, { favorited: action === 'favorite' });
        } else if (action === 'tag') {
          await messagesApi.update(message.id, { tagIds });
        } else if (action === 'delete') {
          await messagesApi.remove(message.id);
        }
      } else if (file) {
        if (action === 'favorite' || action === 'unfavorite') {
          await filesApi.update(file.id, { favorited: action === 'favorite' });
        } else if (action === 'tag') {
          await filesApi.update(file.id, { tagIds });
        } else if (action === 'delete') {
          await filesApi.remove(file.id);
        }
      }
      refresh();
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const allKeys = results.map(keyOf);
  const allSelected = allKeys.length > 0 && selected.size === allKeys.length;

  return (
    <div className="mx-auto w-full max-w-4xl px-3 py-4 sm:px-5">
      <TextInput ref={inputRef} value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t('search.placeholder')} aria-label={t('search.placeholder')} className="!py-3 !text-base" />
      <div className="mt-3 flex flex-wrap items-center gap-2 rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-700 dark:bg-slate-900">
        <span className="text-xs font-medium text-slate-500">{t('search.filters')}</span>
        <select value={type} onChange={(e) => setType(e.target.value)} className={inputClass + ' w-auto !py-1.5 !text-xs'} aria-label={t('common.type')}>
          <option value="">{t('files.allTypes')}</option>
          <option value="message">{t('search.kindMessage')}</option>
          <option value="file">{t('search.kindFile')}</option>
          <option value="image">{t('files.image')}</option>
          <option value="video">{t('files.video')}</option>
          <option value="audio">{t('files.audio')}</option>
          <option value="pdf">{t('files.pdf')}</option>
        </select>
        <TextInput value={ext} onChange={(e) => setExt(e.target.value)} placeholder={t('search.ext')} className="w-24 !py-1.5 !text-xs" aria-label={t('search.ext')} />
        <TextInput type="number" min={0} value={minSizeMb} onChange={(e) => setMinSizeMb(e.target.value)} placeholder={t('search.minSize')} className="w-24 !py-1.5 !text-xs" aria-label={t('search.minSize')} />
        <TextInput type="number" min={0} value={maxSizeMb} onChange={(e) => setMaxSizeMb(e.target.value)} placeholder={t('search.maxSize')} className="w-24 !py-1.5 !text-xs" aria-label={t('search.maxSize')} />
        <select value={tag} onChange={(e) => setTag(e.target.value)} className={inputClass + ' w-auto !py-1.5 !text-xs'} aria-label={t('tags.title')}>
          <option value="">{t('tags.title')}</option>
          {tags.filter((x) => !x.isSystem).map((x) => <option key={x.id} value={x.id}>{x.icon ? `${x.name} · ${x.icon}` : x.name}</option>)}
        </select>
        <select value={device} onChange={(e) => setDevice(e.target.value)} className={inputClass + ' w-auto !py-1.5 !text-xs'} aria-label={t('search.device')}>
          <option value="">{t('search.device')}</option>
          {devices.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
        </select>
        <input type="date" value={from} onChange={(e) => setFrom(e.target.value)} aria-label={t('search.dateFrom')} className={inputClass + ' w-auto !py-1.5 !text-xs'} />
        <input type="date" value={to} onChange={(e) => setTo(e.target.value)} aria-label={t('search.dateTo')} className={inputClass + ' w-auto !py-1.5 !text-xs'} />
        <label className="flex items-center gap-1 text-xs text-slate-500"><input type="checkbox" checked={favorite} onChange={(e) => setFavorite(e.target.checked)} className="size-4 accent-blue-600" />{t('search.favoriteOnly')}</label>
        <label className="flex items-center gap-1 text-xs text-slate-500"><input type="checkbox" checked={pinned} onChange={(e) => setPinned(e.target.checked)} className="size-4 accent-blue-600" />{t('search.pinnedOnly')}</label>
      </div>

      {params.q && (
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <Button variant={selectMode ? 'primary' : 'secondary'} icon="check" onClick={() => { setSelectMode((v) => !v); setSelected(new Set()); }}>
            {selectMode ? t('messages.exitSelectMode') : t('messages.selectMode')}
          </Button>
          <span className="text-xs text-slate-400">{t('search.resultCount', { count: results.length })}</span>
        </div>
      )}

      {selectMode && results.length > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-1.5 rounded-xl border border-blue-200 bg-blue-50 p-2 dark:border-blue-900 dark:bg-blue-950/30">
          <span className="px-1 text-xs font-medium text-blue-700 dark:text-blue-300">{t('common.selected', { count: selected.size })}</span>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => setSelected(allSelected ? new Set() : new Set(allKeys))}>{allSelected ? t('common.clearSelection') : t('common.selectAll')}</Button>
          <div className="flex-1" />
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('favorite')} disabled={selected.size === 0}>{t('messages.bulkFavorite')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('unfavorite')} disabled={selected.size === 0}>{t('messages.bulkUnfavorite')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('pin')} disabled={selected.size === 0}>{t('messages.bulkPin')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('unpin')} disabled={selected.size === 0}>{t('messages.bulkUnpin')}</Button>
          <div className="relative">
            <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" icon="tag" onClick={() => setBulkTagOpen((v) => !v)} disabled={selected.size === 0}>{t('messages.bulkTag')}</Button>
            {bulkTagOpen && (
              <div className="absolute right-0 top-full z-20 mt-1 flex w-56 flex-col gap-1 rounded-xl border border-slate-200 bg-white p-2 shadow-xl dark:border-slate-700 dark:bg-slate-800">
                {tags.filter((x) => !x.isSystem).map((x) => (
                  <button key={x.id} type="button" onClick={() => void bulk('tag', x.id)} className="flex items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-slate-100 dark:hover:bg-slate-700">
                    <span className="size-2.5 rounded-full" style={{ background: x.color || '#64748b' }} />{x.name}
                  </button>
                ))}
                <TagPicker compact selectedIds={[]} onChange={(ids) => ids[0] && void bulk('tag', ids[0])} />
              </div>
            )}
          </div>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" onClick={() => void bulk('untag')} disabled={selected.size === 0}>{t('messages.bulkUntag')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs" icon="download" onClick={() => void downloadSelected()} disabled={selected.size === 0}>{t('files.downloadZip')}</Button>
          <Button variant="ghost" className="!min-h-7 !px-2 !text-xs hover:!text-red-600" icon="trash" onClick={() => void bulk('delete')} disabled={selected.size === 0}>{t('messages.bulkDelete')}</Button>
        </div>
      )}

      {!params.q ? (
        <EmptyState icon="search" title={t('search.startHint')} />
      ) : loading && results.length === 0 ? (
        <div className="flex justify-center py-12"><Spinner /></div>
      ) : results.length === 0 ? (
        <EmptyState icon="search" title={t('search.noResults')} />
      ) : (
        <>
          <ul className="mt-3 flex flex-col gap-2">
            {results.map((result) => {
              const message = result.message as Message | undefined;
              const file = result.file as Attachment | undefined;
              const key = keyOf(result);
              const isFav = Boolean(message?.favorited ?? file?.favorited);
              return (
                <li key={key} className="rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
                  <div className="flex items-start gap-3">
                    {selectMode && (
                      <input type="checkbox" checked={selected.has(key)} onChange={() => toggle(key)} aria-label={t('messages.selectMode')} className="mt-2 size-4 shrink-0 accent-blue-600" />
                    )}
                    <button type="button" onClick={() => openResult(result)} className="flex min-w-0 flex-1 items-start gap-3 text-left">
                      <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-slate-800">
                        <Icon name={result.kind === 'message' ? 'message' : 'file'} size={18} />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="mb-0.5 block text-[11px] font-medium uppercase text-blue-600 dark:text-blue-400">
                          {t(result.kind === 'message' ? 'search.kindMessage' : 'search.kindFile')}
                        </span>
                        <span className="block truncate text-sm text-slate-800 dark:text-slate-100">{message?.text || file?.name || result.id}</span>
                        {file && <span className="block text-xs text-slate-400">{formatBytes(file.size)} · {formatDateTime(file.uploadedAt, i18n.language)}</span>}
                        {message && <span className="block text-xs text-slate-400">{formatDateTime(message.createdAt, i18n.language)} · {message.device?.name}</span>}
                      </span>
                      <Icon name="chevronRight" size={16} className="mt-2 text-slate-300" />
                    </button>
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-0.5 border-t border-slate-100 pt-2 dark:border-slate-800">
                    <IconButton icon="download" label={t('common.download')} onClick={() => void perResult(result, 'download')} />
                    <IconButton icon="star" label={isFav ? t('messages.unfavorite') : t('messages.favorite')} className={isFav ? 'text-yellow-500' : ''} onClick={() => void perResult(result, isFav ? 'unfavorite' : 'favorite')} />
                    <TagPicker compact selectedIds={(message?.tags ?? file?.tags ?? []).map((x) => x.id)} onChange={(ids) => void perResult(result, 'tag', ids)} />
                    <div className="flex-1" />
                    <IconButton icon="trash" label={t('messages.delete')} className="hover:text-red-600" onClick={() => void perResult(result, 'delete')} />
                  </div>
                </li>
              );
            })}
          </ul>
          {cursor && <div className="flex justify-center py-5"><Button onClick={() => void loadMore()} disabled={loading}>{t('messages.loadMore')}</Button></div>}
        </>
      )}
      <FilePreview file={preview} onClose={() => setPreview(null)} />
    </div>
  );
}
