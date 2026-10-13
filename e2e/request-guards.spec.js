// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';

// Request guards (follow-up to issue #563): every client request has a
// timeout, an action can't run twice while its request is open, and a reload
// started by a filter change cancels the older one, so nothing renders twice
// or out of order. Timeouts use Playwright's fake clock, so a 15 s timeout
// costs no real time. Every test registers its own user.

const PASSWORD = 'E2eRequestGuards123!';
const TIMEOUT_TEXT = 'The server did not answer. Try again.';

async function registerUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
  const email = `e2e-guards-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
}

async function seed(page, zh, pinyin, en, startTraining = true) {
  const res = await page.request.post('/api/words', {
    data: { zh_text: zh, pinyin, translations: { en }, tags: [], start_training: startTraining },
  });
  expect(res.ok()).toBe(true);
  return (await res.json()).id;
}

async function useMode(page, mode) {
  await page.request.patch('/api/training-filters', {
    data: { mode, langs: ['en'], bucket: '', mnemonics: true, components: false, tags: [] },
  });
  await syncNewWordMode(page, mode);
}

// Counts requests to a URL pattern and holds each one for delayMs before it
// goes to the server.
async function countAndDelay(page, pattern, delayMs) {
  const counter = { n: 0 };
  await page.route(pattern, async route => {
    counter.n++;
    await new Promise(r => setTimeout(r, delayMs));
    await route.continue();
  });
  return counter;
}

// A request that never answers. The page's own timeout must end it.
async function hang(page, pattern) {
  await page.route(pattern, () => {});
}

test.describe('Request guards – Train', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('fast filter changes show only the newest card', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await seed(page, '再见', 'zài jiàn', ['bye']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // The older request (mnemonics off) answers late; the newer one at once.
    await page.route('**/api/quiz/next**', async route => {
      const slow = route.request().url().includes('mnemonics=false');
      const res = await route.fetch();
      const card = await res.json();
      if (slow) await new Promise(r => setTimeout(r, 1_500));
      await route.fulfill({ response: res, json: { ...card, prompt: slow ? '旧旧' : '新新' } });
    });
    await page.locator('#open-filter-overlay').click();
    await page.locator('#overlay-mnemonics-pill').click();
    await page.locator('#overlay-mnemonics-pill').click();
    await page.locator('#filter-overlay-close').click();

    await expect(page.locator('#prompt-word')).toHaveText('新新', { timeout: 8_000 });
    await page.waitForTimeout(2_000);
    await expect(page.locator('#prompt-word')).toHaveText('新新');
  });

  test('a double tap on Skip sends one request', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await seed(page, '再见', 'zài jiàn', ['bye']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    const skips = await countAndDelay(page, '**/api/quiz/skip', 600);
    await page.locator('#skip-today-btn').dblclick();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    await page.waitForTimeout(800);
    expect(skips.n).toBe(1);
  });

  test('a double tap on a new word\'s Skip sends one request', async ({ page }) => {
    await registerUser(page);
    await seed(page, '年', 'nián', ['year'], false);
    await page.request.patch('/api/training-filters', {
      data: { mode: 'progressive', langs: ['en'], bucket: '', mnemonics: true, components: false, tags: [] },
    });
    await page.goto('/train');
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 12_000 });

    const skips = await countAndDelay(page, '**/api/quiz/skip', 600);
    await page.locator('#new-word-skip-btn').dblclick();
    await page.waitForTimeout(1_500);
    expect(skips.n).toBe(1);
  });

  test('an action that does not answer shows a toast and can be tried again', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'zh_to_transl');
    await page.clock.install();
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await hang(page, '**/api/quiz/skip');
    await page.locator('#skip-today-btn').click();
    await expect(page.locator('#skip-today-btn')).toBeDisabled();
    await page.clock.fastForward(16_000);

    const toast = page.locator('[data-testid="toast"]');
    await expect(toast).toBeVisible();
    await expect(toast).toContainText(TIMEOUT_TEXT);
    await expect(page.locator('#skip-today-btn')).toBeEnabled();
    await captureForPR(page, 'request-guards-timeout-toast');
  });

  test('a card load that does not answer shows the error card', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'zh_to_transl');
    await page.clock.install();
    await hang(page, '**/api/quiz/next**');
    await page.goto('/train');

    await page.waitForTimeout(500);
    await page.clock.fastForward(16_000);
    await expect(page.locator('#error-state')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#error-msg')).toHaveText(TIMEOUT_TEXT);
  });
});
