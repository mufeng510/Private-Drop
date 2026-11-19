import type { InitUploadRequest, InitUploadResponse, UploadedChunkStatus } from '../api/types';

export type UploadStatus =
  | 'hashing'
  | 'initializing'
  | 'uploading'
  | 'completing'
  | 'done'
  | 'error'
  | 'cancelled'
  | 'needs-file';

export interface UploadTask {
  /** Client-generated id, stable across reloads (persisted in localStorage). */
  id: string;
  /** Server upload id, assigned after init. */
  uploadId?: string;
  name: string;
  size: number;
  mime: string;
  sha256?: string;
  chunkSize: number;
  totalChunks: number;
  uploadedChunks: number[];
  status: UploadStatus;
  attachmentId?: string;
  bytesUploaded: number;
  speed: number;
  error?: string;
  createdAt: number;
}

export interface UploadIO {
  init(req: InitUploadRequest): Promise<InitUploadResponse>;
  putChunk(uploadId: string, index: number, chunk: Blob, signal: AbortSignal): Promise<unknown>;
  complete(uploadId: string): Promise<{ attachmentId: string }>;
  status(uploadId: string): Promise<UploadedChunkStatus>;
  cancel(uploadId: string): Promise<unknown>;
  retry(uploadId: string): Promise<unknown>;
  hash(file: File): Promise<string>;
  saveFile(id: string, file: File): Promise<unknown>;
  loadFile(id: string): Promise<File | undefined>;
  removeFile(id: string): Promise<unknown>;
}

export interface UploadHooks {
  onUpdate(task: UploadTask): void;
  onAttachment?(attachmentId: string, task: UploadTask): void;
  onError?(message: string, task: UploadTask): void;
}
