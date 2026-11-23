import { expect, test } from '@playwright/test';
import { message, mockApi, seedSession } from './helpers';

const streamMessages = () => [
  message({ id: 'm3', text: 'third newest', createdAt: '2026-06-03T10:00:00.000Z', updatedAt: '2026-06-03T10:00:00.000Z' }),
  message({ id: 'm2', text: 'second', createdAt: '2026-06-02T10:00:00.000Z', updatedAt: '2026-06-02T10:00:00.000Z' }),
  message({ id: 'm1', text: 'first oldest', createdAt: '2026-06-01T10:00:00.000Z', updatedAt: '2026-06-01T10:00:00.000Z' }),
];

test.describe('chat-style message flow', () => {
  test('shows the newest message at the bottom and the controls above the composer', async ({ page }) => {
    await seedSession(page);
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: streamMessages(), nextCursor: null }) },
    ]);
    await page.goto('/');

    const cards = page.locator('main article');
    await expect(cards).toHaveCount(3);
    await expect(cards.nth(0)).toContainText('first oldest');
    await expect(cards.nth(2)).toContainText('third newest');

    const sort = page.getByRole('button', { name: 'Newest first' });
    const composer = page.getByRole('textbox', { name: /Type a message/ });
    await expect(sort).toBeVisible();
    await expect(composer).toBeVisible();
    const order = await page.evaluate(() => {
      const sortEl = [...document.querySelectorAll('button')].find((b) => b.textContent === 'Newest first')!;
      const lastCard = [...document.querySelectorAll('main article')].at(-1)!;
      const box = document.querySelector('main textarea')!;
      return { sortAfterCards: sortEl.compareDocumentPosition(lastCard) & Node.DOCUMENT_POSITION_PRECEDING, sortBeforeComposer: sortEl.compareDocumentPosition(box) & Node.DOCUMENT_POSITION_FOLLOWING };
    });
    expect(order.sortAfterCards).toBeTruthy();
    expect(order.sortBeforeComposer).toBeTruthy();
  });

  test('no bottom navigation bar on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await seedSession(page);
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: streamMessages(), nextCursor: null }) },
    ]);
    await page.goto('/');

    await expect(page.getByRole('navigation', { name: 'Home' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Menu' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Settings', exact: true })).toHaveCount(0);
  });

  test('puts the end-of-history marker above the messages in chat layout', async ({ page }) => {
    await seedSession(page);
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: streamMessages(), nextCursor: null }) },
    ]);
    await page.goto('/');

    const marker = page.getByText('No older messages');
    await expect(marker).toBeVisible();
    const beforeFirstCard = await page.evaluate(() => {
      const el = [...document.querySelectorAll('main p')].find((p) => p.textContent?.includes('No older messages'))!;
      const firstCard = document.querySelector('main article')!;
      return Boolean(el.compareDocumentPosition(firstCard) & Node.DOCUMENT_POSITION_FOLLOWING);
    });
    expect(beforeFirstCard).toBe(true);
  });

  test('keeps the selection toggle compact on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await seedSession(page);
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: streamMessages(), nextCursor: null }) },
    ]);
    await page.goto('/');

    const toggle = page.getByRole('button', { name: 'Selection mode' });
    await expect(toggle).toBeVisible();
    const box = await toggle.boundingBox();
    expect(box?.width).toBe(36);
    expect(box?.height).toBe(36);

    await toggle.click();
    await expect(page.getByRole('button', { name: 'Exit selection' })).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByRole('button', { name: 'Select all' })).toBeVisible();
  });
});

test.describe('tag button layout', () => {
  test('the tag count badge does not enlarge the compact tag button', async ({ page }) => {
    const tags = [
      { id: 't1', name: 'Work', color: '#2563eb', icon: '', sort: 0, isSystem: false },
      { id: 't2', name: 'Urgent', color: '#dc2626', icon: '', sort: 1, isSystem: false },
    ];
    const tagged = message({ text: 'tagged message', tags });
    await seedSession(page);
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: [tagged], nextCursor: null }) },
      { method: 'GET', path: /^\/api\/tags$/, body: { items: tags } },
    ]);
    await page.goto('/');

    const tagButton = page.locator('main article button').filter({ hasText: /^2$/ });
    const copyButton = page.getByRole('button', { name: 'Copy text' });
    await expect(tagButton).toBeVisible();
    const tagBox = await tagButton.boundingBox();
    const copyBox = await copyButton.boundingBox();
    // The count badge is absolutely positioned: the tag button stays the same
    // 36px square as the other icon buttons instead of growing a second line.
    expect(tagBox?.width).toBe(copyBox?.width);
    expect(tagBox?.height).toBe(copyBox?.height);
  });
});

test.describe('copy message text', () => {
  test('copies via the Clipboard API and shows feedback', async ({ context, page }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await seedSession(page);
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: [message({ text: 'copy me please' })], nextCursor: null }) },
    ]);
    await page.goto('/');

    await page.getByRole('button', { name: 'Copy text' }).click();
    await expect(page.getByText('Copied', { exact: true })).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('copy me please');
  });

  test('falls back to execCommand when the Clipboard API is unavailable', async ({ page }) => {
    await seedSession(page);
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true });
    });
    let copied = '';
    await page.addInitScript(() => {
      document.execCommand = ((command: string) => {
        if (command === 'copy') {
          const ta = document.querySelector('textarea[readonly]') as HTMLTextAreaElement | null;
          (window as unknown as { __copied: string }).__copied = ta?.value ?? '';
        }
        return true;
      }) as typeof document.execCommand;
    });
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: [message({ text: 'fallback text' })], nextCursor: null }) },
    ]);
    await page.goto('/');

    await page.getByRole('button', { name: 'Copy text' }).click();
    await expect(page.getByText('Copied', { exact: true })).toBeVisible();
    copied = await page.evaluate(() => (window as unknown as { __copied: string }).__copied);
    expect(copied).toBe('fallback text');
  });
});
