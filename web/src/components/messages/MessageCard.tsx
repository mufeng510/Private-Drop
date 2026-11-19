import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Attachment, Message } from '../../api/types';
import { messagesApi } from '../../api/messages';
import { errorMessage } from '../../api/client';
import { useMessagesStore } from '../../store/messages';
import { useUiStore } from '../../store/ui';
import { formatDateTime, formatRelativeTime, retentionInfo } from '../../lib/format';
import { AttachmentGrid } from './AttachmentGrid';
import { TagPicker } from '../ui/TagPicker';
import { TagChip } from '../ui/TagChip';
import { IconButton, Button, inputClass } from '../ui/primitives';
import { Icon, type IconName } from '../ui/Icon';

const typeIcon: Record<string, IconName> = {
  image: 'image', video: 'video', audio: 'audio', pdf: 'document', office: 'document', text: 'document', code: 'code', other: 'file',
};

export function MessageCard({
  message,
  selected,
  selectMode,
  onToggleSelect,
  onPreview,
  onChanged,
}: {
  message: Message;
  selected: boolean;
  selectMode: boolean;
  onToggleSelect: () => void;
  onPreview: (file: Attachment) => void;
  onChanged: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(message.text);
  const [busy, setBusy] = useState(false);
  const patch = useMessagesStore((s) => s.patch);
  const remove = useMessagesStore((s) => s.remove);
  const confirm = useUiStore((s) => s.requestConfirm);
  const toast = useUiStore((s) => s.toast);
  const retention = retentionInfo(message.expiresAt);
  const hasAttachments = message.attachments.length > 0;
  const kinds = new Set(message.attachments.map((a) => a.type));
  const typeLabel = hasAttachments
    ? kinds.size > 1 ? t('messages.mixedType') : t(`messages.${[...kinds][0]}Type`)
    : t('messages.textType');

  const mutate = async (action: () => Promise<unknown>, changes?: Partial<Message>) => {
    setBusy(true);
    try {
      await action();
      if (changes) patch(message.id, changes);
      onChanged();
    } catch (e) {
      toast(errorMessage(e), 'error');
    } finally {
      setBusy(false);
    }
  };

  const saveEdit = () => {
    if (draft.trim() === message.text) {
      setEditing(false);
      return;
    }
    void mutate(() => messagesApi.update(message.id, { text: draft }), { text: draft, updatedAt: new Date().toISOString() }).then(() => setEditing(false));
  };

  const doDelete = async () => {
    const ok = await confirm({ title: t('messages.delete'), message: t('recycle.title'), danger: true, confirmLabel: t('common.delete') });
    if (!ok) return;
    await mutate(async () => {
      await messagesApi.remove(message.id);
      remove(message.id);
    });
  };

  return (
    <article
      className={`group rounded-xl border bg-white p-3.5 shadow-sm transition-colors dark:bg-slate-900 sm:p-4 ${
        selected
          ? 'border-blue-400 ring-2 ring-blue-500/20 dark:border-blue-500'
          : 'border-slate-200 dark:border-slate-800'
      }`}
    >
      <div className="flex items-start gap-2.5">
        {selectMode && (
          <input
            type="checkbox"
            checked={selected}
            onChange={onToggleSelect}
            aria-label={t('messages.selectMode')}
            className="mt-1 size-4 shrink-0 accent-blue-600"
          />
        )}
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-slate-400">
            <span className="inline-flex items-center gap-1 font-medium text-slate-600 dark:text-slate-300">
              <Icon name="devices" size={13} /> {t('messages.fromDevice', { name: message.device?.name || t('common.unknown') })}
            </span>
            <span title={formatDateTime(message.createdAt, i18n.language)}>{formatRelativeTime(message.createdAt, i18n.language)}</span>
            <span>·</span>
            <span className="inline-flex items-center gap-1"><Icon name={typeIcon[message.attachments[0]?.type] || 'message'} size={12} /> {typeLabel}</span>
            {message.pinned && <span className="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">{t('messages.pinned')}</span>}
            {message.updatedAt !== message.createdAt && <span className="text-[10px]">({t('messages.edited')})</span>}
            <span className={`inline-flex items-center gap-1 ${retention.expired ? 'text-red-500' : retention.days <= 3 ? 'text-amber-600' : ''}`}>
              <Icon name="clock" size={12} />
              {message.expiresAt ? t('messages.expiresIn', { time: retention.expired ? t('messages.expired') : formatRelativeTime(message.expiresAt, i18n.language) }) : t('messages.noExpiry')}
            </span>
          </div>

          {editing ? (
            <div className="mt-2 flex flex-col gap-2">
              <textarea value={draft} onChange={(e) => setDraft(e.target.value)} rows={3} className={inputClass} autoFocus />
              <div className="flex gap-2">
                <Button variant="primary" onClick={saveEdit} disabled={busy}>{t('messages.saveEdit')}</Button>
                <Button variant="ghost" onClick={() => { setDraft(message.text); setEditing(false); }}>{t('messages.cancelEdit')}</Button>
              </div>
            </div>
          ) : (
            <>
              {message.text && <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-relaxed text-slate-800 dark:text-slate-100">{message.text}</p>}
              <AttachmentGrid attachments={message.attachments} onPreview={onPreview} />
              {message.links?.length > 0 && (
                <div className="mt-2 flex flex-col gap-1">
                  {message.links.map((link, i) => (
                    <a key={`${link.url}-${i}`} href={link.url} target="_blank" rel="noopener noreferrer" className="inline-flex max-w-full items-center gap-1.5 rounded-lg bg-slate-50 px-2.5 py-1.5 text-xs text-blue-600 hover:underline dark:bg-slate-800 dark:text-blue-400">
                      <Icon name="link" size={13} /> <span className="truncate">{link.title || link.url}</span>
                    </a>
                  ))}
                </div>
              )}
              {message.tags?.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {message.tags.map((tag) => <TagChip key={tag.id} tag={tag} />)}
                </div>
              )}
            </>
          )}
        </div>
      </div>

      {!editing && (
        <div className="mt-3 flex flex-wrap items-center gap-0.5 border-t border-slate-100 pt-2 dark:border-slate-800">
          <IconButton icon={message.pinned ? 'pin' : 'pin'} label={message.pinned ? t('messages.unpin') : t('messages.pin')} className={message.pinned ? 'text-amber-600' : ''} disabled={busy} onClick={() => void mutate(() => messagesApi.update(message.id, { pinned: !message.pinned }), { pinned: !message.pinned })} />
          <IconButton icon="star" label={message.favorited ? t('messages.unfavorite') : t('messages.favorite')} className={message.favorited ? 'text-yellow-500' : ''} disabled={busy} onClick={() => void mutate(() => messagesApi.update(message.id, { favorited: !message.favorited }), { favorited: !message.favorited })} />
          {message.text && <IconButton icon="edit" label={t('common.edit')} disabled={busy} onClick={() => { setDraft(message.text); setEditing(true); }} />}
          {message.text && <IconButton icon="copy" label={t('messages.copyText')} onClick={() => { void navigator.clipboard.writeText(message.text).then(() => toast(t('common.copied'), 'success')).catch(() => undefined); }} />}
          <TagPicker compact selectedIds={message.tags?.map((x) => x.id) ?? []} onChange={(ids) => void mutate(() => messagesApi.update(message.id, { tagIds: ids }))} />
          <div className="flex-1" />
          <IconButton icon="trash" label={t('messages.delete')} className="hover:text-red-600" disabled={busy} onClick={() => void doDelete()} />
        </div>
      )}
    </article>
  );
}
