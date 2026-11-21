// Types mirror docs/API_CONTRACT.md exactly. Keep in sync with the backend.

export type DeviceType = 'phone' | 'tablet' | 'desktop' | 'unknown';
export type AttachmentType =
  | 'image'
  | 'video'
  | 'audio'
  | 'pdf'
  | 'office'
  | 'text'
  | 'code'
  | 'other';
export type MessageSort = 'newest' | 'oldest' | 'favorite' | 'pinned';
export type DevicePolicy = 'open' | 'whitelist' | 'approval';

export interface User {
  id: string;
  username: string;
  createdAt: string;
  /** Backend serializes these as hasTotp / hasPasskey. */
  hasTotp?: boolean;
  hasPasskey?: boolean;
  /** Kept for compatibility with older callers. */
  totpEnabled?: boolean;
  passkeyEnabled?: boolean;
}

export interface Device {
  id: string;
  name: string;
  type: DeviceType;
  ip?: string;
  createdAt: string;
  lastActive?: string;
  authorized: boolean;
  revokedAt?: string | null;
  current?: boolean;
}

export interface DeviceRequest {
  id: string;
  name: string;
  type: DeviceType;
  ip?: string;
  code?: string;
  createdAt: string;
  status?: 'pending' | 'approved' | 'rejected';
}

export type Settings = Record<string, string | number | boolean | null>;

export interface LinkPreview {
  url: string;
  title?: string;
  description?: string;
  imageUrl?: string;
}

export interface Tag {
  id: string;
  name: string;
  color?: string;
  icon?: string;
  sort?: number;
  isSystem?: boolean;
}

export interface MessageAttachmentRef {
  id: string;
  name: string;
  mime: string;
  size: number;
  type: AttachmentType;
  width?: number;
  height?: number;
  favorited?: boolean;
  thumbnailUrl?: string;
}

export interface Message {
  id: string;
  text: string;
  pinned: boolean;
  favorited: boolean;
  createdAt: string;
  updatedAt: string;
  expiresAt?: string | null;
  deletedAt?: string | null;
  device: Pick<Device, 'id' | 'name' | 'type'>;
  attachments: MessageAttachmentRef[];
  tags: Tag[];
  links: LinkPreview[];
}

export interface Attachment {
  id: string;
  name: string;
  mime: string;
  size: number;
  sha256: string;
  width?: number;
  height?: number;
  favorited: boolean;
  uploadedAt: string;
  type: AttachmentType;
  messageIds: string[];
  tags?: Tag[];
  thumbnailUrl?: string;
  previewUrl?: string;
}

export interface UploadedChunkStatus {
  uploadId: string;
  uploadedChunks: number[];
  status: 'uploading' | 'completed' | 'failed' | 'cancelled' | 'needs_source';
  filename?: string;
  size?: number;
  mime?: string;
  chunkSize?: number;
  totalChunks?: number;
  /** Contract v2: byte-level progress reported by the server. */
  bytesUploaded?: number;
  bytesRemaining?: number;
  speed?: number;
}

export interface InitUploadRequest {
  name: string;
  size: number;
  mime: string;
  sha256?: string;
  totalChunks: number;
  chunkSize: number;
  /** Contract v2: bind the upload to a message for cancellation on delete. */
  messageId?: string;
}

export interface InitUploadResponse {
  uploadId: string;
  uploadedChunks: number[];
  reusedAttachmentId?: string;
}

export interface UploadListResponse {
  items: UploadedChunkStatus[];
}

export interface SearchResult {
  kind: 'message' | 'file';
  id: string;
  score?: number;
  message?: Message;
  file?: Attachment;
}

export interface RecycleItem {
  id: string;
  kind: 'message' | 'file';
  name?: string;
  text?: string;
  deletedAt: string;
  originalCreatedAt?: string;
  expiresAt?: string;
  attachmentCount?: number;
  size?: number;
}

export interface NotificationItem {
  id: string;
  type: string;
  payload: Record<string, unknown>;
  read: boolean;
  createdAt: string;
  /** Suggested in-app route, e.g. "/messages". */
  link?: string;
}

export interface NotificationsResponse {
  items: NotificationItem[];
  unread: number;
}

export interface LogEntry {
  id: string;
  type: 'app' | 'upload' | 'clean' | 'error' | 'security';
  level: 'debug' | 'info' | 'warn' | 'error';
  message: string;
  createdAt: string;
}

export interface AuditEntry {
  id: string;
  action: string;
  detail?: string;
  ip?: string;
  deviceId?: string;
  createdAt: string;
}

export interface StorageBreakdown {
  type: AttachmentType;
  bytes: number;
  count: number;
}

export interface StorageStats {
  totalBytes: number;
  fileCount: number;
  messageCount: number;
  byType: StorageBreakdown[];
  largestFiles: Attachment[];
  expiringFiles: Attachment[];
  recycleBytes: number;
  favoriteBytes: number;
  uploadTempBytes: number;
  duplicateBytes?: number;
  warnings?: string[];
}

export interface SystemInfo {
  version: string;
  port: number;
  dataDir: string;
  os?: string;
  uptime?: string;
  startedAt?: string;
  deployment?: string;
}

export interface DiagnoseResult {
  ok: boolean;
  checks: Array<{ name: string; ok: boolean; message?: string }>;
  checkedAt: string;
}

export interface AuthStatus {
  initialized: boolean;
  lanOnly: boolean;
}

export interface InitRequest {
  username: string;
  password: string;
  deviceName?: string;
  policy?: DevicePolicy;
}

export interface LoginRequest {
  username: string;
  password: string;
  deviceName?: string;
  totp?: string;
  /** Per-login session lifetime: positive = days, negative = forever, 0 = default. */
  ttlDays?: number;
}

export type MfaType = 'totp' | 'passkey';

export interface LoginResponse {
  token?: string;
  user: User;
  device: Device;
  mfaRequired?: MfaType;
  /** Bootstrap admin grant, issued only until a second factor is enrolled. */
  mfaToken?: string;
}

export interface InitResponse {
  token: string;
  user: User;
  device: Device;
  mfaToken?: string;
}

export interface MeResponse {
  user: User;
  device: Device;
  settings: Settings;
  /** Whether the current X-MFA-Token is still accepted by the server. */
  mfaValid: boolean;
}

export interface TotpSetup {
  secret: string;
  otpauthUrl: string;
  qr?: string;
}

export interface MfaVerifyRequest {
  type: MfaType;
  code?: string;
  assertion?: unknown;
}

export interface MfaVerifyResponse {
  mfaToken: string;
}

export interface LinkPreviewResponse {
  title?: string;
  description?: string;
  imageUrl?: string;
}

export interface Paginated<T> {
  items: T[];
  nextCursor: string | null;
}

export interface ApiErrorBody {
  error: { code: string; message: string };
}

/** Event frame pushed by the WebSocket server. */
export interface WsFrame<T = unknown> {
  type: string;
  data: T;
  ts: string;
}

export type WsEventType =
  | 'message.created'
  | 'message.updated'
  | 'message.deleted'
  | 'upload.progress'
  | 'upload.completed'
  | 'notification'
  | 'device.new'
  | 'device.revoked'
  | 'settings.changed';
