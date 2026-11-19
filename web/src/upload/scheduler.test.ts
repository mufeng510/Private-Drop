import { describe, expect, it, vi } from 'vitest';
import { runWithConcurrency, splitChunks } from './scheduler';

describe('runWithConcurrency', () => {
  it('processes every item exactly once', async () => {
    const seen: number[] = [];
    await runWithConcurrency([1, 2, 3, 4, 5], 2, async (item) => {
      seen.push(item);
    });
    expect(seen.sort()).toEqual([1, 2, 3, 4, 5]);
  });

  it('never exceeds the concurrency limit', async () => {
    let active = 0;
    let peak = 0;
    await runWithConcurrency(Array.from({ length: 20 }, (_, i) => i), 3, async () => {
      active++;
      peak = Math.max(peak, active);
      await Promise.resolve();
      active--;
    });
    expect(peak).toBe(3);
  });

  it('stops claiming new items when shouldStop is true', async () => {
    let claimed = 0;
    let stop = false;
    await runWithConcurrency(
      Array.from({ length: 10 }, (_, i) => i),
      1,
      async () => {
        claimed++;
        if (claimed === 2) stop = true;
      },
      () => stop,
    );
    expect(claimed).toBeLessThanOrEqual(3);
  });

  it('surfaces a worker error after siblings settle', async () => {
    const err = new Error('chunk failed');
    await expect(
      runWithConcurrency([1, 2, 3], 2, async (item) => {
        if (item === 2) throw err;
      }),
    ).rejects.toBe(err);
  });

  it('resolves immediately for an empty list', async () => {
    const worker = vi.fn();
    await runWithConcurrency([], 3, worker);
    expect(worker).not.toHaveBeenCalled();
  });
});

describe('splitChunks', () => {
  it('requires a positive chunk size', () => {
    expect(() => splitChunks(10, 0)).toThrow();
  });
});
