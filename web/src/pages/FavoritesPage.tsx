import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Attachment, Message } from '../api/types';
import { messagesApi } from '../api/messages';
import { filesApi } from '../api/files';
import { useUiStore } from '../store/ui';
import { MessageStream } from '../components/messages/MessageStream';
import { FilePreview } from '../components/FilePreview';
import { AuthImage } from '../components/ui/AuthImage';
import { EmptyState, IconButton } from '../components/ui/primitives';
import { formatBytes } from '../lib/format';
import { Icon } from '../components/ui/Icon';

export function FavoritesPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<'messages' | 'files'>('messages');
  const [messages, setMessages] = useState<Message[]>([]);
  const [files, setFiles] = useState<Attachment[]>([]);
  const [preview, setPreview] = useState<Attachment | null>(null);
  const dataVersion = useUiStore((s) => s.dataVersion);

  const load = useCallback(async () => {
    const [m, f] = await Promise.all([
      messagesApi.list({ favorite: true, limit: 100 }),
      filesApi.list({ favorite: true, limit: 100 }),
    ]);
    setMessages(m.items);
    setFiles(f.items);
  }, []);

  useEffect(() => { void load(); }, [load, dataVersion]);

  const unfavoriteFile = async (file: Attachment) => {
    const updated = await filesApi.update(file.id, { favorited: false });
    setFiles((prev) => prev.filter((f) => f.id !== updated.id));
  };

  return (
    <div className="mx-auto w-full max-w-4xl px-3 py-4 sm:px-5">
      <div className="mb-4 inline-flex rounded-lg border border-slate-200 bg-white p-0.5 dark:border-slate-700 dark:bg-slate-800">
        {(['messages', 'files'] as const).map((item) => (
          <button
            key={item}
            type="button"
            onClick={() => setTab(item)}
            aria-pressed={tab === item}
            className={`rounded-md px-4 py-1.5 text-sm font-medium ${tab === item ? 'bg-blue-600 text-white' : 'text-slate-500'}`}
          >
            {t(item === 'messages' ? 'nav.messages' : 'nav.files')}
            <span className="ml-1.5 text-xs opacity-70">{item === 'messages' ? messages.length : files.length}</span>
          </button>
        ))}
      </div>

      {tab === 'messages' ? (
        <div className="-mx-3 sm:-mx-5">
          <MessageStream messages={messages} onChanged={() => void load()} emptyTitle={t('files.noFiles')} />
        </div>
      ) : files.length === 0 ? (
        <EmptyState icon="star" title={t('files.noFiles')} />
      ) : (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {files.map((file) => (
            <article key={file.id} className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
              <button type="button" onClick={() => setPreview(file)} className="flex aspect-square w-full items-center justify-center bg-slate-100 dark:bg-slate-800">
                {file.type === 'image'
                  ? <AuthImage path={file.thumbnailUrl || `/api/files/${file.id}/thumbnail`} alt={file.name} className="size-full object-cover" />
                  : <Icon name="file" size={32} className="text-slate-400" />}
              </button>
              <div className="flex items-center gap-1 p-2">
                <span className="min-w-0 flex-1 truncate text-sm" title={file.name}>{file.name}</span>
                <span className="text-[10px] text-slate-400">{formatBytes(file.size)}</span>
                <IconButton icon="star" label={t('files.unfavorite')} className="text-yellow-500" onClick={() => void unfavoriteFile(file)} />
              </div>
            </article>
          ))}
        </div>
      )}
      <div className="h-16 md:hidden" />
      <FilePreview file={preview} onClose={() => setPreview(null)} />
    </div>
  );
}
