import { useTranslation } from 'react-i18next';
import type { Attachment, MessageAttachmentRef } from '../../api/types';
import { getAuthBlobUrl, downloadBlob } from '../../lib/blob';
import { authHeaders } from '../../api/session';
import { useUiStore } from '../../store/ui';
import { formatBytes } from '../../lib/format';
import { Icon, type IconName } from '../ui/Icon';
import { AuthImage } from '../ui/AuthImage';

const iconFor: Record<string, IconName> = {
  image: 'image', video: 'video', audio: 'audio', pdf: 'document', office: 'document', text: 'document', code: 'code', other: 'file',
};

export function AttachmentGrid({
  attachments,
  onPreview,
}: {
  attachments: MessageAttachmentRef[];
  onPreview: (file: Attachment) => void;
}) {
  const { t } = useTranslation();
  const openLightbox = useUiStore((s) => s.openLightbox);
  const toast = useUiStore((s) => s.toast);
  if (attachments.length === 0) return null;

  const images = attachments.filter((a) => a.type === 'image');

  const openImage = async (attachment: MessageAttachmentRef) => {
    try {
      const urls = await Promise.all(
        images.map((a) => getAuthBlobUrl(a.thumbnailUrl || `/api/files/${a.id}/preview`)),
      );
      const index = images.findIndex((a) => a.id === attachment.id);
      openLightbox(urls.map((url, i) => ({ url, name: images[i].name })), Math.max(0, index));
    } catch {
      toast(t('common.error'), 'error');
    }
  };

  const quickDownload = async (attachment: MessageAttachmentRef) => {
    try {
      const res = await fetch(`/api/files/${attachment.id}/download`, {
        headers: authHeaders(),
        credentials: 'same-origin',
      });
      if (!res.ok) throw new Error(String(res.status));
      downloadBlob(await res.blob(), attachment.name);
    } catch {
      toast(t('common.error'), 'error');
    }
  };

  const asAttachment = (a: MessageAttachmentRef): Attachment => ({
    ...a,
    sha256: '',
    favorited: Boolean(a.favorited),
    uploadedAt: '',
    messageIds: [],
  });

  return (
    <div className={`mt-2 grid gap-2 ${images.length > 1 ? 'grid-cols-2 sm:grid-cols-3' : 'grid-cols-1'}`}>
      {attachments.map((a) => {
        if (a.type === 'image') {
          return (
            <button
              key={a.id}
              type="button"
              onClick={() => void openImage(a)}
              className="group relative aspect-square overflow-hidden rounded-lg bg-slate-100 dark:bg-slate-800"
              aria-label={a.name}
            >
              <AuthImage
                path={a.thumbnailUrl || `/api/files/${a.id}/thumbnail`}
                alt={a.name}
                className="size-full object-cover transition-transform group-hover:scale-105"
              />
            </button>
          );
        }
        return (
          <div key={a.id} className="flex min-w-0 items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 p-2 dark:border-slate-700 dark:bg-slate-800/50">
            <button type="button" onClick={() => onPreview(asAttachment(a))} className="flex min-w-0 flex-1 items-center gap-2 text-left">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-white text-blue-600 dark:bg-slate-700"><Icon name={iconFor[a.type] || 'file'} size={18} /></span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{a.name}</span>
                <span className="block text-xs text-slate-400">{formatBytes(a.size)}</span>
              </span>
            </button>
            <button type="button" onClick={() => void quickDownload(a)} aria-label={t('common.download')} className="rounded-lg p-1.5 text-slate-400 hover:bg-white hover:text-blue-600 dark:hover:bg-slate-700">
              <Icon name="download" size={16} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
