import { uploadsApi } from '../api/uploads';
import type { InitUploadRequest } from '../api/types';
import { fileStore } from '../lib/idb';
import { calculateSpeed, chunkCount } from '../lib/format';
import { hashFile } from './hash';
import { runWithConcurrency, splitChunks } from './scheduler';
import type { UploadHooks, UploadIO, UploadTask, UploadStatus } from './types';

export const DEFAULT_CHUNK_SIZE = 4 * 1024 * 1024;
const META_KEY = 'pd.uploads.meta';

interface TaskMeta {
  id: string;
  name: string;
  size: number;
  mime: string;
  chunkSize: number;
  createdAt: number;
  uploadId?: string;
  sha256?: string;
  status: UploadStatus;
}

function newId(): string {
  return globalThis.crypto?.randomUUID?.() ?? `u-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export function defaultUploadIO(): UploadIO {
  return {
    init: (req) => uploadsApi.init(req),
    putChunk: (id, index, chunk, signal) => uploadsApi.putChunk(id, index, chunk, signal),
    complete: (id) => uploadsApi.complete(id),
    status: (id) => uploadsApi.status(id),
    cancel: (id) => uploadsApi.cancel(id),
    retry: (id) => uploadsApi.retry(id),
    hash: (file) => hashFile(file),
    saveFile: (id, file) => fileStore.put(id, file),
    loadFile: (id) => fileStore.get(id),
    removeFile: (id) => fileStore.remove(id),
  };
}

/**
 * Owns the upload lifecycle: hashing, init/dedup, chunked transfer with a
 * global concurrency limit, per-task progress/speed, cancel/retry and
 * resume-after-reload by reconciling server chunk state with the local File.
 */
export class UploadManager {
  private tasks = new Map<string, UploadTask>();
  private files = new Map<string, File>();
  private controllers = new Map<string, AbortController>();
  private concurrency: number;
  private readonly chunkSize: number;

  constructor(
    private readonly io: UploadIO,
    private readonly hooks: UploadHooks,
    options: { concurrency?: number; chunkSize?: number } = {},
  ) {
    this.concurrency = Math.max(1, options.concurrency ?? 3);
    this.chunkSize = options.chunkSize ?? DEFAULT_CHUNK_SIZE;
  }

  setConcurrency(value: number): void {
    this.concurrency = Math.max(1, Math.floor(value) || 1);
  }

  getTasks(): UploadTask[] {
    return [...this.tasks.values()].sort((a, b) => b.createdAt - a.createdAt);
  }

  private emit(task: UploadTask): void {
    this.persistMeta();
    this.hooks.onUpdate({ ...task, uploadedChunks: [...task.uploadedChunks] });
  }

  private persistMeta(): void {
    try {
      const metas: TaskMeta[] = [...this.tasks.values()].map((t) => ({
        id: t.id,
        name: t.name,
        size: t.size,
        mime: t.mime,
        chunkSize: t.chunkSize,
        createdAt: t.createdAt,
        uploadId: t.uploadId,
        sha256: t.sha256,
        status: t.status,
      }));
      globalThis.localStorage?.setItem(META_KEY, JSON.stringify(metas));
    } catch {
      /* storage unavailable */
    }
  }

  private loadMeta(): TaskMeta[] {
    try {
      const raw = globalThis.localStorage?.getItem(META_KEY);
      return raw ? (JSON.parse(raw) as TaskMeta[]) : [];
    } catch {
      return [];
    }
  }

  private makeTask(file: File): UploadTask {
    return {
      id: newId(),
      name: file.name,
      size: file.size,
      mime: file.type || 'application/octet-stream',
      chunkSize: this.chunkSize,
      totalChunks: chunkCount(file.size, this.chunkSize),
      uploadedChunks: [],
      status: 'initializing',
      bytesUploaded: 0,
      speed: 0,
      createdAt: Date.now(),
    };
  }

  addFiles(files: File[]): UploadTask[] {
    const created: UploadTask[] = [];
    for (const file of files) {
      const task = this.makeTask(file);
      this.tasks.set(task.id, task);
      this.files.set(task.id, file);
      created.push(task);
      this.emit(task);
      void this.io.saveFile(task.id, file).catch(() => undefined);
      void this.runTask(task, file, false);
    }
    return created;
  }

  /** Reconcile persisted tasks with server-side chunk state after a reload. */
  async resumeAll(): Promise<void> {
    const metas = this.loadMeta();
    for (const meta of metas) {
      if (meta.status === 'done' || meta.status === 'cancelled') {
        const task: UploadTask = {
          ...meta,
          totalChunks: chunkCount(meta.size, meta.chunkSize),
          uploadedChunks: [],
          bytesUploaded: 0,
          speed: 0,
        };
        this.tasks.set(task.id, task);
        this.emit(task);
        continue;
      }
      // Try the in-memory map first, then IndexedDB.
      const file = this.files.get(meta.id) ?? (await this.io.loadFile(meta.id).catch(() => undefined));
      const task: UploadTask = {
        ...meta,
        totalChunks: chunkCount(meta.size, meta.chunkSize),
        uploadedChunks: [],
        bytesUploaded: 0,
        speed: 0,
        status: 'initializing',
      };
      this.tasks.set(task.id, task);
      if (!file) {
        // Original file is no longer accessible -> clear message + reselect.
        task.status = 'needs-file';
        task.error = 'ORIGINAL_FILE_MISSING';
        this.emit(task);
        continue;
      }
      this.files.set(task.id, file);
      if (!task.uploadId) {
        void this.runTask(task, file, false);
        continue;
      }
      try {
        const status = await this.io.status(task.uploadId);
        task.uploadedChunks = [...(status.uploadedChunks ?? [])].sort((a, b) => a - b);
        task.bytesUploaded = this.bytesForChunks(task);
        this.emit(task);
        void this.runTask(task, file, true);
      } catch {
        void this.runTask(task, file, false);
      }
    }
  }

  private bytesForChunks(task: UploadTask): number {
    const ranges = splitChunks(task.size, task.chunkSize);
    const done = new Set(task.uploadedChunks);
    return ranges.filter((r) => done.has(r.index)).reduce((sum, r) => sum + (r.end - r.start), 0);
  }

  cancel(id: string): void {
    const task = this.tasks.get(id);
    if (!task) return;
    this.controllers.get(id)?.abort();
    this.controllers.delete(id);
    if (task.uploadId) void this.io.cancel(task.uploadId).catch(() => undefined);
    task.uploadId = undefined;
    task.uploadedChunks = [];
    task.bytesUploaded = 0;
    task.status = 'cancelled';
    task.error = undefined;
    this.emit(task);
  }

  retry(id: string): void {
    const task = this.tasks.get(id);
    if (!task) return;
    const file = this.files.get(id);
    if (!file) {
      task.status = 'needs-file';
      task.error = 'ORIGINAL_FILE_MISSING';
      this.emit(task);
      return;
    }
    task.error = undefined;
    void this.runTask(task, file, Boolean(task.uploadId));
  }

  /** User re-selects the original file for a task stuck in 'needs-file'. */
  reattach(id: string, file: File): void {
    const task = this.tasks.get(id);
    if (!task) return;
    this.files.set(id, file);
    task.name = file.name;
    task.size = file.size;
    task.mime = file.type || 'application/octet-stream';
    task.chunkSize = this.chunkSize;
    task.totalChunks = chunkCount(file.size, this.chunkSize);
    task.uploadedChunks = [];
    task.bytesUploaded = 0;
    task.uploadId = undefined;
    task.sha256 = undefined;
    task.error = undefined;
    void this.io.saveFile(id, file).catch(() => undefined);
    this.emit(task);
    void this.runTask(task, file, false);
  }

  removeTask(id: string): void {
    this.controllers.get(id)?.abort();
    this.controllers.delete(id);
    this.tasks.delete(id);
    this.files.delete(id);
    void this.io.removeFile(id).catch(() => undefined);
    this.persistMeta();
  }

  clearFinished(): void {
    for (const task of [...this.tasks.values()]) {
      if (task.status === 'done' || task.status === 'cancelled') this.removeTask(task.id);
    }
  }

  private finishWithAttachment(task: UploadTask, attachmentId: string, _file: File): void {
    task.attachmentId = attachmentId;
    task.status = 'done';
    task.bytesUploaded = task.size;
    task.uploadedChunks = Array.from({ length: task.totalChunks }, (_, i) => i);
    task.error = undefined;
    this.files.delete(task.id);
    void this.io.removeFile(task.id).catch(() => undefined);
    this.emit(task);
    this.hooks.onAttachment?.(attachmentId, task);
  }

  private async runTask(task: UploadTask, file: File, resume: boolean): Promise<void> {
    let done = new Set<number>();
    try {
      if (!resume) {
        task.status = 'hashing';
        this.emit(task);
        if (!task.sha256) task.sha256 = await this.io.hash(file);

        task.status = 'initializing';
        this.emit(task);
        const req: InitUploadRequest = {
          name: task.name,
          size: task.size,
          mime: task.mime,
          sha256: task.sha256,
          totalChunks: task.totalChunks,
          chunkSize: task.chunkSize,
        };
        const init = await this.io.init(req);
        task.uploadId = init.uploadId;
        if (init.reusedAttachmentId) {
          this.finishWithAttachment(task, init.reusedAttachmentId, file);
          return;
        }
        done = new Set(init.uploadedChunks ?? []);
      } else {
        await this.io.retry(task.uploadId!);
        const status = await this.io.status(task.uploadId!);
        done = new Set(status.uploadedChunks ?? []);
      }

      task.uploadedChunks = [...done].sort((a, b) => a - b);
      task.bytesUploaded = this.bytesForChunks(task);
      task.status = 'uploading';
      this.emit(task);

      const ranges = splitChunks(task.size, task.chunkSize).filter((r) => !done.has(r.index));
      const controller = new AbortController();
      this.controllers.set(task.id, controller);
      let lastByteAt = Date.now();

      await runWithConcurrency(
        ranges,
        this.concurrency,
        async (range) => {
          if (controller.signal.aborted) throw new DOMException('Aborted', 'AbortError');
          const blob = file.slice(range.start, range.end);
          await this.io.putChunk(task.uploadId!, range.index, blob, controller.signal);
          done.add(range.index);
          const now = Date.now();
          const delta = range.end - range.start;
          task.speed = calculateSpeed(delta, now - lastByteAt, task.speed);
          lastByteAt = now;
          task.bytesUploaded = Math.min(task.size, task.bytesUploaded + delta);
          task.uploadedChunks = [...done].sort((a, b) => a - b);
          this.emit(task);
        },
        () => controller.signal.aborted,
      );
      this.controllers.delete(task.id);

      if (controller.signal.aborted) {
        task.status = 'cancelled';
        this.emit(task);
        return;
      }

      task.status = 'completing';
      this.emit(task);
      const { attachmentId } = await this.io.complete(task.uploadId!);
      this.finishWithAttachment(task, attachmentId, file);
    } catch (e) {
      this.controllers.delete(task.id);
      if (e instanceof DOMException && e.name === 'AbortError') {
        task.status = 'cancelled';
        task.error = undefined;
      } else {
        task.status = 'error';
        task.error = e instanceof Error ? e.message : String(e);
        this.hooks.onError?.(task.error, task);
      }
      this.emit(task);
    }
  }
}
