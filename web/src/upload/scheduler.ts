export interface ChunkRange {
  index: number;
  start: number;
  end: number;
}

/** Split a byte range into fixed-size chunk descriptors. Pure + testable. */
export function splitChunks(size: number, chunkSize: number): ChunkRange[] {
  if (chunkSize <= 0) throw new Error('chunkSize must be positive');
  const chunks: ChunkRange[] = [];
  for (let start = 0, index = 0; start < size; start += chunkSize, index++) {
    chunks.push({ index, start, end: Math.min(start + chunkSize, size) });
  }
  return chunks;
}

/**
 * Run `worker` over `items` with at most `limit` in flight. Rejections are
 * captured and surfaced after all started work settles; `shouldStop` lets a
 * cancelled task stop claiming new items. Order of execution is not guaranteed.
 */
export async function runWithConcurrency<T>(
  items: readonly T[],
  limit: number,
  worker: (item: T, index: number) => Promise<void>,
  shouldStop: () => boolean = () => false,
): Promise<void> {
  if (items.length === 0) return;
  const concurrency = Math.max(1, Math.min(limit, items.length));
  let cursor = 0;
  let firstError: unknown = null;

  const runner = async (): Promise<void> => {
    while (true) {
      if (shouldStop()) return;
      const index = cursor++;
      if (index >= items.length) return;
      try {
        await worker(items[index], index);
      } catch (e) {
        if (firstError === null) firstError = e;
        return; // this worker stops on failure; siblings finish their item
      }
    }
  };

  await Promise.all(Array.from({ length: concurrency }, () => runner()));
  if (firstError !== null) throw firstError;
}
