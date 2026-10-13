// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';
import { seedReviewWord, makeWordDueNow, getWordDueDate } from './helpers/db.js';

// Issue #554: after 3 wrong answers in a row on the same card in this
// session, the wrong-answer screen also offers "Skip for today".

async function setup(page) {
  const email = `e2e-skipwrong-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'SkipWrong123!' } });
  expect(regRes.ok()).toBeTruthy();
  const res = await page.request.post('/api/words', {
    data: { zh_text: '关于', pinyin: 'guān yú', translations: { en: ['about'] }, tags: [], start_training: true },
  });
  expect(res.ok()).toBeTruthy();
  seedReviewWord(email, '关于', 0);
  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: false, components: false, tags: [] },
  });
  await syncNewWordMode(page, 'zh_to_transl');
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  await page.goto('/train');
  return email;
}

async function answer(page, text) {
  await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
  await expect(page.locator('#prompt-word')).toHaveText('关于');
  await page.locator('#answer-input').fill(text);
  await page.locator('#answer-form button[type="submit"]').click();
  await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
}

// Next card: the same word again, made due at once (no 3-minute retry wait).
async function nextSameCard(page, email) {
  makeWordDueNow(email, '关于');
  await page.locator('#next-btn').click();
}

test.describe('Skip for today after repeated wrong answers', () => {
  test('shows the skip button on the 3rd wrong answer in a row and skips the card', async ({ page }) => {
    const email = await setup(page);
    const skipBtn = page.locator('#result-skip-today-btn');

    await answer(page, 'wrong one');
    await expect(skipBtn).not.toBeVisible();
    await nextSameCard(page, email);

    await answer(page, 'wrong two');
    await expect(skipBtn).not.toBeVisible();
    await nextSameCard(page, email);

    await answer(page, 'wrong three');
    await expect(skipBtn).toBeVisible();
    await captureForPR(page, 'train-skip-after-wrong');

    await skipBtn.click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });
    // Skipped for 1 day: the word is due tomorrow, not later today.
    const due = getWordDueDate(email, '关于');
    expect(new Date(due.replace(' ', 'T') + 'Z').getTime()).toBeGreaterThan(Date.now() + 20 * 3600 * 1000);
  });

  test('a correct answer resets the count', async ({ page }) => {
    const email = await setup(page);
    const skipBtn = page.locator('#result-skip-today-btn');

    await answer(page, 'wrong one');
    await nextSameCard(page, email);
    await answer(page, 'wrong two');
    await nextSameCard(page, email);
    await answer(page, 'about');
    await nextSameCard(page, email);
    await answer(page, 'wrong three');
    await expect(skipBtn).not.toBeVisible();
  });

  test('"Accept as correct" resets the count', async ({ page }) => {
    const email = await setup(page);
    const skipBtn = page.locator('#result-skip-today-btn');

    await answer(page, 'wrong one');
    await nextSameCard(page, email);
    await answer(page, 'wrong two');
    await nextSameCard(page, email);
    await answer(page, 'abou');
    await expect(page.locator('#accept-correct-btn')).toBeVisible();
    // Accept makes the word due later and loads the next card at once; make
    // it due again before that next-card request.
    await page.route('**/api/quiz/accept-correct', async route => {
      const res = await route.fetch();
      makeWordDueNow(email, '关于');
      await route.fulfill({ response: res });
    });
    await page.locator('#accept-correct-btn').click();
    await answer(page, 'wrong three');
    await expect(skipBtn).not.toBeVisible();
  });
});
