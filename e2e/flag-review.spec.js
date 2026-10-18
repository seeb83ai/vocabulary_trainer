// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';
import { seedReviewWord } from './helpers/db.js';

// Issue #585: the result screen shows when a word is already flagged for
// review, and a click on the flagged button removes the flag.

async function setup(page) {
  const email = `e2e-flag-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'FlagReview123!' } });
  expect(regRes.ok()).toBeTruthy();
  const res = await page.request.post('/api/words', {
    data: { zh_text: '关于', pinyin: 'guān yú', translations: { en: ['about'] }, tags: [], start_training: true },
  });
  expect(res.ok()).toBeTruthy();
  const word = await res.json();
  seedReviewWord(email, '关于', 0);
  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: false, components: false, tags: [] },
  });
  await syncNewWordMode(page, 'zh_to_transl');
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  return word.id;
}

async function answer(page, text) {
  await page.goto('/train');
  await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
  await expect(page.locator('#prompt-word')).toHaveText('关于');
  await page.locator('#answer-input').fill(text);
  await page.locator('#answer-form button[type="submit"]').click();
  await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
}

async function needsReview(page, id) {
  const res = await page.request.get(`/api/words/${id}`);
  expect(res.ok()).toBeTruthy();
  return (await res.json()).needs_review;
}

test.describe('Flag for review on the result screen', () => {
  test('an already flagged word shows the flagged button, and a click removes the flag', async ({ page }) => {
    const id = await setup(page);
    const flagRes = await page.request.post(`/api/words/${id}/review`);
    expect(flagRes.ok()).toBeTruthy();

    await answer(page, 'about');
    const btn = page.locator('#needs-review-btn');
    await expect(btn).toHaveClass(/is-flagged/);
    await expect(btn).toHaveAttribute('aria-pressed', 'true');
    await expect(btn).toContainText('Flagged');
    await expect(btn).toHaveCSS('border-color', 'rgb(253, 186, 116)');
    await captureForPR(page, 'train-flag-review-flagged');

    await btn.click();
    await expect(btn).not.toHaveClass(/is-flagged/);
    await expect(btn).toHaveAttribute('aria-pressed', 'false');
    await expect(btn).toContainText('Flag for review');
    await expect(btn).toBeEnabled();
    expect(await needsReview(page, id)).toBe(false);
    // Let the colour transition end, so the screenshot shows the plain button.
    await page.mouse.move(0, 0);
    await expect(btn).toHaveCSS('border-color', 'rgb(209, 213, 219)');
    await captureForPR(page, 'train-flag-review-unflagged');
  });

  test('a word that is not flagged can be flagged and unflagged again', async ({ page }) => {
    const id = await setup(page);

    await answer(page, 'about');
    const btn = page.locator('#needs-review-btn');
    await expect(btn).not.toHaveClass(/is-flagged/);
    await expect(btn).toHaveAttribute('aria-pressed', 'false');

    await btn.click();
    await expect(btn).toHaveClass(/is-flagged/);
    await expect(btn).toBeEnabled();
    expect(await needsReview(page, id)).toBe(true);

    await btn.click();
    await expect(btn).not.toHaveClass(/is-flagged/);
    expect(await needsReview(page, id)).toBe(false);
  });
});
