import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './midscene/tests',
  timeout: 90_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
    ['@midscene/web/playwright-reporter'],
  ],
  use: {
    baseURL: process.env.SMARA_WEB_URL ?? 'http://localhost:5173',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
