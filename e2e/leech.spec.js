// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';
import { seedReviewWord, getWordProgress } from './helpers/db.js';

// Leech warning: a word that fails its review 5 days in a row (the default
// leech threshold) gets a warning box on the wrong-answer screen. The user
// keeps training it or puts it back to the unseen words (history erased).

async function setupLeech(page) {
  const email = `e2e-leech-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'LeechTest123!' } });
  expect(regRes.ok()).toBeTruthy();
  const res = await page.request.post('/api/words', {
    data: { zh_text: '记住', pinyin: 'jì zhu', translations: { en: ['remember'] }, tags: [], start_training: true },
  });
  expect(res.ok()).toBeTruthy();
  seedReviewWord(email, '记住', 4);
  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: false, components: false, tags: [] },
  });
  await syncNewWordMode(page, 'zh_to_transl');
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  return email;
}

async function answerWrong(page) {
  await page.goto('/train');
  await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
  await expect(page.locator('#prompt-word')).toHaveText('记住');
  await page.locator('#answer-input').fill('forget');
  await page.locator('#answer-form button[type="submit"]').click();
  await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
}

test.describe('Leech warning', () => {
  test('warns after 5 failed reviews in a row and puts the word back to unseen', async ({ page }) => {
    const email = await setupLeech(page);
    await answerWrong(page);

    const box = page.locator('#leech-box');
    await expect(box).toBeVisible();
    await expect(box).toContainText('You failed this word 5 reviews in a row');
    await expect(page.locator('#leech-keep-btn')).toBeVisible();
    await expect(page.locator('#leech-reset-btn')).toBeVisible();
    await captureForPR(page, 'train-leech-warning');

    await page.locator('#leech-reset-btn').click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });

    const p = getWordProgress(email, '记住');
    expect(p.first_seen_at).toBeNull();
    expect(p.lapses).toBe(0);
    expect(p.consecutive_lapses).toBe(0);
    const wrong = await (await page.request.get('/api/quiz/wrong-today?langs=en')).json();
    expect(JSON.stringify(wrong)).not.toContain('记住');
  });

  test('Keep training hides the warning and keeps the word', async ({ page }) => {
    const email = await setupLeech(page);
    await answerWrong(page);

    await expect(page.locator('#leech-box')).toBeVisible();
    await page.locator('#leech-keep-btn').click();
    await expect(page.locator('#leech-box')).not.toBeVisible();
    await expect(page.locator('#next-btn')).toBeVisible();

    const p = getWordProgress(email, '记住');
    expect(p.first_seen_at).not.toBeNull();
    expect(p.consecutive_lapses).toBe(5);
  });

  test('no warning below the threshold', async ({ page }) => {
    const email = await setupLeech(page);
    seedReviewWord(email, '记住', 2);
    await answerWrong(page);
    await expect(page.locator('#result-icon')).toHaveText('Not quite');
    await expect(page.locator('#leech-box')).not.toBeVisible();
  });

  test('the threshold is a Daily Learning setting', async ({ page }) => {
    await setupLeech(page);
    await page.goto('/settings');
    const input = page.locator('#leech-threshold');
    await expect(input).toHaveValue('5');
    await input.scrollIntoViewIfNeeded();
    await captureForPR(page, 'settings-leech-threshold');
    await input.fill('3');
    await input.dispatchEvent('change');
    await expect.poll(async () => (await (await page.request.get('/api/settings')).json()).leech_threshold).toBe(3);
  });
});
