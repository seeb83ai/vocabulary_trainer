// @ts-check
// Regenerates the landing-page teaser images in
// service/frontend/landing/teasers/<slug>/ from the real, seeded app.
// Run via `make screenshots-landing` — NOT part of `make test-e2e` / CI
// (separate config, so the default `npx playwright test` never picks it up).
//
// Not covered (kept by hand): 110-open-source/01-github.png (a GitHub page) and
// 120-self-hosted/01-self_hosted.png (a terminal).
import { test, expect } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { dirname } from 'node:path';
import { seedWords, seedConfusions, runGoSeedTool, patchSettings, openCard, answer } from './seed.js';

const OUT = 'service/frontend/landing/teasers';
test.use({ storageState: 'e2e/.auth/user.json', viewport: { width: 1280, height: 900 } });

/** Viewport-relative bounding box of a locator. */
async function box(loc) {
  await loc.first().waitFor({ state: 'visible', timeout: 10_000 });
  return loc.first().evaluate(el => {
    const r = el.getBoundingClientRect();
    return { x: r.x, y: r.y, width: r.width, height: r.height };
  });
}

/**
 * Screenshot the union of one or more locators (plus padding) to OUT/<rel>.
 * Grows the viewport to fit tall content instead of using a full-page capture:
 * a full-page screenshot resizes the viewport mid-capture, which restarts the
 * Chart.js animations and mis-positions fixed sheets.
 * @param {import('@playwright/test').Page} page
 * @param {string} rel
 * @param {import('@playwright/test').Locator[]} locs
 * @param {number} [pad] padding on every side
 * @param {number} [padBottom] overrides the bottom padding
 */
async function shot(page, rel, locs, pad = 0, padBottom = pad) {
  await page.mouse.move(0, 0); // no stray hover states
  await page.evaluate(() => window.scrollTo(0, 0));
  let boxes = await Promise.all(locs.map(box));
  const need = Math.ceil(Math.max(...boxes.map(b => b.y + b.height)) + padBottom + 40);
  if (need > page.viewportSize().height) {
    await page.setViewportSize({ width: page.viewportSize().width, height: need });
    await page.waitForTimeout(2500); // layout + Chart.js resize animation
    boxes = await Promise.all(locs.map(box));
  }
  const x = Math.max(0, Math.min(...boxes.map(b => b.x)) - pad);
  const y = Math.max(0, Math.min(...boxes.map(b => b.y)) - pad);
  const right = Math.max(...boxes.map(b => b.x + b.width)) + pad;
  const bottom = Math.max(...boxes.map(b => b.y + b.height)) + padBottom;
  const path = `${OUT}/${rel}`;
  mkdirSync(dirname(path), { recursive: true });
  await page.screenshot({ path, clip: { x, y, width: right - x, height: bottom - y } });
}

