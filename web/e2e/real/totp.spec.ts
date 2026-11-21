import { expect, test } from '@playwright/test';
import { api, ensureAdmin, injectSession, PASSWORD, totpCode, USERNAME } from './helpers';

test('enables TOTP in the browser and logs in with a generated code', async ({ browser }) => {
  const admin = await ensureAdmin();
  // TOTP setup is auth-only, so we can obtain a secret even after a passkey
  // exists. The UI generates its own secret on click; this one opens the gate.
  const gate = await api('/api/auth/totp/setup', {
    method: 'POST',
    headers: { Authorization: `Bearer ${admin.token}` },
  });

  const context = await browser.newContext();
  const page = await context.newPage();
  await injectSession(context, admin);
  await page.goto('/admin');
  // Depending on test order the stored MFA grant may be stale (e.g. a passkey
  // was registered after it was issued, which revokes it server-side). The gate
  // then correctly asks for a second factor. Enable this pending TOTP secret
  // first so the code we type is a valid one, then unlock the gate with it.
  if (await page.getByRole('button', { name: '验证并进入' }).isVisible()) {
    await api('/api/auth/totp/enable', {
      method: 'POST',
      headers: { Authorization: `Bearer ${admin.token}` },
      body: JSON.stringify({ code: totpCode(String(gate.secret)) }),
    });
    await page.locator('input[inputmode="numeric"]').first().fill(totpCode(String(gate.secret)));
    await page.getByRole('button', { name: '验证并进入' }).click();
  }
  await expect(page.getByRole('button', { name: '设置动态验证码' })).toBeVisible({ timeout: 15_000 });

  await page.getByRole('button', { name: '设置动态验证码' }).click();
  const secret = (await page.locator('code').first().innerText()).trim();
  expect(secret.length).toBeGreaterThan(10);
  await page.locator('input[inputmode="numeric"]').first().fill(totpCode(secret));
  await page.getByRole('button', { name: '启用 TOTP' }).click();
  await expect.poll(async () => {
    const me = await api('/api/auth/me', { headers: { Authorization: `Bearer ${admin.token}` } });
    const user = me.user as { hasTotp?: boolean } | undefined;
    return Boolean(user?.hasTotp);
  }).toBe(true);

  // Full browser login with a real TOTP code.
  await page.evaluate(() => {
    sessionStorage.setItem('pd.no-restore', '1');
    localStorage.clear();
  });
  await page.goto('/login');
  await page.getByLabel('用户名').fill(USERNAME);
  await page.getByLabel('密码').fill(PASSWORD);
  // First submit reveals the TOTP field (mfaRequired="totp").
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.locator('input[inputmode="numeric"]').first()).toBeVisible({ timeout: 10_000 });
  await page.locator('input[inputmode="numeric"]').first().fill(totpCode(secret));
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await expect(page.getByText('打开管理界面')).toBeVisible({ timeout: 15_000 });
  await context.close();
});
