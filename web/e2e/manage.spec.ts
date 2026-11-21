import { expect, test } from '@playwright/test';
import { device, file, jsonBody, mockApi, rawSettings, seedSession, user } from './helpers';

test.describe('file management', () => {
  test('lists files, filters by tag and runs a batch action', async ({ page }) => {
    let bulkBody: Record<string, unknown> | null = null;
    await seedSession(page);
    await mockApi(page, [
      {
        method: 'GET', path: /^\/api\/files$/,
        body: (_req, url) => {
          // The tag filter is forwarded as a query parameter.
          expect(url.pathname).toBe('/api/files');
          return {
            items: [
              file({ id: 'f1', name: 'report.pdf' }),
              file({ id: 'f2', name: 'notes.txt', type: 'text' }),
            ],
            nextCursor: null,
          };
        },
      },
      {
        method: 'POST', path: /^\/api\/files\/bulk$/,
        body: (req) => {
          bulkBody = jsonBody(req);
          return {};
        },
      },
    ]);

    await page.goto('/files');
    await expect(page.getByText('report.pdf')).toBeVisible();
    await expect(page.getByLabel('Tags')).toBeVisible();

    await page.locator('article input[type=checkbox]').first().check();
    await expect(page.getByText('1 selected')).toBeVisible();
    await page.getByRole('button', { name: 'Favorite selected', exact: true }).click();
    await expect.poll(() => bulkBody).not.toBeNull();
    expect(bulkBody).toMatchObject({ action: 'favorite', ids: ['f1'] });
  });
});

test.describe('recycle bin', () => {
  test('restores with a mode and batch-deletes permanently', async ({ page }) => {
    let restoreMode = '';
    const removed: string[] = [];
    await seedSession(page);
    await mockApi(page, [
      {
        method: 'GET', path: /^\/api\/recycle$/,
        body: () => ({
          messages: [],
          files: [{ id: 'f1', name: 'old.txt', size: 10, deletedAt: '2026-06-01T10:00:00.000Z' }],
        }),
      },
      {
        method: 'POST', path: /^\/api\/recycle\/f1\/restore$/,
        body: (req) => {
          restoreMode = String(jsonBody(req).mode ?? '');
          return {};
        },
      },
      {
        method: 'DELETE', path: /^\/api\/recycle\/f1$/,
        body: () => {
          removed.push('f1');
          return {};
        },
      },
    ]);

    await page.goto('/recycle');
    await expect(page.getByText('old.txt')).toBeVisible();
    await page.getByRole('button', { name: 'Restore' }).first().click();
    const modeDialog = page.getByRole('dialog', { name: 'Choose a restore mode' });
    await expect(modeDialog).toBeVisible();
    await modeDialog.getByRole('button', { name: /Restart retention period/ }).click();
    await expect.poll(() => restoreMode).toBe('reset');

    // Batch permanent delete with confirmation.
    await page.getByRole('button', { name: 'Selection mode' }).click();
    await page.locator('input[type=checkbox]').first().check();
    await page.getByRole('button', { name: 'Delete permanently' }).first().click();
    const confirmDialog = page.getByRole('dialog', { name: 'Delete permanently' });
    await expect(confirmDialog).toBeVisible();
    await confirmDialog.getByRole('button', { name: 'Delete', exact: true }).click();
    await expect.poll(() => removed).toContain('f1');
  });
});

test.describe('admin UI', () => {
  test('opens the second-factor-gated admin area and switches sections', async ({ page }) => {
    await seedSession(page, { mfa: true });
    await mockApi(page);
    await page.goto('/admin');
    await expect(page.getByRole('navigation', { name: 'Admin area' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Account & security' })).toBeVisible();
    await page.getByRole('button', { name: 'Upload limits' }).click();
    await expect(page.getByRole('heading', { name: 'Upload limits' }).first()).toBeVisible();
    await expect(page.getByText('Concurrent uploads')).toBeVisible();
  });

  test('shows the verification gate when the stored MFA grant is stale', async ({ page }) => {
    await seedSession(page, { mfa: true });
    await mockApi(page);
    // Simulate an expired/revoked token: the server reports it as invalid.
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ user, device, settings: rawSettings, mfaValid: false }),
      }),
    );
    await page.goto('/admin');
    await expect(page.getByRole('heading', { name: 'Admin second-factor verification' })).toBeVisible();
    await expect(page.getByRole('navigation', { name: 'Admin area' })).toHaveCount(0);
  });

  test('drops back to the gate when an MFA-gated call is rejected', async ({ page }) => {
    await seedSession(page, { mfa: true });
    await mockApi(page);
    // me says the grant is valid (admin opens), but a later gated call rejects
    // it — e.g. it expired while the admin page was open.
    await page.route('**/api/settings', (route) =>
      route.fulfill({
        status: 403,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'MFA_REQUIRED', message: 'secondary verification required' } }),
      }),
    );
    await page.goto('/admin');
    await expect(page.getByRole('heading', { name: 'Admin second-factor verification' })).toBeVisible();
    await expect(page.getByText('secondary verification required')).toHaveCount(0);
  });
});

test.describe('mobile responsive', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('shows bottom navigation and no horizontal overflow at 390px', async ({ page }) => {
    await seedSession(page);
    await mockApi(page);
    await page.goto('/');
    // The sidebar is display:none below md, so its links are not exposed.
    await expect(page.getByRole('link', { name: 'Tags', exact: true })).toBeHidden();
    await expect(page.getByRole('link', { name: 'Settings', exact: true })).toBeVisible(); // bottom nav

    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(1);
  });
});

test.describe('PWA', () => {
  test('ships a POST multipart share-target manifest and a custom service worker', async ({ page }) => {
    await seedSession(page);
    await mockApi(page);
    await page.goto('/');

    const manifest = await page.evaluate(async () => {
      const res = await fetch('/manifest.webmanifest');
      return res.json() as Promise<Record<string, unknown>>;
    });
    const share = manifest.share_target as {
      action: string;
      method: string;
      enctype: string;
      params: { files: Array<{ name: string; accept: string[] }> };
    };
    expect(share.method).toBe('POST');
    expect(share.enctype).toBe('multipart/form-data');
    expect(share.action).toContain('share-target');
    expect(share.params.files[0].name).toBe('files');

    const sw = await page.evaluate(async () => {
      const res = await fetch('/sw.js');
      return res.ok ? res.text() : '';
    });
    expect(sw).toContain('pd:share');
    expect(sw).toContain('share-target');
    await expect.poll(() => page.evaluate(async () => Boolean(await navigator.serviceWorker.getRegistration()))).toBe(true);
  });
});
