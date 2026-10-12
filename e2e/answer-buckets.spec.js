import { test, expect } from '@playwright/test';
import { syncNewWordMode } from './helpers/mode.js';
import { getTodayAnswerBuckets } from './helpers/db.js';

// Answer buckets record the help the user had before answering: pinyin
// seen (shown, or blurred and tapped) and audio played (button or autoplay).
// Help buckets record if a pinyin reveal or play was offered and used.
const wrong = { attempts: 1, errors: 1 };

test.describe('Answer buckets', () => {
  async function setup(page, { blurPinyin }) {
    const email = `e2e-buckets-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
    const regRes = await page.request.post('/api/register', {
      data: { email, password: 'AnswerBuckets123!' },
    });
    expect(regRes.ok()).toBeTruthy();
    const seedRes = await page.request.post('/api/words', {
      data: { zh_text: '橙子', pinyin: 'chéngzi', translations: { en: ['orange'] }, tags: [], start_training: true },
    });
    expect(seedRes.ok()).toBeTruthy();

    const settings = await (await page.request.get('/api/settings')).json();
    await page.request.patch('/api/settings', {
      data: { ...settings, blur_pinyin: blurPinyin, autoplay_always: false, wrong_answer_retry_mode: 'off' },
    });
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_pinyin_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    await syncNewWordMode(page, 'zh_pinyin_to_transl');
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_pinyin_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText('橙子');
    return email;
  }

  async function answerWrong(page) {
    await page.locator('#answer-input').fill('apple');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
  }

  test('pinyin shown and sound played count the error in both buckets', async ({ page }) => {
    const email = await setup(page, { blurPinyin: false });
    await page.locator('#play-btn').click();
    await answerWrong(page);

    expect(getTodayAnswerBuckets(email, '橙子')).toEqual({
      pinyin: wrong, voice: wrong, play_offered: wrong, play_pressed: wrong,
    });
  });

  test('pinyin shown without sound counts only as pinyin', async ({ page }) => {
    const email = await setup(page, { blurPinyin: false });
    await answerWrong(page);

    expect(getTodayAnswerBuckets(email, '橙子')).toEqual({
      pinyin: wrong, play_offered: wrong,
    });
  });

  test('blurred pinyin that was not tapped counts as hanzi only', async ({ page }) => {
    const email = await setup(page, { blurPinyin: true });
    await answerWrong(page);

    expect(getTodayAnswerBuckets(email, '橙子')).toEqual({
      zh_only: wrong, pinyin_reveal_offered: wrong, play_offered: wrong,
    });
  });

  test('blurred pinyin that was tapped counts as pinyin', async ({ page }) => {
    const email = await setup(page, { blurPinyin: true });
    await page.locator('#pinyin-hint').click();
    await answerWrong(page);

    expect(getTodayAnswerBuckets(email, '橙子')).toEqual({
      pinyin: wrong, pinyin_reveal_offered: wrong, pinyin_revealed: wrong, play_offered: wrong,
    });
  });

  test('"Accept as correct" removes the error from the same bucket', async ({ page }) => {
    // Blurred and not tapped: zh_only. Without the flags the server would
    // fall back to pinyin + voice and leave the zh_only error in place.
    const email = await setup(page, { blurPinyin: true });
    await page.locator('#answer-input').fill('orang');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#accept-correct-btn')).toBeVisible({ timeout: 8_000 });
    const acceptRes = page.waitForResponse(r => r.url().includes('/api/quiz/accept-correct'));
    await page.locator('#accept-correct-btn').scrollIntoViewIfNeeded();
    await page.locator('#accept-correct-btn').click();
    expect((await acceptRes).status()).toBe(200);

    const accepted = { attempts: 1, errors: 0 };
    expect(getTodayAnswerBuckets(email, '橙子')).toEqual({
      zh_only: accepted, pinyin_reveal_offered: accepted, play_offered: accepted,
    });
  });
});
