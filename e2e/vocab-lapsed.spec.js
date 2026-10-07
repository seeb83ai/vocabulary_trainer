// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { seedReviewWord } from './helpers/db.js';

// Vocabulary "Failed last review" chip: lists the words whose last review
// was wrong (consecutive_lapses >= 1), so the user can reset them from the
// edit sheet.

test('Vocabulary chip lists the words that failed their last review', async ({ page }) => {
  const email = `e2e-vlapsed-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'VocabLapsed123!' } });
  expect(regRes.ok()).toBeTruthy();
  for (const [zh, en] of [['记住', 'remember'], ['山', 'mountain']]) {
    const res = await page.request.post('/api/words', {
      data: { zh_text: zh, translations: { en: [en] }, tags: [], start_training: true },
    });
    expect(res.ok()).toBeTruthy();
  }
  seedReviewWord(email, '记住', 2);
  seedReviewWord(email, '山', 0);

  await page.goto('/vocab');
  const rows = page.locator('#words-tbody .vb-row');
  await expect(rows).toHaveCount(2, { timeout: 10_000 });

  const chip = page.locator('#due-lapsed-btn');
  await expect(chip).toHaveText('Failed last review');
  await chip.click();
  await expect(chip).toHaveAttribute('aria-pressed', 'true');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText('记住');
  await captureForPR(page, 'vocab-lapsed-filter');

  // Reset from the edit sheet: the word leaves the list.
  await rows.first().click();
  await expect(page.locator('#vocab-sheet')).toBeVisible();
  page.once('dialog', d => d.accept());
  await page.locator('#form-reset-btn').click();
  await expect(rows).toHaveCount(0, { timeout: 8_000 });
});
