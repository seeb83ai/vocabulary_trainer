// @ts-check
import { test, expect } from '@playwright/test';
import { syncNewWordMode } from './helpers/mode.js';
import { seedReviewWord } from './helpers/db.js';

// Issue #581: when the result screen fails to draw, the app shows the error
// and goes on to the next card instead of leaving a half-drawn screen.

async function setup(page) {
  const email = `e2e-clienterr-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'ClientErr123!' } });
  expect(regRes.ok()).toBeTruthy();
  for (const [zh, pinyin, en] of [['关于', 'guān yú', 'about'], ['已经', 'yǐ jīng', 'already']]) {
    const res = await page.request.post('/api/words', {
      data: { zh_text: zh, pinyin, translations: { en: [en] }, tags: [], start_training: true },
    });
    expect(res.ok()).toBeTruthy();
    seedReviewWord(email, zh, 0);
  }
  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: false, components: false, tags: [] },
  });
  await syncNewWordMode(page, 'zh_to_transl');
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  await page.goto('/train');
  await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
}

// The answer is saved, but the reply has a shape the result screen can't draw.
async function breakResultScreen(page) {
  await page.route('**/api/quiz/answer', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ correct: false, translations: { en: 'not a list' } }),
  }));
}

test.describe('Result screen errors', () => {
  test('a result screen that fails to draw shows the error and loads the next card', async ({ page }) => {
    await setup(page);
    await breakResultScreen(page);

    const nextCard = page.waitForResponse(r => r.url().includes('/api/quiz/next'));
    await page.locator('#answer-input').fill('wrong');
    await page.locator('#answer-form button[type="submit"]').click();

    await expect(page.getByTestId('toast')).toBeVisible();
    await expect(page.getByTestId('toast')).toHaveClass(/app-toast-error/);
    await nextCard;
    await expect(page.locator('#result-area')).not.toBeVisible();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#answer-input')).toHaveValue('');
  });
});
