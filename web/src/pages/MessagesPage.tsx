import { useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { Message } from '../api/types';
import { messagesApi } from '../api/messages';
import { useMessagesStore } from '../store/messages';
import { useUiStore } from '../store/ui';
import { MessageStream } from '../components/messages/MessageStream';
import { MessageCard } from '../components/messages/MessageCard';
import { Composer } from '../components/messages/Composer';
import { FilePreview } from '../components/FilePreview';
import { IconButton } from '../components/ui/primitives';

export function MessagesPage() {
  const messages = useMessagesStore((s) => s.items);
  const pinned = useMessagesStore((s) => s.pinned);
  const sort = useMessagesStore((s) => s.sort);
  const loading = useMessagesStore((s) => s.loading);
  const nextCursor = useMessagesStore((s) => s.nextCursor);
  const load = useMessagesStore((s) => s.load);
  const loadMore = useMessagesStore((s) => s.loadMore);
  const setSort = useMessagesStore((s) => s.setSort);
  const dataVersion = useUiStore((s) => s.dataVersion);
  const [searchParams, setSearchParams] = useSearchParams();
  const jumpId = searchParams.get('msg');
  const [jumpMessage, setJumpMessage] = useState<Message | null>(null);
  const [preview, setPreview] = useState<import('../api/types').Attachment | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const nearBottomRef = useRef(true);
  const initialLoadedRef = useRef(false);
  const lastFirstIdRef = useRef<string | null>(null);

  useEffect(() => {
    void load();
  }, [load, dataVersion]);

  /**
   * Auto-locate (taskbook §5.2): jump to the newest message on the first load
   * of the "newest" sort, and follow WebSocket upserts only when the user is
   * already near the bottom. Loading older pages or other sort orders never
   * moves the scroll position.
   */
  useEffect(() => {
    if (loading || messages.length === 0) return;
    const el = scrollRef.current;
    if (!el) return;
    const firstId = messages[0]?.id ?? null;
    if (!initialLoadedRef.current) {
      initialLoadedRef.current = true;
      lastFirstIdRef.current = firstId;
      if (sort === 'newest') el.scrollTop = el.scrollHeight;
      return;
    }
    if (firstId !== lastFirstIdRef.current) {
      lastFirstIdRef.current = firstId;
      if (sort === 'newest' && nearBottomRef.current) el.scrollTop = el.scrollHeight;
    }
  }, [messages, loading, sort]);

  const onScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    nearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 140;
  }, []);

  useEffect(() => {
    if (!jumpId) {
      setJumpMessage(null);
      return;
    }
    let alive = true;
    messagesApi.get(jumpId).then((m) => alive && setJumpMessage(m)).catch(() => alive && setJumpMessage(null));
    return () => { alive = false; };
  }, [jumpId]);

  const onChanged = useCallback(() => {
    void load();
  }, [load]);

  // In chat layout "load more" prepends older messages above the viewport.
  // Keep the reader's current position anchored instead of jumping to the top.
  const handleLoadMore = useCallback(async () => {
    const el = scrollRef.current;
    const prevTop = el?.scrollTop ?? 0;
    const prevHeight = el?.scrollHeight ?? 0;
    await loadMore();
    requestAnimationFrame(() => {
      const node = scrollRef.current;
      if (node) node.scrollTop = prevTop + (node.scrollHeight - prevHeight);
    });
  }, [loadMore]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div ref={scrollRef} onScroll={onScroll} style={{ overflowAnchor: 'none' }} className="min-h-0 flex-1 overflow-y-auto">
        {jumpMessage && (
          <div className="mx-auto mt-4 max-w-3xl px-3 sm:px-5">
            <div className="mb-2 flex items-center justify-between rounded-lg bg-blue-50 px-3 py-1.5 text-xs text-blue-700 dark:bg-blue-950/40 dark:text-blue-300">
              <span>Original message</span>
              <IconButton icon="x" label="Close" onClick={() => { setJumpMessage(null); setSearchParams({}, { replace: true }); }} />
            </div>
            <MessageCard message={jumpMessage} selected={false} selectMode={false} onToggleSelect={() => undefined} onPreview={setPreview} onChanged={() => void load()} />
          </div>
        )}
        <MessageStream
          messages={messages}
          pinned={pinned}
          loading={loading}
          hasMore={Boolean(nextCursor)}
          onLoadMore={() => void handleLoadMore()}
          onChanged={onChanged}
          sort={sort}
          onSortChange={setSort}
          chatLayout
          controlsAtBottom
        />
      </div>
      <div className="shrink-0 border-t border-slate-200 bg-white/80 px-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))] pt-3 backdrop-blur dark:border-slate-800 dark:bg-slate-900/80 sm:px-5">
        <div className="mx-auto max-w-3xl"><Composer onSent={onChanged} /></div>
      </div>
      <FilePreview file={preview} onClose={() => setPreview(null)} />
    </div>
  );
}
