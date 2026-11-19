import { expect, test } from '@playwright/test';
import { attachment, attachment as img, jsonBody, message, mockApi, seedSession } from './helpers';

test.describe('navigation', () => {
  test('moves between the primary sections', async ({ page }) => {
    await seedSession(page);
    await mockApi(page);
    await page.goto('/');

    await expect(page.getByRole('navigation', { name: 'Home' }).first()).toBeVisible();
    await page.getByRole('link', { name: 'Files', exact: true }).first().click();
    await expect(page).toHaveURL(/\/files$/);
    await expect(page.getByPlaceholder(/Search file names/)).toBeVisible();

    await page.getByRole('link', { name: 'Search', exact: true }).first().click();
    await expect(page).toHaveURL(/\/search$/);
    await expect(page.getByPlaceholder(/Search messages or file names/)).toBeVisible();
  });
});

test.describe('message flow', () => {
  test('renders messages, favorites them and sends a new one', async ({ page }) => {
    let current = message({ id: 'm1', text: 'hello from phone' });
    let patched: Record<string, unknown> | null = null;
    let created: Record<string, unknown> | null = null;
    await seedSession(page);
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: [current], nextCursor: null }) },
      {
        method: 'PATCH', path: /^\/api\/messages\/m1$/,
        body: (req) => {
          patched = jsonBody(req);
          current = { ...current, ...patched };
          return current;
        },
      },
      {
        method: 'POST', path: /^\/api\/messages$/,
        body: (req) => {
          created = jsonBody(req);
          return message({ id: 'm2', text: String(created.text ?? '') });
        },
      },
    ]);

    await page.goto('/');
    await expect(page.getByText('hello from phone')).toBeVisible();
    await page.getByRole('button', { name: 'Favorite', exact: true }).first().click();
    await expect.poll(() => patched).not.toBeNull();
    expect(patched).toMatchObject({ favorited: true });

    const composer = page.getByRole('textbox', { name: /Type a message/ });
    await composer.fill('a second message');
    await page.getByRole('button', { name: 'Send' }).click();
    await expect.poll(() => created).not.toBeNull();
    expect(created).toMatchObject({ text: 'a second message' });
  });
});

test.describe('search', () => {
  test('returns results, exposes size filters and supports selection', async ({ page }) => {
    let seenQuery = '';
    await seedSession(page);
    await mockApi(page, [
      {
        method: 'GET', path: /^\/api\/search$/,
        body: (_req, url) => {
          seenQuery = url.searchParams.get('q') ?? '';
          return {
            items: [{ kind: 'file', id: 'f1', score: 1, file: attachment({ name: 'report.pdf', type: 'pdf' }) }],
            nextCursor: null,
          };
        },
      },
    ]);

    await page.goto('/search');
    await expect(page.getByLabel('Min size')).toBeVisible();
    await expect(page.getByLabel('Max size')).toBeVisible();
    await page.getByPlaceholder(/Search messages or file names/).fill('report');
    await expect(page.getByText('report.pdf')).toBeVisible();
    expect(seenQuery).toBe('report');

    await page.getByRole('button', { name: 'Selection mode' }).click();
    await expect(page.getByRole('button', { name: 'Select all' })).toBeVisible();
    await page.getByRole('button', { name: 'Select all' }).click();
    await expect(page.getByText('1 selected')).toBeVisible();
  });
});

test.describe('upload interaction', () => {
  test('queues a file and finishes the chunked upload', async ({ page }) => {
    let initBody: Record<string, unknown> | null = null;
    await seedSession(page);
    await mockApi(page, [
      {
        method: 'POST', path: /^\/api\/uploads\/init$/,
        body: (req) => {
          initBody = jsonBody(req);
          return { uploadId: 'u1', uploadedChunks: [] };
        },
      },
      { method: 'PUT', path: /^\/api\/uploads\/u1\/chunk\/\d+$/, body: { received: true } },
      { method: 'POST', path: /^\/api\/uploads\/u1\/complete$/, body: { attachmentId: 'a1' } },
    ]);

    await page.goto('/');
    await page.getByRole('button', { name: 'Choose files' }).click();
    await page.locator('input[type=file]').first().setInputFiles({
      name: 'note.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('hello upload'),
    });

    await expect(page.getByText('note.txt').first()).toBeVisible();
    await expect.poll(() => initBody).not.toBeNull();
    expect(initBody).toMatchObject({ name: 'note.txt', totalChunks: 1 });
    await expect(page.getByText('Done').first()).toBeVisible({ timeout: 12_000 });
  });
});

test.describe('image preview and lightbox', () => {
  test('opens the gallery from a message and closes it with Escape', async ({ page }) => {
    const image = img({ id: 'a1', name: 'photo.png', type: 'image' });
    await seedSession(page);
    await mockApi(page, [
      { method: 'GET', path: /^\/api\/messages$/, body: () => ({ items: [message({ attachments: [image] })], nextCursor: null }) },
    ]);

    await page.goto('/');
    await page.getByRole('button', { name: 'photo.png' }).click();
    const dialog = page.getByRole('dialog', { name: 'Messages' });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('img', { name: 'photo.png' })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
  });
});
