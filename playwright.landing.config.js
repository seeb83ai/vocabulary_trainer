// @ts-check
// Separate, on-demand Playwright config for regenerating the landing-page
// teaser images (`make screenshots-landing`). Deliberately not merged into
// playwright.config.js so the default `npx playwright test` / CI run never
// picks up e2e-screenshots/landing.spec.js.
//
// Always runs against a fresh temp server: the spec seeds demo data straight
// into that server's SQLite file, so it must never target a real instance.
import { defineConfig, devices } from '@playwright/test';
import { existsSync } from 'fs';

const PREINSTALLED_CHROMIUM = '/opt/pw-browsers/chromium';
const chromiumExecutable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ||
  (existsSync(PREINSTALLED_CHROMIUM) ? PREINSTALLED_CHROMIUM : undefined);

export default defineConfig({
  testDir: './e2e-screenshots',
  testMatch: 'landing.spec.js',
  globalSetup: './e2e/global-setup.js',
  globalTeardown: './e2e/global-teardown.js',

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
