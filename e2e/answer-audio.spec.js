// @ts-check
import { test, expect } from '@playwright/test';
import { syncNewWordMode } from './helpers/mode.js';
import { captureForPR } from './helpers/screenshot.js';

// Issue #638: a wrong Chinese answer gets a play button in the "Your answer"
// box, also when the typed word is not in the user's vocabulary.

test.describe('Play button for a wrong Chinese answer (issue #638)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useMode(page, mode) {
    await page.request.patch('/api/training-filters', {
      data: { mode, langs: ['en'], bucket: '', mnemonics: false, components: false, tags: [] },
    });
    await syncNewWordMode(page, mode);
    await page.addInitScript((m) => {
      localStorage.setItem('quizMode', m);
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    }, mode);
  }

  // recordAudio answers every audio request with an empty MP3 and records the URL.
  async function recordAudio(page) {
    const urls = [];
    await page.route('**/api/audio/**', (route) => {
      urls.push(decodeURIComponent(route.request().url()));
      return route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });
    return urls;
  }

  async function answerWrong(page, answer) {
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.locator('#answer-input').fill(answer);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
  }

  test('a Chinese answer that is not a vocabulary word can be played', async ({ page }) => {
    const urls = await recordAudio(page);
    await useMode(page, 'transl_to_zh');
    await answerWrong(page, '鼹');

    const box = page.locator('.result-your-answer');
    await expect(box).toContainText('鼹');
    const play = box.locator('.your-answer-play');
    await expect(play).toBeVisible();
    await captureForPR(page, 'train-your-answer-play');

    await play.click();
    await expect.poll(() => urls.some((u) => u.includes('/api/audio/text?zh=鼹')), { timeout: 5_000 }).toBe(true);
  });

  test('an answer with Latin letters has no play button', async ({ page }) => {
    await recordAudio(page);
    await useMode(page, 'transl_to_zh');
    await answerWrong(page, 'xxxxxxxx');

    await expect(page.locator('.result-your-answer')).toBeVisible();
    await expect(page.locator('.result-your-answer .your-answer-play')).toHaveCount(0);
  });
});
