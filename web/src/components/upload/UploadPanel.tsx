import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useUiStore } from '../../store/ui';
import { useUploadsStore } from '../../store/uploads';
import type { UploadTask } from '../../upload/types';
import { formatBytes, formatDuration } from '../../lib/format';
import { Modal } from '../ui/Modal';
import { Button, IconButton, ProgressBar } from '../ui/primitives';
import { Icon } from '../ui/Icon';

function statusLabel(t: (k: string) => string, task: UploadTask): string {
  if (task.status === 'error' && task.error) return task.error === 'ORIGINAL_FILE_MISSING' ? t('upload.needsFile') : `${t('upload.error')}: ${task.error}`;
  return t(`upload.${task.status}`);
}

function TaskRow({ task }: { task: UploadTask }) {
  const { t } = useTranslation();
  const cancel = useUploadsStore((s) => s.cancel);
  const retry = useUploadsStore((s) => s.retry);
  const reattach = useUploadsStore((s) => s.reattach);
  const removeTask = useUploadsStore((s) => s.removeTask);
  const fileInput = useRef<HTMLInputElement>(null);
  const indeterminate = ['hashing', 'initializing', 'completing'].includes(task.status);
  const percent = task.size > 0 ? (task.bytesUploaded / task.size) * 100 : 0;
  const remaining = task.speed > 0 ? Math.max(0, (task.size - task.bytesUploaded) / task.speed) * 1000 : Infinity;

  return (
    <li className="rounded-xl border border-slate-200 p-3 dark:border-slate-700">
      <div className="flex items-start gap-2">
        <span className="mt-0.5 text-slate-400"><Icon name="file" size={18} /></span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium" title={task.name}>{task.name}</p>
          <p className="text-xs text-slate-400">{formatBytes(task.size)} · {task.totalChunks} {t('upload.chunkSize').toLowerCase()}</p>
        </div>
        <div className="flex items-center">
          {(task.status === 'uploading' || task.status === 'hashing' || task.status === 'initializing' || task.status === 'completing') && (
            <IconButton icon="x" label={t('upload.cancel')} onClick={() => cancel(task.id)} />
          )}
          {(task.status === 'error' || task.status === 'needs-file') && (
            <IconButton icon="refresh" label={t('upload.retry')} onClick={() => retry(task.id)} />
          )}
          {['done', 'cancelled', 'error', 'needs-file'].includes(task.status) && (
            <IconButton icon="trash" label={t('upload.remove')} onClick={() => removeTask(task.id)} />
          )}
        </div>
      </div>

      <div className="mt-2">
        <ProgressBar value={indeterminate ? undefined : percent} label={`${task.name} ${t('upload.progress')}`} />
        <div className="mt-1 flex justify-between text-[11px] text-slate-400">
          <span>{statusLabel(t, task)}</span>
          <span className="tabular-nums">
            {task.status === 'uploading'
              ? `${formatBytes(task.speed)}/s · ${formatDuration(remaining)}`
              : `${formatBytes(task.bytesUploaded)} / ${formatBytes(task.size)}`}
          </span>
        </div>
      </div>

      {task.status === 'needs-file' && (
        <div className="mt-2 rounded-lg bg-amber-50 p-2 text-xs text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
          <p>{t('upload.originalMissing')}</p>
          <input
            ref={fileInput}
            type="file"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) reattach(task.id, file);
              e.target.value = '';
            }}
          />
          <Button variant="secondary" className="mt-2" icon="paperclip" onClick={() => fileInput.current?.click()}>
            {t('upload.reselect')}
          </Button>
        </div>
      )}
    </li>
  );
}

export function UploadPanel() {
  const { t } = useTranslation();
  const open = useUiStore((s) => s.uploadPanelOpen);
  const setOpen = useUiStore((s) => s.setUploadPanelOpen);
  const tasks = useUploadsStore((s) => s.tasks);
  const clearFinished = useUploadsStore((s) => s.clearFinished);
  const hasFinished = tasks.some((task) => ['done', 'cancelled'].includes(task.status));

  return (
    <Modal
      open={open}
      onClose={() => setOpen(false)}
      title={t('upload.panel')}
      size="md"
      footer={
        <>
          <Button variant="ghost" onClick={() => setOpen(false)}>{t('common.close')}</Button>
          <Button variant="secondary" onClick={clearFinished} disabled={!hasFinished}>{t('upload.clearFinished')}</Button>
        </>
      }
    >
      {tasks.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-slate-400">
          <Icon name="upload" size={36} />
          <p className="text-sm">{t('upload.noTasks')}</p>
          <p className="max-w-xs text-center text-xs">{t('upload.resumeHint')}</p>
        </div>
      ) : (
        <ul className="flex flex-col gap-2">
          {tasks.map((task) => <TaskRow key={task.id} task={task} />)}
        </ul>
      )}
    </Modal>
  );
}
