import { wsClient } from './api/ws';
import { authApi } from './api/auth';
import type { Message, NotificationItem, WsFrame } from './api/types';
import { useMessagesStore } from './store/messages';
import { useNotificationsStore } from './store/notifications';
import { useUploadsStore } from './store/uploads';
import { useUiStore } from './store/ui';
import { useAuthStore } from './store/auth';

let wired = false;

function wireHandlers(): void {
  if (wired) return;
  wired = true;

  wsClient.onStatus((status) => useUiStore.getState().setWsStatus(status));

  wsClient.on('message.created', (frame: WsFrame) => {
    useMessagesStore.getState().upsert(frame.data as Message);
    useUiStore.getState().bumpData();
  });
  wsClient.on('message.updated', (frame: WsFrame) => {
    useMessagesStore.getState().upsert(frame.data as Message);
    useUiStore.getState().bumpData();
  });
  wsClient.on('message.deleted', (frame: WsFrame) => {
    const data = frame.data as { id?: string } | string;
    const id = typeof data === 'string' ? data : data?.id;
    if (id) useMessagesStore.getState().remove(id);
    useUiStore.getState().bumpData();
  });
  wsClient.on('upload.progress', (frame: WsFrame) => {
    const data = frame.data as { uploadId?: string; uploadedChunks?: number[]; status?: string };
    if (data?.uploadId) {
      useUploadsStore.getState().remoteUpdate(data.uploadId, data.uploadedChunks ?? [], data.status);
    }
  });
  wsClient.on('upload.cancelled', (frame: WsFrame) => {
    const data = frame.data as { uploadId?: string };
    if (data?.uploadId) useUploadsStore.getState().remoteUpdate(data.uploadId, [], 'cancelled');
    useUiStore.getState().bumpData();
  });
  wsClient.on('upload.needs_source', (frame: WsFrame) => {
    const data = frame.data as { uploadId?: string };
    if (data?.uploadId) useUploadsStore.getState().remoteUpdate(data.uploadId, [], 'needs_source');
    useUiStore.getState().toast('upload.needsFile', 'error');
    useUiStore.getState().bumpData();
  });
  // Attachment state is synced separately from messages (taskbook §6/§11).
  for (const type of ['file.updated', 'file.deleted', 'file.restored']) {
    wsClient.on(type, () => useUiStore.getState().bumpData());
  }
  wsClient.on('upload.completed', () => {
    useUiStore.getState().toast('upload.done', 'success');
    useUiStore.getState().bumpData();
  });
  wsClient.on('notification', (frame: WsFrame) => {
    useNotificationsStore.getState().push(frame.data as NotificationItem);
    const item = frame.data as NotificationItem;
    if (item?.type) useUiStore.getState().toast(`notifications.${item.type}`);
  });
  wsClient.on('device.new', () => {
    useUiStore.getState().toast('notifications.newDevice');
    useUiStore.getState().bumpData();
  });
  wsClient.on('device.revoked', () => {
    useUiStore.getState().toast('admin.revoked');
    useUiStore.getState().bumpData();
  });
  wsClient.on('settings.changed', () => {
    // Backend publishes only the changed key names, not values. Re-read the
    // authoritative settings instead of merging an empty payload (which would
    // otherwise reset the upload concurrency to 1).
    void authApi.me().then((me) => {
      useAuthStore.getState().setSettings(me.settings);
      const raw = me.settings as Record<string, unknown>;
      const concurrency = Number(raw.upload_concurrency ?? raw.uploadConcurrency);
      if (Number.isFinite(concurrency) && concurrency > 0) {
        useUploadsStore.getState().setConcurrency(concurrency);
      }
    }).catch(() => undefined);
    useUiStore.getState().bumpData();
  });
}

export function connectRealtime(token: string): void {
  wireHandlers();
  wsClient.connect(token);
}

export function disconnectRealtime(): void {
  wsClient.stop();
}
