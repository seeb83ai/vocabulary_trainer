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

  test('bucket breakdown can be filtered by tag (issue #286)', async ({ page }) => {
    // Seed a word with a unique tag so the tag-filter chip bar has something to
    // show, and so the filtered count is deterministic regardless of what
    // other specs have added to this shared DB.
    const createRes = await page.request.post('/api/words', {
      data: {
        zh_text: '桶词', pinyin: 'tǒng cí', translations: { en: ['bucket word'] },
        tags: ['bucket286'], start_training: true,
      },
    });
    const created = await createRes.json();

    try {
      await page.goto('/stats');
      await expect(page.locator('#word-stats-section')).toBeVisible({ timeout: 10_000 });

      const chips = page.locator('#bucket-tag-chips');
      const chip = chips.getByText('bucket286', { exact: true });
      await expect(chip).toBeVisible();

      await chip.click();
      await expect(chip).toHaveClass(/bg-blue-600/);

      // Only the tagged word matches the filter, so the bucket breakdown's
      // total across all tiers must be exactly one word (100%).
      await expect(page.locator('#tier-legend')).toContainText('(100%)');
    } finally {
      // Shared single-worker DB — clean up so this word/tag doesn't leak
      // into other specs' word counts or tag lists.
      await page.request.delete(`/api/words/${created.id}`);
    }
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
