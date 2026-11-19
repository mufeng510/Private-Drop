import { useCallback, useEffect, useRef, useState, type DragEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { messagesApi } from '../../api/messages';
import { errorMessage } from '../../api/client';
import { useUploadsStore } from '../../store/uploads';
import { useUiStore } from '../../store/ui';
import { SEND_EVENT } from '../../hooks/useKeyboardShortcuts';
import { formatBytes } from '../../lib/format';
import { Button, IconButton, ProgressBar } from '../ui/primitives';
import { Icon } from '../ui/Icon';

interface ShareDetail {
  text?: string;
  files?: File[];
}

export function Composer({ onSent }: { onSent: () => void }) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const [taskIds, setTaskIds] = useState<string[]>([]);
  const [dragging, setDragging] = useState(false);
  const [sending, setSending] = useState(false);
  const textarea = useRef<HTMLTextAreaElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const cameraInput = useRef<HTMLInputElement>(null);

  const allTasks = useUploadsStore((s) => s.tasks);
  const addFiles = useUploadsStore((s) => s.addFiles);
  const getTask = useUploadsStore((s) => s.getTask);
  const removeTask = useUploadsStore((s) => s.removeTask);
  const cancel = useUploadsStore((s) => s.cancel);
  const toast = useUiStore((s) => s.toast);
  const panelOpen = useUiStore((s) => s.uploadPanelOpen);
  const setPanelOpen = useUiStore((s) => s.setUploadPanelOpen);

  const tasks = taskIds.map((id) => allTasks.find((task) => task.id === id)).filter((x) => x !== undefined);

  const importFiles = useCallback((files: File[]) => {
    if (files.length === 0) return;
    const created = addFiles(files);
    setTaskIds((prev) => [...prev, ...created.map((task) => task.id)]);
  }, [addFiles]);

  useEffect(() => {
    const onPaste = (e: ClipboardEvent) => {
      const files: File[] = [];
      for (const item of e.clipboardData?.items ?? []) {
        if (item.kind === 'file') {
          const file = item.getAsFile();
          if (file) files.push(file);
        }
      }
      if (files.length > 0) {
        e.preventDefault();
        importFiles(files);
        return;
      }
      const active = document.activeElement;
      const isEditable = active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement || (active as HTMLElement)?.isContentEditable;
      if (!isEditable) {
        const pasted = e.clipboardData?.getData('text') ?? '';
        if (pasted) {
          e.preventDefault();
          setText((prev) => prev + pasted);
          textarea.current?.focus();
        }
      }
    };
    const onShare = (e: Event) => {
      const detail = (e as CustomEvent<ShareDetail>).detail;
      const sharedText = detail?.text ?? '';
      if (sharedText) setText((prev) => (prev ? `${prev}\n${sharedText}` : sharedText));
      if (detail?.files?.length) importFiles(detail.files);
      toast(t('composer.shareReceived'), 'success');
      textarea.current?.focus();
    };
    const onSend = () => void send();
    window.addEventListener('paste', onPaste);
    window.addEventListener('pd:share', onShare);
    window.addEventListener(SEND_EVENT, onSend);
    return () => {
      window.removeEventListener('paste', onPaste);
      window.removeEventListener('pd:share', onShare);
      window.removeEventListener(SEND_EVENT, onSend);
    };
  });

  const send = async () => {
    if (sending) return;
    const bodies = taskIds.map(getTask).filter((x) => x !== undefined);
    const attachmentIds = bodies.filter((task) => task.status === 'done' && task.attachmentId).map((task) => task.attachmentId!);
    if (!text.trim() && attachmentIds.length === 0) return;
    if (bodies.length > attachmentIds.length) {
      toast(t('composer.waitingUpload'), 'info');
      if (!panelOpen) setPanelOpen(true);
      return;
    }
    setSending(true);
    try {
      await messagesApi.create({ text: text.trim(), attachmentIds });
      setText('');
      setTaskIds([]);
      onSent();
    } catch (e) {
      toast(errorMessage(e), 'error');
    } finally {
      setSending(false);
    }
  };

  const drop = (e: DragEvent) => {
    e.preventDefault();
    setDragging(false);
    importFiles(Array.from(e.dataTransfer.files));
  };

  const remove = (id: string) => {
    cancel(id);
    removeTask(id);
    setTaskIds((prev) => prev.filter((x) => x !== id));
  };

  return (
    <div
      className={`relative rounded-2xl border bg-white shadow-sm dark:bg-slate-900 ${dragging ? 'border-blue-500 ring-2 ring-blue-500/20' : 'border-slate-200 dark:border-slate-700'}`}
      onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
      onDragEnter={(e) => { e.preventDefault(); setDragging(true); }}
      onDragLeave={(e) => { if (e.currentTarget === e.target) setDragging(false); }}
      onDrop={drop}
    >
      {dragging && (
        <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center rounded-2xl bg-blue-50/90 text-sm font-medium text-blue-700 dark:bg-blue-950/80 dark:text-blue-300">
          <Icon name="upload" size={20} className="mr-2" /> {t('composer.dropHere')}
        </div>
      )}

      {tasks.length > 0 && (
        <div className="flex flex-wrap gap-2 border-b border-slate-100 p-3 dark:border-slate-800">
          {tasks.map((task) => (
            <div key={task.id} className="w-44 rounded-lg border border-slate-200 p-2 dark:border-slate-700">
              <div className="flex items-center gap-1.5">
                <Icon name="file" size={14} className="shrink-0 text-slate-400" />
                <span className="min-w-0 flex-1 truncate text-xs" title={task.name}>{task.name}</span>
                <button type="button" onClick={() => remove(task.id)} aria-label={t('composer.removeAttachment')} className="text-slate-400 hover:text-red-600"><Icon name="x" size={14} /></button>
              </div>
              <div className="mt-1.5"><ProgressBar value={task.status === 'done' ? 100 : task.size > 0 ? (task.bytesUploaded / task.size) * 100 : undefined} /></div>
              <p className="mt-1 truncate text-[10px] text-slate-400">{t(`upload.${task.status}`)} · {formatBytes(task.size)}</p>
            </div>
          ))}
        </div>
      )}

      <textarea
        ref={textarea}
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={3}
        placeholder={t('composer.placeholder')}
        aria-label={t('composer.placeholder')}
        className="w-full resize-none rounded-t-2xl bg-transparent px-4 py-3 text-sm text-slate-800 placeholder:text-slate-400 focus:outline-none dark:text-slate-100"
      />
      <div className="flex items-center gap-1 px-3 pb-3">
        <input ref={fileInput} type="file" multiple className="hidden" onChange={(e) => { importFiles(Array.from(e.target.files ?? [])); e.target.value = ''; }} />
        <input ref={cameraInput} type="file" accept="image/*,video/*" capture="environment" className="hidden" onChange={(e) => { importFiles(Array.from(e.target.files ?? [])); e.target.value = ''; }} />
        <IconButton icon="paperclip" label={t('composer.attachFile')} onClick={() => fileInput.current?.click()} />
        <IconButton icon="camera" label={t('composer.takePhoto')} onClick={() => cameraInput.current?.click()} />
        <span className="hidden text-[11px] text-slate-400 sm:inline">{t('composer.CtrlEnter')}</span>
        <div className="flex-1" />
        <Button variant="primary" icon="send" onClick={() => void send()} disabled={sending || (!text.trim() && tasks.length === 0)}>
          {sending ? t('composer.sending') : t('composer.send')}
        </Button>
      </div>
    </div>
  );
}
