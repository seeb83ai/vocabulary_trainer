// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// ─────────────────────────────────────────────────────────────────────────────
// Issue #466: some characters look the same (囗 wéi vs 口 kǒu). When pinyin is
// hidden, the glyph alone can't tell them apart, so the quiz card shows a
// "≠ 口 (mouth)" hint for known look-alike pairs.
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Look-alike character hint (issue #466)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function mockNextCard(page, card) {
    await page.addInitScript(() => {
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
    await page.route('**/api/quiz/next*', (route) => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(card),
    }));
  }

  test('component card shows the look-alike hint', async ({ page }) => {
    await mockNextCard(page, {
      card_type: 'component',
      prompt: '囗',
      pinyin: null,
      is_new: false,
      definitions: { en: 'enclosure' },
      lookalikes: [{ character: '口', definitions: { en: 'mouth; opening' } }],
    });
    await page.goto('/train');
    await expect(page.locator('#prompt-word')).toHaveText('囗', { timeout: 12_000 });
    await expect(page.locator('#lookalike-hint')).toBeVisible();
    await expect(page.locator('#lookalike-hint')).toHaveText('≠ 口 (mouth)');
    await captureForPR(page, 'train-lookalike-component');
  });

  test('word card in zh_to_transl mode shows the look-alike hint', async ({ page }) => {
    await mockNextCard(page, {
      word_id: 999999,
      mode: 'zh_to_transl',
      prompt: '口',
      pinyin: null,
      lookalikes: [{ character: '囗', definitions: { en: 'enclosure' } }],
    });
    await page.goto('/train');
    await expect(page.locator('#prompt-word')).toHaveText('口', { timeout: 12_000 });
    await expect(page.locator('#lookalike-hint')).toHaveText('≠ 囗 (enclosure)');
    await captureForPR(page, 'train-lookalike-word');
  });

  test('card without look-alikes hides the hint', async ({ page }) => {
    await mockNextCard(page, {
      word_id: 999998,
      mode: 'zh_to_transl',
      prompt: '人',
      pinyin: null,
    });
    await page.goto('/train');
    await expect(page.locator('#prompt-word')).toHaveText('人', { timeout: 12_000 });
    await expect(page.locator('#lookalike-hint')).toBeHidden();
  });
});
