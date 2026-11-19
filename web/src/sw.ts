import { precacheAndRoute } from 'workbox-precaching';
import { fileStore, shareStore, type PendingShare } from './lib/idb';

// Custom service worker (vite-plugin-pwa injectManifest strategy).
// It precaches build assets and implements the Web Share Target POST hand-off:
//   OS -> POST /?share-target=1 (multipart/form-data) -> SW stores files in
//   IndexedDB -> redirect to /?shared=1 -> SPA reads the pending share.
//
// iOS limitation (taskbook §21): Safari has no Web Share Target API, so files
// cannot be shared into the PWA there. iOS users paste or use the file picker.
// Background upload after the tab is closed is likewise not guaranteed.

interface ManifestEntry {
  url: string;
  revision: string | null;
}
interface SWFetchEvent {
  request: Request;
  respondWith(response: Promise<Response> | Response): void;
}
interface SWClient {
  postMessage(data: unknown): void;
}
interface SWGlobal {
  location: { origin: string };
  clients: { matchAll(options?: { type?: string }): Promise<SWClient[]> };
  addEventListener(type: 'fetch', listener: (event: SWFetchEvent) => void): void;
  addEventListener(type: 'message', listener: (event: { data?: unknown }) => void): void;
  __WB_MANIFEST?: ManifestEntry[];
}

// Typed view of the service-worker globals without pulling in the WebWorker
// lib (which conflicts with the DOM lib used by the rest of the SPA).
declare const self: SWGlobal & { __WB_MANIFEST?: ManifestEntry[] };
const sw = self;

// workbox-build replaces the literal `self.__WB_MANIFEST` with the generated
// precache list, so the identifier must appear verbatim in the emitted file.
precacheAndRoute(self.__WB_MANIFEST ?? []);

sw.addEventListener('message', (event) => {
  const data = event.data as { type?: string } | undefined;
  if (data?.type === 'SKIP_WAITING') {
    void (globalThis as unknown as { skipWaiting?: () => Promise<void> }).skipWaiting?.();
  }
});

sw.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);
  // A Web Share Target POST is a cross-navigation multipart request. Only the
  // SW can read its body, so intercept it and hand the data to the SPA via IDB.
  if (event.request.method === 'POST' && url.origin === sw.location.origin && url.searchParams.has('share-target')) {
    event.respondWith(handleShare(event.request));
  }
});

async function handleShare(request: Request): Promise<Response> {
  try {
    const form = await request.formData();
    const title = String(form.get('title') ?? '');
    const bodyText = String(form.get('text') ?? '');
    const sharedUrl = String(form.get('url') ?? '');
    const text = [title, bodyText, sharedUrl].filter(Boolean).join('\n');
    const files = form.getAll('files').filter((entry): entry is File => entry instanceof File);
    const nonce = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`;

    const fileIds: string[] = [];
    for (const [index, file] of files.entries()) {
      const id = `share-${nonce}-${index}`;
      await fileStore.put(id, file);
      fileIds.push(id);
    }

    const pending: PendingShare = { nonce, text, fileIds, ts: Date.now() };
    await shareStore.put(pending);

    // Warm clients: post an immediate message with the reconstructed Files.
    const restored: File[] = [];
    for (const id of fileIds) {
      const file = await fileStore.get(id).catch(() => undefined);
      if (file) restored.push(file);
    }
    const clients = await sw.clients.matchAll({ type: 'window' });
    for (const client of clients) {
      client.postMessage({ type: 'pd:share', ...pending, files: restored });
    }
  } catch {
    // Parsing may fail on odd payloads; still navigate into the app so the
    // user is never left on a blank share-target page.
  }
  return Response.redirect(new URL('/?shared=1', sw.location.origin), 303);
}
