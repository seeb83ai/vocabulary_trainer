// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';
import { seedReviewWord } from './helpers/db.js';

// Lapsed-words baseline: new words pause while too many words failed their
// last review. The stats bar "!" says why. On by default with limit 10.

async function registerUser(page) {
  const email = `e2e-lapsed-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'LapsedTest123!' } });
  expect(regRes.ok()).toBeTruthy();
  return email;
}

async function seed(page, zh, en, startTraining) {
  const res = await page.request.post('/api/words', {
    data: { zh_text: zh, translations: { en: [en] }, tags: [], start_training: startTraining },
  });
  expect(res.ok()).toBeTruthy();
}

test.describe('Lapsed-words baseline', () => {
  test('stats bar says new words are paused because of lapsed words', async ({ page }) => {
    const email = await registerUser(page);
    await seed(page, '水', 'water', true);
    await seed(page, '山', 'mountain', true);
    await seed(page, '火', 'fire', false);
    seedReviewWord(email, '水', 1);
    seedReviewWord(email, '山', 1);
    const st = await (await page.request.get('/api/settings')).json();
    const patch = await page.request.patch('/api/settings', { data: { ...st, baseline_lapsed_value: 2 } });
    expect(patch.ok()).toBeTruthy();
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: false, components: false, tags: [] },
    });
    await syncNewWordMode(page, 'zh_to_transl');

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    // The unseen word is not introduced; a lapsed review word comes instead.
    await expect(page.locator('#prompt-word')).not.toHaveText('火');
    const hint = page.locator('#stats-new-paused-btn');
    await expect(hint).toBeVisible();
    await hint.click();
    const notice = page.locator('#stats-new-paused');
    await expect(notice).toBeVisible();
    await expect(notice).toHaveText('New words paused: 2 words failed their last review (limit 2)');
    await captureForPR(page, 'train-lapsed-paused');
  });

  test('the baseline is on with limit 10 in the Daily Learning settings', async ({ page }) => {
    await registerUser(page);
    await page.goto('/settings');
    const toggle = page.locator('#baseline-lapsed-enabled');
    const value = page.locator('#baseline-lapsed-value');
    await expect(toggle).toBeChecked();
    await expect(value).toHaveValue('10');
    await value.scrollIntoViewIfNeeded();
    await captureForPR(page, 'settings-lapsed-baseline');
    await value.fill('7');
    await expect.poll(async () => (await (await page.request.get('/api/settings')).json()).baseline_lapsed_value).toBe(7);
  });
});
