import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Attachment } from '../api/types';
import { authHeaders } from '../api/session';
import { useAuthImage } from '../hooks/useAuthImage';
import { downloadBlob } from '../lib/blob';
import { Modal } from './ui/Modal';
import { Button } from './ui/primitives';
import { Icon } from './ui/Icon';

const BASE = (import.meta.env?.VITE_API_BASE as string | undefined)?.replace(/\/$/, '') || '';

export function FilePreview({ file, onClose }: { file: Attachment | null; onClose: () => void }) {
  const { t } = useTranslation();
  const [text, setText] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const type = file?.type;
  const { url } = useAuthImage(file && type !== 'text' && type !== 'code' ? `/api/files/${file.id}/preview` : undefined);
  const { url: thumb } = useAuthImage(file?.thumbnailUrl);

  useEffect(() => {
    if (!file || (type !== 'text' && type !== 'code')) return;
    let alive = true;
    setLoading(true);
    fetch(`${BASE}/api/files/${file.id}/preview`, { headers: authHeaders(), credentials: 'same-origin' })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(String(r.status)))))
      .then((body) => alive && setText(body))
      .catch(() => alive && setText(t('files.previewUnavailable')))
      .finally(() => alive && setLoading(false));
    return () => {
      alive = false;
    };
  }, [file, type, t]);

  const download = async () => {
    if (!file) return;
    const res = await fetch(`${BASE}/api/files/${file.id}/download`, { headers: authHeaders(), credentials: 'same-origin' });
    if (res.ok) downloadBlob(await res.blob(), file.name);
  };

  const isText = type === 'text' || type === 'code';
  const canShow = isText || Boolean(url) || Boolean(thumb);

  return (
    <Modal
      open={Boolean(file)}
      onClose={onClose}
      title={file?.name ?? ''}
      size="full"
      footer={<Button onClick={() => void download()} icon="download">{t('common.download')}</Button>}
    >
      {!file ? null : isText ? (
        loading ? <p className="text-sm text-slate-400">{t('common.loading')}</p> : (
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg bg-slate-50 p-4 text-xs leading-relaxed dark:bg-slate-950">{text}</pre>
        )
      ) : type === 'image' ? (
        <div className="flex min-h-[50vh] items-center justify-center">
          {(thumb || url) ? <img src={thumb || url} alt={file.name} className="max-h-[70vh] max-w-full object-contain" /> : null}
        </div>
      ) : type === 'video' ? (
        <video src={url} controls className="max-h-[70vh] w-full rounded-lg bg-black" />
      ) : type === 'audio' ? (
        <div className="flex flex-col items-center gap-4 py-12">
          <Icon name="audio" size={48} className="text-slate-400" />
          <audio src={url} controls className="w-full max-w-lg" />
        </div>
      ) : type === 'pdf' ? (
        <iframe src={url} title={file.name} className="h-[70vh] w-full rounded-lg border-0" />
      ) : (
        <div className="flex flex-col items-center gap-3 py-16 text-slate-500">
          <Icon name="file" size={44} />
          <p className="text-sm">{t('files.previewUnavailable')}</p>
        </div>
      )}
      {!canShow && !isText && <p className="text-sm text-slate-400">{t('common.loading')}</p>}
    </Modal>
  );
}
