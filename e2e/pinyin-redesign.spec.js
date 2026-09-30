// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// Redesigned Pinyin page (app redesign handoff): sticky bar with violet
// progress and group chips, big play button, 2×2 options, a result card
// with all tones, and the done / empty states. The e2e DB has no pinyin
// sounds, so the pinyin API is mocked per test.

test.use({ storageState: 'e2e/.auth/user.json' });

const CARD_MC = {
  sound_id: 11, audio_file: 'ma3.mp3', mode: 'multiple_choice',
  options: [
    { sound_id: 10, label: 'mā' }, { sound_id: 12, label: 'má' },
    { sound_id: 11, label: 'mǎ' }, { sound_id: 13, label: 'mà' },
  ],
};

const WRONG_ANSWER = {
  correct: false, correct_answer: 'mǎ', your_answer: '13', interval_days: 0,
  total_correct: 2, total_attempts: 5, learning: false, tier: 'Struggling',
  confused_with: { confused_with_label: 'mà', count: 3 },
  tone_variants: [
    { label: 'mā', filename: 'ma1.mp3', tone: 1, current: false },
    { label: 'má', filename: 'ma2.mp3', tone: 2, current: false },
    { label: 'mǎ', filename: 'ma3.mp3', tone: 3, current: true },
    { label: 'mà', filename: 'ma4.mp3', tone: 4, current: false },
  ],
};

async function mockPinyin(page, { next, stats = { due_today: 4, total: 1600 }, answer = WRONG_ANSWER, tags = ['tones', 'initials'] }) {
  await page.route('**/api/pinyin-quiz/audio/**', r => r.fulfill({ status: 204, body: '' }));
  await page.route('**/api/pinyin-quiz/tags', r => r.fulfill({ json: tags }));
  await page.route('**/api/pinyin-quiz/stats**', r => r.fulfill({ json: stats }));
  await page.route('**/api/pinyin-quiz/next**', r => next
    ? r.fulfill({ json: next })
    : r.fulfill({ status: 404, json: { error: 'no pinyin sounds available' } }));
  await page.route('**/api/pinyin-quiz/answer', r => r.fulfill({ json: answer }));
}

test.describe('Pinyin redesign', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('question card with the sticky bar, then a wrong answer shows all tones', async ({ page }) => {
    await mockPinyin(page, { next: CARD_MC });
    await page.goto('/pinyin');

    await expect(page.locator('#card-area')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#pinyin-bar')).toBeVisible();
    await expect(page.locator('#pinyin-progress-label')).toHaveText('0 of 4 today');
    await expect(page.locator('#pinyin-total-label')).toHaveText('4 due · 1600 sounds');
    await expect(page.locator('#tag-chips .tag-btn')).toHaveCount(3);
    await expect(page.locator('#tag-chips .tag-btn').first()).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#mode-label')).toHaveText('Listen → pick');
    await expect(page.locator('#mc-options .mc-btn')).toHaveCount(4);
    await captureForPR(page, 'pinyin-question');

    await page.locator('#mc-options .mc-btn', { hasText: 'mà' }).click();
    await expect(page.locator('#result-area')).toBeVisible();
    await expect(page.locator('#result-icon')).toHaveText('Not quite');
    await expect(page.locator('#result-correct-answer')).toHaveText('mǎ');
    await expect(page.locator('#result-your-answer')).toContainText('mà');
    await expect(page.locator('#result-confusion')).toContainText('mà');
    const variants = page.locator('#tone-variants .py-variant');
    await expect(variants).toHaveCount(4);
    await expect(variants.nth(2)).toHaveAttribute('aria-current', 'true');
    await expect(variants.nth(0)).toContainText('1st');
    await expect(page.locator('#bucket-info')).toHaveClass(/tier-chip-struggling/);
    await expect(page.locator('#bucket-info')).toContainText('Struggling');
    await expect(page.locator('#pinyin-progress-label')).toHaveText('1 of 5 today');
    await captureForPR(page, 'pinyin-result-wrong');
  });

  test('done state links to word training', async ({ page }) => {
    await mockPinyin(page, { next: null, stats: { due_today: 0, total: 1600 } });
    await page.goto('/pinyin');
    await expect(page.locator('#success-state')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#success-state a[href="/train"]')).toBeVisible();
    await captureForPR(page, 'pinyin-done');
  });

  test('empty state explains the import', async ({ page }) => {
    await mockPinyin(page, { next: null, stats: { due_today: 0, total: 0 }, tags: [] });
    await page.goto('/pinyin');
    await expect(page.locator('#empty-state')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#empty-state code')).toHaveText('make import-pinyin');
  });
});

test.describe('Pinyin redesign – phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('the play button and 2×2 options fit a phone', async ({ page }) => {
    await mockPinyin(page, { next: CARD_MC });
    await page.goto('/pinyin');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 10_000 });
    const play = await page.locator('#play-btn').boundingBox();
    expect(play?.width).toBe(112);
    const first = await page.locator('#mc-options .mc-btn').nth(0).boundingBox();
    const second = await page.locator('#mc-options .mc-btn').nth(1).boundingBox();
    expect(Math.abs((first?.y ?? 0) - (second?.y ?? 1))).toBeLessThan(1);
    const scrollW = await page.evaluate(() => document.documentElement.scrollWidth);
    expect(scrollW).toBeLessThanOrEqual(390);
    await captureForPR(page, 'pinyin-question-phone');
  });
});
