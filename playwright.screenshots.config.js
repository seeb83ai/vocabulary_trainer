// @ts-check
// Separate, on-demand Playwright config for regenerating the README
// screenshots (`make screenshots-readme`). Deliberately not merged into
// playwright.config.js so the default `npx playwright test` / CI run never
// picks up e2e-screenshots/capture.spec.js.
//
// When USE_LOCAL_SERVER=1: logs in to the already-running local server as the
// user given by LOCAL_USER_EMAIL / LOCAL_USER_PASSWORD / LOCAL_SERVER_URL.
import { defineConfig, devices } from '@playwright/test';
import { existsSync, readFileSync } from 'fs';
import { join } from 'path';

const PREINSTALLED_CHROMIUM = '/opt/pw-browsers/chromium';
const chromiumExecutable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ||
  (existsSync(PREINSTALLED_CHROMIUM) ? PREINSTALLED_CHROMIUM : undefined);

const useLocal = process.env.USE_LOCAL_SERVER === '1';

function resolveBaseURL() {
  if (!useLocal) return 'http://localhost:18080';
  const cached = join('e2e/.auth', 'local-base-url.txt');
  if (existsSync(cached)) return readFileSync(cached, 'utf8').trim();
  return process.env.LOCAL_SERVER_URL || 'http://localhost:8080';
}

export default defineConfig({
  testDir: './e2e-screenshots',
  globalSetup: useLocal ? './e2e/global-setup-local.js' : './e2e/global-setup.js',
  globalTeardown: useLocal ? './e2e/global-teardown-local.js' : './e2e/global-teardown.js',

  workers: 1,
  retries: 0,
  timeout: 30_000,

  use: {
    baseURL: resolveBaseURL(),
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
