// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { seedYesterdayBucketSnapshot } from './helpers/db.js';

const PASSWORD = 'E2eSessionEndPass123!';

/**
 * Register a brand-new user in the browser, seed one due word via the API
 * (page.request shares the browser session cookies), and force
 * zh_to_transl mode so the expected answer is known.
 */
async function setupUserWithOneDueWord(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => {
    route.fulfill({ status: 200, body: '' });
  });
  const email = `e2e-sessionend-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });

  const wordRes = await page.request.post('/api/words', {
    data: {
      zh_text: '好',
      pinyin: 'hǎo',
      translations: { en: ['good'] },
      tags: [],
      start_training: true,
    },
  });
  expect(wordRes.ok()).toBe(true);

  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
  });
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  return email;
}

/** Answers the one seeded due word correctly 3 times in a row, graduating it
 * out of today's queue and reaching the "all done" success screen. */
async function finishSessionWithCorrectAnswers(page) {
  for (let i = 0; i < 3; i++) {
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.locator('#answer-input').fill('good');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 8_000 });
    await page.locator('#next-btn').click();
  }
  await expect(page.locator('#success-state')).toBeVisible({ timeout: 12_000 });
}

test.describe('End-of-session comeback screen', () => {
  test('finishing the session shows day streak and due-tomorrow info', async ({ page }) => {
    await setupUserWithOneDueWord(page);

    await page.goto('/train');

    // The acknowledged word is in the learning phase: it needs 3 consecutive
    // correct answers to graduate out of today's queue.
    await finishSessionWithCorrectAnswers(page);

    // All done for today → the comeback block must be shown with a day
    // streak of 1 (the user trained today) and a numeric due-tomorrow count.
    await expect(page.locator('#success-comeback')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#success-streak')).toHaveText('1');
    await expect(page.locator('#success-due-tomorrow')).toHaveText(/^\d+$/);
  });

  test('shows how many words moved up a bucket today', async ({ page }) => {
    const email = await setupUserWithOneDueWord(page);

    // Yesterday: the seeded word had 0 attempts and sat in every bucket at 0.
    // Answering it correctly 3 times today graduates it out of the "new"
    // learning phase into bucket_learning (attempts=3, 100% accuracy), a net
    // +1 in a non-new bucket relative to yesterday's all-zero snapshot.
    seedYesterdayBucketSnapshot(email);

    await page.goto('/train');
    await finishSessionWithCorrectAnswers(page);

    await expect(page.locator('#success-comeback')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#success-improved')).toBeVisible();
    await expect(page.locator('#success-improved-count')).toHaveText('1');
    await captureForPR(page, 'train-all-done-words-improved');
  });
});
