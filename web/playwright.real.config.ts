import { defineConfig, devices } from '@playwright/test';

/**
 * E2E against a REAL running backend. Start the server first:
 *   PORT=18081 DATA_DIR=/tmp/pd-e2e-real go run ./cmd/server
 * Then: npm run test:e2e:real
 *
 * Uses the system Chrome channel (no browser download) and localhost as the
 * WebAuthn RP ID, since IP literals are not valid relying-party identifiers.
 */
export default defineConfig({
  testDir: './e2e/real',
  timeout: 90_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: 'list',
  use: {
    baseURL: process.env.REAL_BASE_URL || 'http://localhost:18081',
    channel: 'chrome',
    locale: 'zh-CN',
    trace: 'retain-on-failure',
    ...devices['Desktop Chrome'],
  },
});
