import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useUiStore } from '../store/ui';
import { Icon } from './ui/Icon';
import { IconButton } from './ui/primitives';
import { downloadBlob } from '../lib/blob';

export function Lightbox() {
  const { t } = useTranslation();
  const lightbox = useUiStore((s) => s.lightbox);
  const close = useUiStore((s) => s.closeLightbox);
  const step = useUiStore((s) => s.lightboxStep);
  const ref = useRef<HTMLDialogElement>(null);
  const [zoom, setZoom] = useState(1);
  const [rotation, setRotation] = useState(0);

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (lightbox && !dialog.open) dialog.showModal();
    if (!lightbox && dialog.open) dialog.close();
  }, [lightbox]);

  useEffect(() => {
    setZoom(1);
    setRotation(0);
  }, [lightbox?.index]);

  const current = lightbox?.images[lightbox.index];

  const copy = async () => {
    if (!current) return;
    try {
      const blob = await fetch(current.url).then((r) => r.blob());
      await navigator.clipboard.write([new ClipboardItem({ [blob.type || 'image/png']: blob })]);
      useUiStore.getState().toast(t('common.copied'), 'success');
    } catch {
      useUiStore.getState().toast(t('common.error'), 'error');
    }
  };

  const download = async () => {
    if (!current) return;
    const blob = await fetch(current.url).then((r) => r.blob());
    downloadBlob(blob, current.name || 'image');
  };

  return (
    <dialog
      ref={ref}
      onCancel={close}
      aria-label={t('nav.messages')}
      className="size-full max-w-none max-h-none border-0 bg-black/95 p-0 text-white"
      {...({ closedby: 'any' } as Record<string, string>)}
    >
      {current && (
        <div className="relative flex size-full flex-col">
          <header className="flex items-center justify-between gap-3 px-4 py-3">
            <span className="truncate text-sm text-white/80">{current.name}</span>
            <div className="flex items-center gap-0.5 text-white">
              <IconButton icon="zoomOut" label={t('lightbox.zoomOut')} onClick={() => setZoom((z) => Math.max(0.25, +(z - 0.25).toFixed(2)))} className="hover:bg-white/10 hover:text-white" />
              <span className="w-14 text-center text-xs tabular-nums">{t('lightbox.zoomLevel', { percent: Math.round(zoom * 100) })}</span>
              <IconButton icon="zoomIn" label={t('lightbox.zoomIn')} onClick={() => setZoom((z) => Math.min(5, +(z + 0.25).toFixed(2)))} className="hover:bg-white/10 hover:text-white" />
              <IconButton icon="rotate" label={t('lightbox.rotate')} onClick={() => setRotation((r) => (r + 90) % 360)} className="hover:bg-white/10 hover:text-white" />
              <IconButton icon="copy" label={t('lightbox.copy')} onClick={() => void copy()} className="hover:bg-white/10 hover:text-white" />
              <IconButton icon="download" label={t('lightbox.download')} onClick={() => void download()} className="hover:bg-white/10 hover:text-white" />
              <IconButton icon="x" label={t('lightbox.close')} onClick={close} className="hover:bg-white/10 hover:text-white" />
            </div>
          </header>

          <div className="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden" onClick={(e) => { if (e.target === e.currentTarget) close(); }}>
            <img
              src={current.url}
              alt={current.name || ''}
              className="max-h-full max-w-full object-contain transition-transform duration-150"
              style={{ transform: `scale(${zoom}) rotate(${rotation}deg)` }}
            />
            {lightbox && lightbox.images.length > 1 && (
              <>
                <button type="button" onClick={() => step(-1)} aria-label={t('lightbox.prev')} className="absolute left-3 rounded-full bg-black/50 p-3 hover:bg-black/70"><Icon name="chevronLeft" size={24} /></button>
                <button type="button" onClick={() => step(1)} aria-label={t('lightbox.next')} className="absolute right-3 rounded-full bg-black/50 p-3 hover:bg-black/70"><Icon name="chevronRight" size={24} /></button>
              </>
            )}
          </div>
          <footer className="py-2 text-center text-xs text-white/50">
            {lightbox ? `${lightbox.index + 1} / ${lightbox.images.length}` : ''}
          </footer>
        </div>
      )}
    </dialog>
  );
}
