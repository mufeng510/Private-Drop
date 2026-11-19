// Persist original File objects for resumable uploads across page reloads.
// IndexedDB supports structured-clone of File/Blob; localStorage does not.
// If the entry is missing (cleared site data, different device) the upload
// manager falls back to asking the user to re-select the original file.

const DB_NAME = 'pd-uploads';
const STORE = 'files';
const SHARE_STORE = 'share';
const VERSION = 2;

export interface PendingShare {
  /** Stable nonce used to dedupe the SW postMessage and the cold-start read. */
  nonce: string;
  text: string;
  fileIds: string[];
  ts: number;
}

function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    if (typeof indexedDB === 'undefined') {
      reject(new Error('IndexedDB unavailable'));
      return;
    }
    const req = indexedDB.open(DB_NAME, VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) db.createObjectStore(STORE);
      if (!db.objectStoreNames.contains(SHARE_STORE)) db.createObjectStore(SHARE_STORE);
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error ?? new Error('IndexedDB open failed'));
  });
}

function tx<T>(storeName: string, mode: IDBTransactionMode, fn: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return openDb().then(
    (db) =>
      new Promise<T>((resolve, reject) => {
        const transaction = db.transaction(storeName, mode);
        const req = fn(transaction.objectStore(storeName));
        req.onsuccess = () => resolve(req.result);
        req.onerror = () => reject(req.error ?? new Error('IndexedDB request failed'));
        transaction.oncomplete = () => db.close();
        transaction.onerror = () => {
          db.close();
          reject(transaction.error ?? new Error('IndexedDB transaction failed'));
        };
      }),
  );
}

export const fileStore = {
  put: (id: string, file: File): Promise<IDBValidKey> => tx(STORE, 'readwrite', (s) => s.put(file, id)),
  get: (id: string): Promise<File | undefined> => tx(STORE, 'readonly', (s) => s.get(id)) as Promise<File | undefined>,
  remove: (id: string): Promise<undefined> => tx(STORE, 'readwrite', (s) => s.delete(id)),
  clear: (): Promise<undefined> => tx(STORE, 'readwrite', (s) => s.clear()),
};

/**
 * Hand-off slot for Web Share Target POSTs handled by the service worker.
 * The SW writes the shared files + text here, then the SPA reads it on load.
 */
export const shareStore = {
  put: (share: PendingShare): Promise<IDBValidKey> => tx(SHARE_STORE, 'readwrite', (s) => s.put(share, 'pending')),
  get: (): Promise<PendingShare | undefined> => tx(SHARE_STORE, 'readonly', (s) => s.get('pending')) as Promise<PendingShare | undefined>,
  remove: (): Promise<undefined> => tx(SHARE_STORE, 'readwrite', (s) => s.delete('pending')),
};

export const idbAvailable = typeof indexedDB !== 'undefined';
