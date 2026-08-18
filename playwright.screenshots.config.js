// @ts-check
// Separate, on-demand Playwright config for regenerating the README
// screenshots (`make screenshots`). Deliberately not merged into
// playwright.config.js so the default `npx playwright test` / CI run never
// picks up e2e-screenshots/capture.spec.js.
import { defineConfig, devices } from '@playwright/test';
import { existsSync } from 'fs';

const PREINSTALLED_CHROMIUM = '/opt/pw-browsers/chromium';
const chromiumExecutable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ||
  (existsSync(PREINSTALLED_CHROMIUM) ? PREINSTALLED_CHROMIUM : undefined);

export default defineConfig({
  testDir: './e2e-screenshots',
  globalSetup: './e2e/global-setup.js',
  globalTeardown: './e2e/global-teardown.js',

  // Reuses the same single-worker E2E server as playwright.config.js.
  workers: 1,
  retries: 0,
  timeout: 30_000,

  use: {
    baseURL: 'http://localhost:18080',
    headless: true,
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 2,
    reducedMotion: 'reduce',
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

  reporter: [['list']],
});
