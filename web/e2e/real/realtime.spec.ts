import { expect, test } from '@playwright/test';
import { ensureAdmin, injectSession, login } from './helpers';

test('a message sent on device A appears immediately on device B', async ({ browser }) => {
  const admin = await ensureAdmin();
  const second = await login('E2E Second Device');

  const ctxA = await browser.newContext();
  const ctxB = await browser.newContext();
  // Distinct device keys are essential: with approval policy a reused key would
  // be treated as the same device. The instance uses policy=open.
  await injectSession(ctxA, admin);
  await injectSession(ctxB, second);

  const a = await ctxA.newPage();
  const b = await ctxB.newPage();
  await a.goto('/');
  await b.goto('/');
  await expect(a.getByText('发送', { exact: true })).toBeVisible();
  await expect(b.getByText('发送', { exact: true })).toBeVisible();

  const text = `realtime-${Date.now()}`;
  await a.locator('textarea').first().fill(text);
  await a.getByRole('button', { name: '发送', exact: true }).click();

  // Device B receives the message over WebSocket without a reload.
  await expect(b.getByText(text)).toBeVisible({ timeout: 10_000 });
  await ctxA.close();
  await ctxB.close();
});