test.describe.serial('landing teasers', () => {
  test('seed demo data', async ({ page }) => {
    await seedWords(page);
    await seedConfusions(page);
    runGoSeedTool('e2e-seed-hmm');
    runGoSeedTool('e2e-seed-pinyin');
    // Gamification on (day-streak card on the all-done screen), but rarely
    // enough that the mini-game stays out of the training shots.
    await patchSettings(page, { gamification_enabled: true, gamification_frequency: 60 });
  });

  test('010 training', async ({ page }) => {
    const card = page.locator('#card-area');
    await openCard(page, '超市', 'zh_pinyin_to_transl');
    await shot(page, '010-training/10-chinese_pinyin_visible.png', [card]);

    await patchSettings(page, { blur_pinyin: true });
    await page.reload();
    await expect(card).toBeVisible({ timeout: 12_000 });
    await shot(page, '010-training/14-chinese_pinyin.png', [card]);
    await patchSettings(page, { blur_pinyin: false });

    await openCard(page, '超市', 'zh_to_transl');
    await shot(page, '010-training/15-chinese.png', [card]);

    // Correct answer.
    await answer(page, 'supermarket');
    await shot(page, '010-training/11-correct_answer.png', [page.locator('#result-area')]);

    // Another word's meaning -> mix-up layout.
    await openCard(page, '超市', 'zh_to_transl');
    await answer(page, 'Stadt');
    await shot(page, '010-training/13-mismatch_small.png', [page.locator('#result-area')]);

    // Wrong answer with the retype gate.
    await patchSettings(page, { wrong_answer_retry_mode: 'matched' });
    await openCard(page, '行', 'zh_to_transl');
    await answer(page, 'Vorname');
    await shot(page, '010-training/12-wrong_answer.png', [page.locator('#result-area')]);
    await patchSettings(page, { wrong_answer_retry_mode: 'off' });

    // All done for today.
    await openCard(page, '号', 'zh_to_transl');
    const next = page.locator('#success-state:visible, #match-game-overlay:visible, #answer-input:visible').first();
    for (let i = 0; i < 6; i++) {
      await expect(next).toBeVisible({ timeout: 8_000 });
      if (await page.locator('#success-state').isVisible()) break;
      if (await page.locator('#match-game-overlay').isVisible()) {
        await page.locator('#match-game-overlay').getByText('Skip round').click();
        continue;
      }
      await answer(page, 'date');
      await page.locator('#next-btn').click();
    }
    await expect(page.locator('#success-state')).toBeVisible({ timeout: 12_000 });
    await shot(page, '010-training/20-all_done.png', [page.locator('#success-state')], 8);
  });

  test('040 match game', async ({ page }) => {
    await patchSettings(page, { gamification_enabled: true, gamification_frequency: 1 });
    await openCard(page, '超市', 'zh_to_transl');
    await answer(page, 'xxxxxxxxxxx');
    await page.locator('#next-btn').click();
    const overlay = page.locator('#match-game-overlay');
    await expect(overlay).toBeVisible({ timeout: 8_000 });
    await shot(page, '040-match-game/01-match.png', [overlay]);
  });

  test('020 pinyin', async ({ page }) => {
    await page.goto('/pinyin');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 10_000 });
    await shot(page, '020-pinyin/01-quiz.png', [page.locator('#card-area')]);
  });

  test('030 mnemonics', async ({ page }) => {
    for (const [radical, prop_name] of [['氵', 'Water bottle'], ['木', 'Wooden chair'], ['火', 'Torch'],
      ['口', 'Megaphone'], ['心', 'Heart balloon'], ['日', 'Sun lamp'], ['女', 'Ballet shoes'], ['手', 'Boxing glove']]) {
      await page.request.put('/api/hmm/props', { data: { radical, prop_name } });
    }
    await page.goto('/mnemonics');
    await expect(page.locator('#actors-container input').first()).toHaveValue('Bruce Lee', { timeout: 10_000 });
    // One shot per tab: title, tab bar and the whole open panel.
    for (const [tab, panel, file] of [
      ['actors', 'actors', '01-actors'], ['locations', 'locations', '02-locations'],
      ['rooms', 'rooms', '03-tone_rooms'], ['props', 'props', '04-props'],
    ]) {
      await page.locator(`#mn-tab-${tab}`).click();
      await expect(page.locator(`#mn-panel-${panel}`)).toBeVisible();
      await shot(page, `030-mnemonics/${file}.png`,
        [page.locator('h1').first(), page.locator(`#mn-panel-${panel}`)], 16);
    }

    // Tall viewport up front: the edit sheet is capped at a share of the viewport height.
    await page.setViewportSize({ width: 1280, height: 2200 });
    await page.goto('/vocab');
    await page.locator('#search-input').fill('想');
    await page.locator('#words-tbody > *', { hasText: '想' }).first().click();
    await page.locator('#hmm-builder-toggle').click();
    await expect(page.locator('#hmm-builder-container')).not.toContainText('Loading', { timeout: 10_000 });
    await page.waitForTimeout(500);
    const builder = page.locator('#hmm-builder-container');
    await shot(page, '030-mnemonics/05-scene_builder.png', [
      builder.getByText('Mnemonic Scene Builder'),
      builder.getByText('How does this work?'),
      builder.getByRole('button', { name: 'Save Scene' }),
    ], 6, 8);
  });

  test('050 stats', async ({ page }) => {
    runGoSeedTool('e2e-seed-history');
    await page.setViewportSize({ width: 1280, height: 2300 });
    await page.goto('/stats');
    await expect(page.locator('#stats-chart')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#hardest-body .sx-empty')).toHaveCount(0, { timeout: 10_000 });
    await page.waitForTimeout(2500); // Chart.js animation
    await shot(page, '050-stats/01-stats_training_history.png', [page.locator('.sx-box:has(#stats-chart)')]);
    await page.locator('#stats-table-toggle').click();
    await page.waitForTimeout(1500);
    await shot(page, '050-stats/02-stats_last_14_days.png', [page.locator('.sx-box:has(#stats-table-toggle)')]);
    // The due box stretches to the height of the Levels box next to it; let it
    // shrink to its own content so the forecast stands alone.
    await page.addStyleTag({ content: '.sx-due-box { align-self: start !important; }' });
    await page.waitForTimeout(1500);
    await shot(page, '050-stats/03-stats_words_by_duedate.png', [page.locator('.sx-due-box')]);
    // Top 8 rows of Hardest words / Most practiced.
    await shot(page, '050-stats/04-stats_words.png', [
      page.locator('.sx-box:has(#hardest-body) .sx-title'),
      page.locator('.sx-box:has(#most-practiced-body) .sx-title'),
      page.locator('#hardest-body > *').nth(7),
      page.locator('#most-practiced-body > *').nth(7),
    ], 10);
    await shot(page, '050-stats/05-stats_accuracy.png', [page.locator('.sx-box:has(#levels-bar)')]);
  });

  test('060 vocabulary and 070 import/export and 090 add word', async ({ page }) => {
    await page.goto('/vocab');
    await expect(page.locator('#words-tbody > *').first()).toBeVisible({ timeout: 10_000 });
    await shot(page, '060-vocabulary/01-list.png',
      [page.locator('h1').first(), page.locator('#words-tbody > *').nth(4)], 16);

    await page.locator('#vocab-menu-btn').click();
    await page.locator('#download-btn').click();
    await shot(page, '070-vendor-lock-in/01-download_words.png', [page.locator('#download-modal > div')]);
    await page.locator('#dl-cancel-btn').click();

    await page.locator('#vocab-menu-btn').click();
    await page.locator('#csv-upload-btn').click();
    await shot(page, '070-vendor-lock-in/02-upload_csv.png', [page.locator('#csv-upload-modal > div')]);
    await page.locator('#csv-upload-cancel-btn').click();

    await page.locator('#open-add-btn').click();
    await expect(page.locator('#word-form-panel')).toBeVisible();
    await shot(page, '090-llm-support/02-add_word.png', [page.locator('#word-form-panel')]);
  });

  test('100 mismatches', async ({ page }) => {
    await page.goto('/mismatches');
    await expect(page.locator('#mismatches-list')).toBeVisible({ timeout: 10_000 });
    await shot(page, '100-mismatches/01-mismatches.png',
      [page.locator('h1').first(), page.locator('#mismatches-list > *').nth(3)], 16);
  });

  test('080 settings and 090 api keys', async ({ page }) => {
    await patchSettings(page, {
      blur_pinyin: true, no_auto_voice_on_blur: true, celebrate_bucket_change: true,
      sentence_blank_enabled: true, sentence_blank_ratio: 80,
      gamification_enabled: true, gamification_frequency: 5,
      primary_lang: 'de', secondary_lang: 'en',
      // Varied training formats (the training shots above pinned one mode).
      prog_new: 'mask_pinyin', prog_tier_struggling: 'zh_pinyin_to_transl', prog_tier_learning: 'transl_to_zh',
      prog_tier_practicing: 'zh_to_transl', prog_tier_mastered: 'random',
      new_word_mode_0: 'mask_pinyin', new_word_mode_1: 'zh_pinyin_to_transl', new_word_mode_2: 'transl_to_zh',
      cycle_sequence: 'zh_pinyin_to_transl,mask_pinyin,zh_to_transl,transl_to_zh,zh_to_transl_no_sound,voice_to_transl',
      cycle_advance_on_known_only: true,
    });
    await page.goto('/settings');
    await expect(page.locator('#sec-training')).toBeVisible({ timeout: 10_000 });
    const sec = id => page.locator(`#${id}`);
    await shot(page, '080-settings/01-settings_words.png',
      [page.locator('#sec-training .st-row', { hasText: 'Require typing the Chinese word' }),
        page.locator('#sec-training .ui-seg').last()], 4);
    await shot(page, '080-settings/02-settings_cycle_mode.png', [sec('sec-cycle')]);
    await shot(page, '080-settings/03-settings_daily_learning.png', [sec('daily-learning-section')]);
    await shot(page, '080-settings/04-settings_gamification.png', [sec('sec-gamification')]);
    await shot(page, '080-settings/05-settings_language.png', [sec('sec-languages')]);
    await shot(page, '080-settings/06-settings_quiz_display.png',
      [sec('sec-training').locator('.st-title'), page.locator('#sec-training .st-row', { hasText: 'Frequency' })], 8);
    await shot(page, '080-settings/07-settings_training_mode.png', [sec('sec-progressive')]);
    await shot(page, '090-llm-support/01-settings_api_keys.png', [sec('apikey-section')]);
  });
});
