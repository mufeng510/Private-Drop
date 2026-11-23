import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Message } from '../api/types';
import { messagesApi } from '../api/messages';
import { errorMessage } from '../api/client';
import { useUiStore } from '../store/ui';
import { MessageStream } from '../components/messages/MessageStream';
import { Button } from '../components/ui/primitives';

export function ExpiringPage() {
  const { t } = useTranslation();
  const [messages, setMessages] = useState<Message[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const toast = useUiStore((s) => s.toast);
  const dataVersion = useUiStore((s) => s.dataVersion);
  const bumpData = useUiStore((s) => s.bumpData);

  const load = useCallback(async () => {
    try {
      const res = await messagesApi.expiring({ limit: 100 });
      setMessages(res.items);
      setCursor(res.nextCursor);
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  }, [toast]);

  useEffect(() => { void load(); }, [load, dataVersion]);

  const favoriteAll = async () => {
    const ids = messages.filter((m) => !m.favorited).map((m) => m.id);
    if (ids.length === 0) return;
    try {
      await messagesApi.bulk({ ids, action: 'favorite' });
      toast(t('common.saved'), 'success');
      bumpData();
      void load();
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  return (
    <div>
      <div className="mx-auto flex w-full max-w-3xl justify-end px-3 pt-4 sm:px-5">
        <Button variant="secondary" icon="star" onClick={() => void favoriteAll()} disabled={messages.every((m) => m.favorited)}>
          {t('expiring.favoriteAll')}
        </Button>
      </div>
      <MessageStream messages={messages} hasMore={Boolean(cursor)} onLoadMore={async () => {
        if (!cursor) return;
        const res = await messagesApi.expiring({ limit: 100, cursor });
        setMessages((prev) => [...prev, ...res.items]);
        setCursor(res.nextCursor);
      }} onChanged={() => void load()} emptyTitle={t('expiring.noItems')} showSort={false} />
    </div>
  );
}
