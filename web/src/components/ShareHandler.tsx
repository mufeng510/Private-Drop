import { useEffect } from 'react';
import { fileStore, shareStore } from '../lib/idb';

interface ShareDetail {
  text?: string;
  files?: File[];
}
interface SWShare {
  type?: string;
  nonce?: string;
  text?: string;
  files?: File[];
  fileIds?: string[];
}

/** Dedupe the SW postMessage path and the cold-start IndexedDB path. */
const seenNonces = new Set<string>();

/**
 * Web Share Target handling.
 *
 * The manifest declares a POST multipart/form-data share target. The service
 * worker (src/sw.ts) reads that request, stores shared Files in IndexedDB and
 * either posts `pd:share` to live clients or redirects to `/?shared=1` for a
 * cold start. This component bridges both paths into the `pd:share` DOM event
 * that the composer already consumes (text is prefilled, files are handed to
 * the upload manager).
 *
 * Browser limitations (documented per taskbook §21):
 * - iOS Safari has no Web Share Target API. Users there can copy text/images
 *   and paste into the composer (Ctrl/Cmd+V) or use the in-app file picker.
 * - Background upload after the tab closes is not guaranteed; keep the tab
 *   open until uploads finish.
 */
export function ShareHandler() {
  useEffect(() => {
    let cancelled = false;

    const cleanUrl = () => {
      const params = new URLSearchParams(window.location.search);
      if (!params.has('shared') && !params.has('share-target') && !params.has('title') && !params.has('text') && !params.has('url')) return;
      params.delete('shared');
      params.delete('share-target');
      params.delete('title');
      params.delete('text');
      params.delete('url');
      const search = params.toString();
      window.history.replaceState(null, '', `${window.location.pathname}${search ? `?${search}` : ''}${window.location.hash}`);
    };

    const dispatchShare = (detail: ShareDetail, nonce?: string) => {
      if (nonce) {
        if (seenNonces.has(nonce)) return;
        seenNonces.add(nonce);
      }
      if (!detail.text && !detail.files?.length) return;
      window.dispatchEvent(new CustomEvent<ShareDetail>('pd:share', { detail }));
      cleanUrl();
    };

    const filesFromIds = async (ids: string[]): Promise<File[]> => {
      const result = await Promise.all(ids.map((id) => fileStore.get(id).catch(() => undefined)));
      return result.filter((file): file is File => Boolean(file));
    };

    const cleanup = async (nonce: string, extraIds: string[] = []) => {
      try {
        const pending = await shareStore.get();
        if (pending?.nonce === nonce) {
          for (const id of pending.fileIds) await fileStore.remove(id).catch(() => undefined);
          await shareStore.remove().catch(() => undefined);
        }
        for (const id of extraIds) await fileStore.remove(id).catch(() => undefined);
      } catch {
        /* IndexedDB unavailable */
      }
    };

    const onMessage = (event: MessageEvent) => {
      const data = event.data as SWShare | undefined;
      if (!data || data.type !== 'pd:share') return;
      void (async () => {
        const files = data.files?.length ? data.files : await filesFromIds(data.fileIds ?? []);
        dispatchShare({ text: data.text, files }, data.nonce);
        if (data.nonce) await cleanup(data.nonce, files.map((_, i) => `share-${data.nonce}-${i}`));
      })();
    };

    // Fallback for browsers that still deliver a text-only share via GET query
    // parameters (kept for compatibility with the previous manifest).
    const params = new URLSearchParams(window.location.search);
    const title = params.get('title') || '';
    const text = params.get('text') || '';
    const url = params.get('url') || '';
    const combined = [text, url].filter(Boolean).join('\n');
    if (title || combined) {
      dispatchShare({ text: title ? `${title}\n${combined}` : combined });
    }

    // Cold start: the SW redirected here after storing the share in IDB.
    void (async () => {
      try {
        const pending = await shareStore.get();
        if (!pending || cancelled) return;
        const files = await filesFromIds(pending.fileIds);
        dispatchShare({ text: pending.text, files }, pending.nonce);
        await cleanup(pending.nonce);
      } catch {
        /* IndexedDB unavailable */
      }
    })();

    navigator.serviceWorker?.addEventListener('message', onMessage);
    return () => {
      cancelled = true;
      navigator.serviceWorker?.removeEventListener('message', onMessage);
    };
  }, []);
  return null;
}
