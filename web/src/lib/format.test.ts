import { describe, expect, it } from 'vitest';
import {
  calculateSpeed,
  chunkCount,
  detectAttachmentType,
  formatBytes,
  formatDuration,
  retentionInfo,
} from './format';
import { splitChunks as split } from '../upload/scheduler';

describe('formatBytes', () => {
  it('formats with the right units', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1024)).toBe('1.0 KB');
    expect(formatBytes(1536, 2)).toBe('1.50 KB');
    expect(formatBytes(1024 ** 3)).toBe('1.0 GB');
  });
});

describe('formatDuration', () => {
  it('renders seconds, minutes and hours', () => {
    expect(formatDuration(30_000)).toBe('30s');
    expect(formatDuration(90_000)).toBe('1m 30s');
    expect(formatDuration(3_661_000)).toBe('1h 01m');
    expect(formatDuration(-1)).toBe('—');
  });
});

describe('retentionInfo', () => {
  const now = Date.UTC(2026, 0, 1);
  it('handles missing expiry as never expiring', () => {
    expect(retentionInfo(null, now).days).toBe(Number.POSITIVE_INFINITY);
  });
  it('computes remaining days and expiry', () => {
    const future = new Date(now + 2 * 86_400_000).toISOString();
    expect(retentionInfo(future, now)).toMatchObject({ expired: false, days: 2 });
    const past = new Date(now - 1000).toISOString();
    expect(retentionInfo(past, now).expired).toBe(true);
  });
});

describe('detectAttachmentType', () => {
  it('classifies by mime and extension', () => {
    expect(detectAttachmentType('image/png')).toBe('image');
    expect(detectAttachmentType('video/mp4')).toBe('video');
    expect(detectAttachmentType('audio/mpeg')).toBe('audio');
    expect(detectAttachmentType('application/pdf')).toBe('pdf');
    expect(detectAttachmentType('application/vnd.openxmlformats-officedocument.wordprocessingml.document')).toBe('office');
    expect(detectAttachmentType('', 'notes.md')).toBe('text');
    expect(detectAttachmentType('', 'main.ts')).toBe('code');
    expect(detectAttachmentType('application/zip', 'a.zip')).toBe('other');
  });
});

describe('chunking', () => {
  it('counts chunks correctly', () => {
    expect(chunkCount(0, 1024)).toBe(1);
    expect(chunkCount(1024, 1024)).toBe(1);
    expect(chunkCount(1025, 1024)).toBe(2);
  });
  it('produces contiguous ranges with a correct tail', () => {
    const ranges = split(2500, 1000);
    expect(ranges).toHaveLength(3);
    expect(ranges[0]).toEqual({ index: 0, start: 0, end: 1000 });
    expect(ranges[2]).toEqual({ index: 2, start: 2000, end: 2500 });
    expect(split(0, 1000)).toHaveLength(0);
  });
});

describe('calculateSpeed', () => {
  it('computes bytes/sec and smooths with the previous estimate', () => {
    expect(calculateSpeed(1000, 1000)).toBe(1000);
    expect(calculateSpeed(1000, 1000, 1000)).toBe(1000);
    expect(calculateSpeed(-5, 1000, 500)).toBe(500);
  });
});
