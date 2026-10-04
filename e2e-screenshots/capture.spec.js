// @ts-check
// Regenerates the README screenshots against a real browser + real server.
// Run via `make screenshots-readme` — NOT part of `make test-e2e` / CI (separate
// testDir + playwright.screenshots.config.js so the default `npx playwright
// test` run never picks this file up).
import { test, expect } from '@playwright/test';
import { openAuthModal } from '../e2e/helpers/auth.js';
import {
  seedWords, seedConfusions, runGoSeedTool, patchSettings, openCard, answer,
} from './seed.js';

test.use({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1 });

// ── Login page needs no session and no data ─────────────────────────────────
test.describe('README screenshots — login', () => {
  test('login / register', async ({ page }) => {
    await page.goto('/');
    await openAuthModal(page);
    await expect(page.locator('#signin-form')).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_login.png' });
  });
});

test.describe.serial('README screenshots — app pages', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('seed demo data', async ({ page }) => {
    await seedWords(page);
    await seedConfusions(page);
    runGoSeedTool('e2e-seed-hmm');
    runGoSeedTool('e2e-seed-pinyin');
    await page.request.put('/api/hmm/props', { data: { radical: '氵', prop_name: 'Water bottle' } });
    // Gamification on so the match game can show; rare enough to stay out of the way.
    await patchSettings(page, { gamification_enabled: true, gamification_frequency: 60 });
  });

  test('training question', async ({ page }) => {
    await openCard(page, '超市', 'zh_pinyin_to_transl');
    await page.screenshot({ path: 'images/chinese_train.png' });
  });

  test('training answer', async ({ page }) => {
    await openCard(page, '超市', 'zh_pinyin_to_transl');
    await answer(page, 'supermarket');
    await page.screenshot({ path: 'images/chinese_train_answer.png' });
  });

  test('settings', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('#sec-training')).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_settings.png' });
  });

  test('mnemonics (HMM) builder', async ({ page }) => {
    await page.goto('/mnemonics');
    // Actor/location/tone-room names render as <input value="…">, not text content.
    await expect(page.locator('#actors-container input').first()).toHaveValue('Bruce Lee', { timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_mnemonics.png' });
  });

  test('pinyin listening quiz', async ({ page }) => {
    await page.goto('/pinyin');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_pinyin.png' });
  });

  test('mismatches overview', async ({ page }) => {
    await page.goto('/mismatches');
    await expect(page.locator('#mismatches-list')).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_mismatches.png' });
  });

  test('gamification match game', async ({ page }) => {
    await patchSettings(page, { gamification_frequency: 1 });
    await openCard(page, '超市', 'zh_to_transl');
    await answer(page, 'xxxxxxxxxxx');
    await page.locator('#next-btn').click();
    await expect(page.locator('#match-game-overlay')).toBeVisible({ timeout: 8_000 });
    await page.screenshot({ path: 'images/chinese_gamification.png' });
  });

  test('vocabulary management', async ({ page }) => {
    runGoSeedTool('e2e-seed-history');
    await page.goto('/vocab');
    await expect(page.locator('#words-tbody > *').first()).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: 'images/chinese_vocabulary.png' });
  });

  test('stats dashboard', async ({ page }) => {
    await page.goto('/stats');
    await expect(page.locator('#stats-chart')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#hardest-body .sx-empty')).toHaveCount(0, { timeout: 10_000 });
    // Let the Chart.js load animation settle before capturing.
    await page.waitForTimeout(2500);
    await page.screenshot({ path: 'images/chinese_stats.png' });
  });
});
