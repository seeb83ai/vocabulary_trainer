// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';

// Issue #537: the all-done screen links to the words answered wrong today,
// and "Train them again" makes them due at once.
//
// A fresh user with two words: 水 is answered wrong, 山 right. Both are then
// skipped to tomorrow, so the all-done screen shows.
async function setupAllDoneWithMistake(page) {
  const email = `e2e-wrong-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'WrongTest123!' } });
  expect(regRes.ok()).toBeTruthy();

  const ids = {};
  for (const [zh, pinyin, en] of [['水', 'shuǐ', 'water'], ['山', 'shān', 'mountain']]) {
    const res = await page.request.post('/api/words', {
      data: { zh_text: zh, pinyin, translations: { en: [en] }, tags: [], start_training: true },
    });
    expect(res.ok()).toBeTruthy();
    ids[zh] = (await res.json()).id;
  }
  for (const [zh, answer] of [['水', 'fire'], ['山', 'mountain']]) {
    const res = await page.request.post('/api/quiz/answer', {
      data: { word_id: ids[zh], mode: 'zh_to_transl', answer, langs: ['en'] },
    });
    expect(res.ok()).toBeTruthy();
    const skip = await page.request.post('/api/quiz/skip', { data: { word_id: ids[zh], days: 1 } });
    expect(skip.ok()).toBeTruthy();
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

test.describe("All done – review today's mistakes (issue #537)", () => {
  test('lists the words answered wrong today and trains them again', async ({ page }) => {
    await setupAllDoneWithMistake(page);
    await page.goto('/train');
    await expect(page.locator('#success-state')).toBeVisible({ timeout: 12_000 });

    const link = page.locator('#wrong-today-btn');
    await expect(link).toBeVisible({ timeout: 8_000 });
    await expect(link).toContainText('1');
    await captureForPR(page, 'train-all-done-mistakes-link');

    await link.click();
    const sheet = page.locator('#wrong-today-overlay');
    await expect(sheet).toBeVisible();
    const rows = sheet.locator('.wt-row');
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText('水');
    await expect(rows.first()).toContainText('shuǐ');
    await expect(rows.first()).toContainText('water');
    await expect(sheet).not.toContainText('山');
    await captureForPR(page, 'train-all-done-mistakes-sheet');

    await sheet.locator('#wrong-today-retrain-btn').click();
    await expect(sheet).not.toBeVisible();
    await expect(page.locator('#success-state')).not.toBeVisible();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#prompt-word')).toHaveText('水');
  });

  test('the sheet closes without changes', async ({ page }) => {
    await setupAllDoneWithMistake(page);
    await page.goto('/train');
    await expect(page.locator('#wrong-today-btn')).toBeVisible({ timeout: 12_000 });

    await page.locator('#wrong-today-btn').click();
    await expect(page.locator('#wrong-today-overlay')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('#wrong-today-overlay')).not.toBeVisible();
    await page.locator('#wrong-today-btn').click();
    await page.locator('#wrong-today-close').click();
    await expect(page.locator('#wrong-today-overlay')).not.toBeVisible();
    await expect(page.locator('#success-state')).toBeVisible();
  });

  test('no link when nothing was answered wrong today', async ({ page }) => {
    const email = `e2e-wrong-none-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
    expect((await page.request.post('/api/register', { data: { email, password: 'WrongTest123!' } })).ok()).toBeTruthy();
    const res = await page.request.post('/api/words', {
      data: { zh_text: '山', pinyin: 'shān', translations: { en: ['mountain'] }, tags: [], start_training: true },
    });
    const id = (await res.json()).id;
    await page.request.post('/api/quiz/answer', { data: { word_id: id, mode: 'zh_to_transl', answer: 'mountain', langs: ['en'] } });
    await page.request.post('/api/quiz/skip', { data: { word_id: id, days: 1 } });

    await page.goto('/train');
    await expect(page.locator('#success-state')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#success-comeback')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#wrong-today-btn')).not.toBeVisible();
  });
});
