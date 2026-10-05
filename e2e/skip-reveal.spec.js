// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';

// Issue #536: with "Show the answer when I skip" on, "Skip for today" shows
// the answer of the skipped card before the next card.
//
// Each test registers a fresh user with one word, because a skip moves the
// word's due date and would change the card order for the shared users.
async function setupUser(page, settings) {
  const email = `e2e-skip-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'SkipTest123!' } });
  expect(regRes.ok()).toBeTruthy();
  const wordRes = await page.request.post('/api/words', {
    data: { zh_text: '水', pinyin: 'shuǐ', translations: { en: ['water'] }, tags: [], start_training: true },
  });
  expect(wordRes.ok()).toBeTruthy();

  if (settings) {
    const current = await (await page.request.get('/api/settings')).json();
    const patchRes = await page.request.patch('/api/settings', { data: { ...current, ...settings } });
    expect(patchRes.ok()).toBeTruthy();
  }

  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
  });
  await syncNewWordMode(page, 'zh_to_transl');
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
}

test.describe('Skip for today – show the answer (issue #536)', () => {
  test('shows the answer of the skipped word when the setting is on', async ({ page }) => {
    await setupUser(page, { skip_reveal_answer: true });
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText('水');

    await page.locator('#skip-today-btn').click();

    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-icon')).toHaveText('Skipped');
    await expect(page.locator('#word-breakdown')).toContainText('水');
    await expect(page.locator('#word-breakdown')).toContainText('shuǐ');
    await expect(page.locator('#word-breakdown')).toContainText('water');
    await expect(page.locator('#next-due-info')).toHaveText('Next review tomorrow');
    await expect(page.locator('#stats-due')).toHaveText('0');
    await captureForPR(page, 'train-skip-reveal');

    await page.locator('#next-btn').click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });

    // The skip moved the word to tomorrow; it did not count as an attempt.
    const stats = await (await page.request.get('/api/quiz/stats')).json();
    expect(stats.due_today).toBe(0);
  });

  test('goes straight to the next card when the setting is off (default)', async ({ page }) => {
    await setupUser(page, null);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#skip-today-btn').click();

    await expect(page.locator('#card-area')).not.toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-area')).not.toBeVisible();
  });

  test('the setting is off by default and persists when turned on', async ({ page }) => {
    await setupUser(page, null);
    await page.goto('/settings');
    const toggle = page.locator('#skip-reveal-answer');
    await expect(toggle).not.toBeChecked();

    await toggle.check();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
    await toggle.locator('xpath=ancestor::div[contains(@class,"st-row")]').scrollIntoViewIfNeeded();
    await captureForPR(page, 'settings-skip-reveal');

    await page.reload();
    await expect(page.locator('#skip-reveal-answer')).toBeChecked();
    const settings = await (await page.request.get('/api/settings')).json();
    expect(settings.skip_reveal_answer).toBe(true);
  });
});
