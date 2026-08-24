// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

test.use({ storageState: 'e2e/.auth/user.json' });

test.describe('Stats page', () => {
  test('renders the stats tabs and the words panel', async ({ page }) => {
    await page.goto('/stats');
    // Tab navigation is present.
    await expect(page.locator('#tab-words')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#tab-pinyin')).toBeVisible();
    await expect(page.locator('#tab-mnemonics')).toBeVisible();
    // The default words panel is shown.
    await expect(page.locator('#panel-words')).toBeVisible();
  });

  test('shows a components by due date chart on the components tab', async ({ page }) => {
    await page.goto('/stats');
    await page.locator('#tab-components').click();
    await expect(page.locator('#panel-components')).toBeVisible();
    await expect(page.locator('#panel-components h2', { hasText: 'Components by Due Date' })).toBeVisible();
    await expect(page.locator('#comp-due-date-chart')).toBeAttached();
  });

  test('training history chart renders after answering quiz questions', async ({ page }) => {
    const wordsRes = await page.request.get('/api/words');
    const { words } = await wordsRes.json();

    // Answer each seeded word (mix of correct/wrong) so RecordDailyStat has
    // data to chart — a fresh user has no daily_stats rows yet.
    const SEED_TRANSLATIONS = {
      '你好': 'hello',
      '谢谢': 'thank you',
      '再见': 'goodbye',
    };
    for (const w of words) {
      await page.request.post('/api/quiz/answer', {
        data: { word_id: w.id, mode: 'zh_to_transl', answer: SEED_TRANSLATIONS[w.zh_text], langs: ['en'] },
      });
    }
    await page.request.post('/api/quiz/answer', {
      data: { word_id: words[0].id, mode: 'zh_to_transl', answer: 'xxxxxxxxxxx', langs: ['en'] },
    });

    await page.goto('/stats');
    await expect(page.locator('#stats-chart')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#chart-empty')).not.toBeVisible();
    // Let the Chart.js load animation settle before capturing.
    await page.waitForTimeout(800);
    await captureForPR(page, 'stats-page');
  });
});
