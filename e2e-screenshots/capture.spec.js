// @ts-check
// Regenerates the README screenshots against a real browser + real server.
// Run via `make screenshots` — NOT part of `make test-e2e` / CI (separate
// testDir + playwright.screenshots.config.js so the default `npx playwright
// test` run never picks this file up).
import { test, expect } from '@playwright/test';
import { execSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { TEST_EMAIL } from '../e2e/global-setup.js';

/** Translations keyed by zh word — must match the seed in e2e/global-setup.js. */
const SEED_TRANSLATIONS = {
  '你好': ['hello', 'hi'],
  '谢谢': ['thank you', 'thanks'],
  '再见': ['goodbye', 'bye'],
};

/**
 * Run one of the test-only e2e-seed-* Go CLIs (service/cmd/e2e-seed-pinyin,
 * service/cmd/e2e-seed-hmm) against the running E2E server's temp DB. Both
 * exist because their target tables have no REST endpoint for creating rows
 * from scratch — only updating rows that already exist.
 */
function runGoSeedTool(tool) {
  const { dbPath } = JSON.parse(readFileSync(join('e2e', '.state', 'server.json'), 'utf8'));
  execSync(`go run ./cmd/${tool} -db "${dbPath}" -email ${TEST_EMAIL}`, {
    cwd: 'service',
    stdio: 'inherit',
  });
}

function useZhToTranslMode(page) {
  return page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
}

// ── Pages that don't mutate shared SM-2/confusion state ─────────────────────
test.describe.serial('README screenshots — clean pages', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('vocabulary management', async ({ page }) => {
    await page.goto('/vocab');
    await expect(page.locator('#words-tbody tr').first()).toBeVisible({ timeout: 10_000 });
    // The word list sits below the Add Word form — scroll it into view so the
    // screenshot shows the list, not the form.
    await page.locator('#words-table-wrap').scrollIntoViewIfNeeded();
    await page.screenshot({ path: 'images/chinese_vocabulary.png' });
  });

  test('training question', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.screenshot({ path: 'images/chinese_train.png' });
  });

  test('training answer', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    const prompt = await page.locator('#prompt-word').textContent();
    const correctAnswer = SEED_TRANSLATIONS[prompt]?.[0];
    await page.locator('#answer-input').fill(correctAnswer);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
    await page.screenshot({ path: 'images/chinese_train_answer.png' });
  });

  test('settings', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('#gamification-enabled')).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_settings.png' });
  });

  test('mnemonics (HMM) builder', async ({ page }) => {
    // Actors/locations/tone-rooms only support UPDATE via REST and a fresh
    // user has no rows to update — seed them directly (see runGoSeedTool).
    // Props are the one library type a fresh user CAN create via REST.
    runGoSeedTool('e2e-seed-hmm');
    await page.request.put('/api/hmm/props', { data: { radical: '氵', prop_name: 'Water bottle' } });

    await page.goto('/mnemonics');
    // Actor/location/tone-room names render as <input value="…">, not text content.
    await expect(page.locator('#actors-container input').first()).toHaveValue('Bruce Lee', { timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_mnemonics.png' });
  });

  test('pinyin listening quiz', async ({ page }) => {
    // No REST endpoint creates pinyin_sounds rows (only the CLI-only import
    // flow, which needs real MP3 files) — seed directly instead.
    runGoSeedTool('e2e-seed-pinyin');

    await page.goto('/pinyin');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_pinyin.png' });
  });
});

// ── Unauthenticated landing page ────────────────────────────────────────────
test.describe('README screenshots — login', () => {
  test('login / register', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#signin-form')).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_login.png' });
  });
});

// ── Pages whose seeding mutates shared SM-2/confusion state — captured last
// so earlier "clean" screenshots aren't affected by the wrong answers below.
test.describe.serial('README screenshots — confusion-seeded pages', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('mismatches overview', async ({ page }) => {
    const wordsRes = await page.request.get('/api/words');
    const { words } = await wordsRes.json();
    const idByZh = Object.fromEntries(words.map(w => [w.zh_text, w.id]));

    // Answer each seeded word with a DIFFERENT seeded word's translation to
    // deliberately record confusion pairs (DetectConfusion + UpsertConfusion).
    const zhWords = Object.keys(SEED_TRANSLATIONS);
    for (const zh of zhWords) {
      const other = zhWords.find(w => w !== zh);
      await page.request.post('/api/quiz/answer', {
        data: {
          word_id: idByZh[zh],
          mode: 'zh_to_transl',
          answer: SEED_TRANSLATIONS[other][0],
          langs: ['en'],
        },
      });
    }
    // Also record a couple of correct answers so the stats charts (below)
    // aren't all mistakes.
    for (const zh of zhWords) {
      await page.request.post('/api/quiz/answer', {
        data: { word_id: idByZh[zh], mode: 'zh_to_transl', answer: SEED_TRANSLATIONS[zh][0], langs: ['en'] },
      });
    }

    await page.goto('/mismatches');
    await expect(page.locator('#table-wrap')).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_mismatches.png' });
  });

  test('gamification match game', async ({ page }) => {
    // PATCH /api/settings replaces the full settings object — required mode
    // fields must be present even though only the gamification ones matter here.
    await page.request.patch('/api/settings', {
      data: {
        primary_lang: 'en',
        prog_new: 'zh_to_transl',
        prog_tier_struggling: 'transl_to_zh',
        prog_tier_learning: 'zh_pinyin_to_transl',
        prog_tier_practicing: 'zh_to_transl',
        prog_tier_mastered: 'random',
        new_word_mode_0: 'transl_to_zh',
        new_word_mode_1: 'zh_pinyin_to_transl',
        new_word_mode_2: 'zh_to_transl',
        gamification_enabled: true,
        gamification_frequency: 1,
      },
    });

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    await page.locator('#next-btn').click();
    await expect(page.locator('#match-game-overlay')).toBeVisible({ timeout: 8_000 });
    await page.screenshot({ path: 'images/chinese_gamification.png' });
  });

  test('stats dashboard', async ({ page }) => {
    await page.goto('/stats');
    await expect(page.locator('#stats-chart')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#chart-empty')).not.toBeVisible();
    // Let the Chart.js load animation settle before capturing.
    await page.waitForTimeout(800);
    await page.screenshot({ path: 'images/chinese_stats.png' });
  });
});
