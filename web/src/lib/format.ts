import type { AttachmentType } from '../api/types';

export function formatBytes(bytes: number, digits = 1): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  const value = bytes / 1024 ** i;
  return `${value.toFixed(i === 0 ? 0 : digits)} ${units[i]}`;
}

/** "2m 30s" / "1h 05m" style duration for remaining time / speed ETA. */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '—';
  const total = Math.floor(ms / 1000);
  if (total < 60) return `${total}s`;
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m`;
  return `${m}m ${String(s).padStart(2, '0')}s`;
}

export function formatDateTime(iso: string, lang = 'zh-CN'): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(lang, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(d);
}

export function formatRelativeTime(iso: string, lang = 'zh-CN', now = Date.now()): string {
  const d = new Date(iso);
  const diffSec = Math.round((d.getTime() - now) / 1000);
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: 'auto' });
  const abs = Math.abs(diffSec);
  if (abs < 60) return rtf.format(diffSec, 'second');
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute');
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour');
  if (abs < 86400 * 30) return rtf.format(Math.round(diffSec / 86400), 'day');
  return formatDateTime(iso, lang);
}

export interface RetentionInfo {
  expired: boolean;
  ms: number;
  /** Whole days left (floored). */
  days: number;
}

export function retentionInfo(expiresAt: string | null | undefined, now = Date.now()): RetentionInfo {
  if (!expiresAt) return { expired: false, ms: Number.POSITIVE_INFINITY, days: Number.POSITIVE_INFINITY };
  const ms = new Date(expiresAt).getTime() - now;
  return { expired: ms <= 0, ms, days: Math.max(0, Math.floor(ms / 86_400_000)) };
}

const CODE_EXT = new Set([
  'js', 'jsx', 'ts', 'tsx', 'json', 'py', 'go', 'rs', 'java', 'c', 'h', 'cpp', 'cs',
  'php', 'rb', 'sh', 'bash', 'css', 'scss', 'html', 'htm', 'xml', 'yaml', 'yml', 'sql', 'toml',
]);
const OFFICE_EXT = new Set(['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'odt', 'ods', 'odp']);

export function fileExtension(name: string): string {
  const idx = name.lastIndexOf('.');
  return idx > 0 ? name.slice(idx + 1).toLowerCase() : '';
}

/** Best-effort classification matching the backend Attachment.type. */
export function detectAttachmentType(mime: string, name = ''): AttachmentType {
  const m = (mime || '').toLowerCase();
  const ext = fileExtension(name);
  if (m.startsWith('image/')) return 'image';
  if (m.startsWith('video/')) return 'video';
  if (m.startsWith('audio/')) return 'audio';
  if (m === 'application/pdf' || ext === 'pdf') return 'pdf';
  if (OFFICE_EXT.has(ext) || m.includes('officedocument') || m.includes('msword')) return 'office';
  if (m.startsWith('text/') || ['txt', 'md', 'markdown', 'log', 'csv'].includes(ext)) return 'text';
  if (CODE_EXT.has(ext) || m.includes('javascript') || m.includes('json')) return 'code';
  return 'other';
}

export const PREVIEWABLE_TYPES = new Set<AttachmentType>(['image', 'video', 'audio', 'pdf', 'text', 'code']);

export function canPreview(type: AttachmentType): boolean {
  return PREVIEWABLE_TYPES.has(type);
}

export function chunkCount(size: number, chunkSize: number): number {
  if (chunkSize <= 0) return 1;
  return Math.max(1, Math.ceil(size / chunkSize));
}

/** Smooth-ish speed estimate in bytes/second from a byte delta and time delta. */
export function calculateSpeed(bytesDelta: number, msDelta: number, previous = 0): number {
  if (msDelta <= 0 || bytesDelta < 0) return previous;
  const instant = (bytesDelta / msDelta) * 1000;
  return previous === 0 ? instant : previous * 0.6 + instant * 0.4;
}

export function initials(name: string): string {
  return name.trim().slice(0, 1).toUpperCase() || '?';
}
