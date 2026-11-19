import { create } from 'zustand';
import { useUiStore } from './ui';
import { UploadManager, defaultUploadIO } from '../upload/manager';
import type { UploadTask } from '../upload/types';

interface UploadsState {
  tasks: UploadTask[];
  manager: UploadManager | null;
  init: (concurrency?: number) => Promise<void>;
  setConcurrency: (concurrency: number) => void;
  addFiles: (files: File[]) => UploadTask[];
  getTask: (id: string) => UploadTask | undefined;
  cancel: (id: string) => void;
  retry: (id: string) => void;
  reattach: (id: string, file: File) => void;
  removeTask: (id: string) => void;
  clearFinished: () => void;
  /** Reflect progress reported by another device for the same server task. */
  remoteUpdate: (uploadId: string, uploadedChunks: number[], status?: string) => void;
}

export const useUploadsStore = create<UploadsState>()((set, get) => ({
  tasks: [],
  manager: null,

  init: async (concurrency) => {
    if (get().manager) return;
    const manager = new UploadManager(defaultUploadIO(), {
      onUpdate: (task) => {
        const tasks = manager.getTasks().map((t) => (t.id === task.id ? task : t));
        set({ tasks });
      },
      onAttachment: () => {
        useUiStore.getState().toast('upload.done', 'success');
        useUiStore.getState().bumpData();
      },
      onError: (message) => useUiStore.getState().toast(message || 'upload.failed', 'error'),
    }, { concurrency });
    set({ manager, tasks: manager.getTasks() });
    await manager.resumeAll();
    set({ tasks: manager.getTasks() });
  },

  setConcurrency: (concurrency) => {
    const manager = get().manager;
    if (manager) manager.setConcurrency(Number(concurrency) || 1);
  },

  addFiles: (files) => {
    const manager = get().manager;
    if (!manager || files.length === 0) return [];
    const created = manager.addFiles(files);
    set({ tasks: manager.getTasks() });
    return created;
  },
  getTask: (id) => get().tasks.find((t) => t.id === id),
  cancel: (id) => {
    get().manager?.cancel(id);
    set({ tasks: get().manager?.getTasks() ?? [] });
  },
  retry: (id) => {
    get().manager?.retry(id);
    set({ tasks: get().manager?.getTasks() ?? [] });
  },
  reattach: (id, file) => {
    get().manager?.reattach(id, file);
    set({ tasks: get().manager?.getTasks() ?? [] });
  },
  removeTask: (id) => {
    get().manager?.removeTask(id);
    set({ tasks: get().manager?.getTasks() ?? [] });
  },
  clearFinished: () => {
    get().manager?.clearFinished();
    set({ tasks: get().manager?.getTasks() ?? [] });
  },
  remoteUpdate: (uploadId, uploadedChunks, status) =>
    set((s) => ({
      tasks: s.tasks.map((t) =>
        t.uploadId === uploadId
          ? {
              ...t,
              uploadedChunks,
              bytesUploaded: uploadedChunks.length * (t.chunkSize || 1),
              status: (status as UploadTask['status']) ?? t.status,
            }
          : t,
      ),
    })),
}));
