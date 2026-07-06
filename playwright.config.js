// @ts-check
import { defineConfig, devices } from '@playwright/test';
import { existsSync } from 'fs';

const PREINSTALLED_CHROMIUM = '/opt/pw-browsers/chromium';
const chromiumExecutable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ||
  (existsSync(PREINSTALLED_CHROMIUM) ? PREINSTALLED_CHROMIUM : undefined);

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/global-setup.js',
  globalTeardown: './e2e/global-teardown.js',

  // Run tests sequentially — the E2E server uses a single SQLite file
  workers: 1,
  retries: 0,
  timeout: 30_000,

  use: {
    baseURL: 'http://localhost:18080',
    headless: true,
    // Capture traces on failure for easier debugging
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        ...(chromiumExecutable ? { launchOptions: { executablePath: chromiumExecutable } } : {}),
      },
    },
  ],

  // Output folders
  outputDir: 'playwright-report/results',
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report/html', open: 'never' }],
  ],
});
