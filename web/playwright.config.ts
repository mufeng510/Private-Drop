import { defineConfig, devices } from '@playwright/test';

/**
 * E2E runs against a preview of the built dist (so the real PWA manifest and
 * custom service worker are present). System Chrome is used via `channel`:
 * Playwright browsers are NOT downloaded on this machine.
 * All API calls are mocked with page.route, so no backend is required.
 */
export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  expect: { timeout: 7_000 },
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:4174',
    channel: 'chrome',
    viewport: { width: 1280, height: 900 },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'system-chrome', use: { ...devices['Desktop Chrome'], channel: 'chrome' } },
  ],
  webServer: {
    command: 'npm run build && npm run preview -- --port 4174 --host 127.0.0.1',
    url: 'http://127.0.0.1:4174',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
