import { expect, test } from '@playwright/test';
import { api, ensureAdmin, injectSession } from './helpers';

test('registers a passkey with a virtual authenticator and logs in with it', async ({ browser }) => {
  const admin = await ensureAdmin();
  const context = await browser.newContext();
  const page = await context.newPage();
  await injectSession(context, admin);

  const cdp = await context.newCDPSession(page);
  await cdp.send('WebAuthn.enable');
  await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      transport: 'internal',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });

  await page.goto('/admin');
  await expect(page.getByRole('heading', { name: '账号与安全' }).first()).toBeVisible();
  await page.getByRole('button', { name: '注册通行密钥' }).click();
  // Wait for the registration ceremony to finish on the backend.
  await expect.poll(async () => {
    const me = await api('/api/auth/me', { headers: { Authorization: `Bearer ${admin.token}` } });
    const user = me.user as { hasPasskey?: boolean } | undefined;
    return Boolean(user?.hasPasskey);
  }).toBe(true);

  // Log out in-browser, then sign in using only the passkey.
  await page.evaluate(() => {
    sessionStorage.setItem('pd.no-restore', '1');
    localStorage.clear();
  });
  await page.goto('/login');
  await page.getByRole('button', { name: '通行密钥登录' }).click();
  await expect(page.getByText('打开管理界面')).toBeVisible({ timeout: 15_000 });
  await context.close();
});
