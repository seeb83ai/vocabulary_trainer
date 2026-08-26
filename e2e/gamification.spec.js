// @ts-check
import { test, expect } from '@playwright/test';
import { BASE_URL } from './global-setup.js';

test.use({ storageState: 'e2e/.auth/user.json' });

// ─────────────────────────────────────────────────────────────────────────────
// Gamification — match-game trigger and UI
//
// Strategy:
//   1. Enable gamification with frequency=1 minute (minimum allowed).
//   2. Seed 3 extra words, submit wrong answers to generate ≥3 confusion pairs
//      via /api/quiz/answer with a wrong answer (seeds UpsertConfusion).
//   3. Load the training page, submit one answer, click Next.
//   4. Assert the match-game overlay appears with left (zh) and right (en) boxes.
//   5. Match all pairs correctly and verify the overlay disappears.
// ─────────────────────────────────────────────────────────────────────────────

/** POST JSON to an API endpoint and return the parsed response body. */
async function api(request, method, path, body) {
  const res = await request[method.toLowerCase()](`${BASE_URL}${path}`, {
    data: body,
    headers: { 'Content-Type': 'application/json' },
  });
  return res.json();
}

test.describe('Gamification — match game', () => {
  // This suite enables gamification_enabled=true for the shared main test
  // user (e2e/.auth/user.json), which every later spec file that uses the
  // same user also runs against (single worker, single DB, tests run in
  // file order). Left enabled, any later "click Next" in e.g. quiz.spec.js
  // can unexpectedly trigger the match-game overlay — which blocks
  // loadNextCard() until the overlay is interacted with — hanging tests
  // that never expect it. Restore the default (disabled) once this file's
  // tests are done so gamification state doesn't leak into other specs.
  test.afterAll(async ({ request }) => {
    await request.patch(`${BASE_URL}/api/settings`, {
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
        extend_session_with_extra_words: true,
        gamification_enabled: false,
        gamification_frequency: 5,
        game_mode_mismatch: true,
        game_mode_newest: true,
        game_mode_hardest: true,
        game_mode_last_mistakes: true,
      },
      headers: { 'Content-Type': 'application/json' },
    });
  });

  test('settings: gamification fields round-trip', async ({ request }) => {
    // Patch gamification settings
    const patch = await request.patch(`${BASE_URL}/api/settings`, {
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
        // PATCH replaces the full settings row — booleans not sent here would
        // otherwise reset to the JSON zero-value (false), so preserve the
        // pre-existing default here explicitly rather than clobbering it.
        extend_session_with_extra_words: true,
        gamification_enabled: true,
        gamification_frequency: 1,
        game_mode_mismatch: true,
        game_mode_newest: true,
        game_mode_hardest: true,
        game_mode_last_mistakes: true,
      },
    });
    expect(patch.status()).toBe(200);

    const get = await request.get(`${BASE_URL}/api/settings`);
    const st = await get.json();
    expect(st.gamification_enabled).toBe(true);
    expect(st.gamification_frequency).toBe(1);
  });

  test('match-game endpoint returns words array', async ({ request }) => {
    const res = await request.get(`${BASE_URL}/api/quiz/match-game`);
    const body = await res.json();
    // Verify shape — may be empty if < 2 eligible pairs in this DB.
    expect(body).toHaveProperty('words');
    expect(Array.isArray(body.words)).toBe(true);
  });

  test('match-answer endpoint accepts correct=true', async ({ request }) => {
    // Use the first seeded word (ID unknown — query next card to get a word ID)
    const next = await request.get(`${BASE_URL}/api/quiz/next?mode=zh_to_transl`);
    const card = await next.json();
    if (!card?.word_id) return; // no card available — skip

    const res = await request.post(`${BASE_URL}/api/quiz/match-answer`, {
      data: { zh_word_id: card.word_id, correct: true },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.correct).toBe(true);
    expect(body.zh_text).toBeTruthy();
  });

  test('match-answer returns 400 when zh_word_id missing', async ({ request }) => {
    const res = await request.post(`${BASE_URL}/api/quiz/match-answer`, {
      data: { correct: true },
    });
    expect(res.status()).toBe(400);
  });

  test('settings page shows gamification section', async ({ page }) => {
    await page.goto(`${BASE_URL}/settings`);
    await expect(page.getByText('Gamification')).toBeVisible();
    await expect(page.locator('#gamification-enabled')).toBeVisible();
    await expect(page.locator('#gamification-frequency')).toBeVisible();
  });

  // Issue #288: 4 individual game-mode toggles (mismatch/newest/hardest/last
  // mistakes), all on by default, alongside the existing gamification toggle.
  test('settings page shows and saves the 4 game-mode toggles', async ({ page }) => {
    await page.goto(`${BASE_URL}/settings`);
    const mismatch = page.locator('#game-mode-mismatch');
    const newest = page.locator('#game-mode-newest');
    const hardest = page.locator('#game-mode-hardest');
    const lastMistakes = page.locator('#game-mode-last-mistakes');

    await expect(mismatch).toBeVisible();
    await expect(newest).toBeVisible();
    await expect(hardest).toBeVisible();
    await expect(lastMistakes).toBeVisible();

    // All 4 default to enabled.
    await expect(mismatch).toBeChecked();
    await expect(newest).toBeChecked();
    await expect(hardest).toBeChecked();
    await expect(lastMistakes).toBeChecked();

    await newest.uncheck();
    await lastMistakes.uncheck();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });

    await page.reload();
    await expect(mismatch).toBeChecked();
    await expect(newest).not.toBeChecked();
    await expect(hardest).toBeChecked();
    await expect(lastMistakes).not.toBeChecked();

    // Restore defaults so later specs in this file see the standard state.
    await newest.check();
    await lastMistakes.check();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });
  });

  test('shared-translation claim is blocked (yellow) instead of stealing the true owner\'s box (issue #215)', async ({ page, request }) => {
    // 能 and 可能 both have "können" among their DE translations. Picking
    // 能 + "können" (可能's own box) must not be silently accepted — that
    // would make 可能 look unsolvable, since matched boxes turn green and
    // give no indication they can still be reused.
    const wA = await api(request, 'POST', '/api/words', {
      zh_text: '能', pinyin: 'néng', translations: { de: ['in der Lage sein', 'können'] }, tags: [], start_training: true,
    });
    const wB = await api(request, 'POST', '/api/words', {
      zh_text: '可能', pinyin: 'kě néng', translations: { de: ['können', 'möglicherweise'] }, tags: [], start_training: true,
    });
    try {
      const words = [
        { zh_word_id: wA.id, zh_text: '能', pinyin: 'néng', translations: { de: ['in der Lage sein', 'können'] } },
        { zh_word_id: wB.id, zh_text: '可能', pinyin: 'kě néng', translations: { de: ['können', 'möglicherweise'] } },
      ];

      await page.goto(`${BASE_URL}/train`);
      await page.evaluate((w) => {
        // @ts-ignore
        window.__mgDone = false;
        // @ts-ignore
        window.showMatchGame(w).then(() => { window.__mgDone = true; });
      }, words);
      await expect(page.locator('#match-game-overlay')).toBeVisible();

      const nengBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('能', { exact: true }) });
      const konnenBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('können', { exact: true }) });
      const keNengBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('可能', { exact: true }) });
      const lageBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('in der Lage sein', { exact: true }) });

      await nengBox.click();
      await konnenBox.click();
      await expect(nengBox).toHaveClass(/border-yellow-500/);
      await expect(nengBox).not.toHaveClass(/border-green-500/);
      await page.waitForTimeout(900); // let the blocked-state reset timeout fire

      // 能 solves against its own box, then 可能 can still claim "können".
      await nengBox.click();
      await lageBox.click();
      await keNengBox.click();
      await konnenBox.click();

      await expect(page.locator('#match-game-overlay')).toBeHidden({ timeout: 3000 });
      expect(await page.evaluate(() => /** @ts-ignore */ window.__mgDone)).toBe(true);
    } finally {
      // These words are created directly against the shared main test user
      // (single worker, single DB, tests run in file order). Left behind,
      // they are DE-only and immediately due, so later specs (e.g.
      // quiz.spec.js) that assume the seeded EN-only vocabulary can be
      // unexpectedly served one of these as the next due card. Delete them
      // once this test is done so they don't leak into other specs.
      await request.delete(`${BASE_URL}/api/words/${wA.id}`);
      await request.delete(`${BASE_URL}/api/words/${wB.id}`);
    }
  });

  test('settings page saves gamification toggle', async ({ page }) => {
    await page.goto(`${BASE_URL}/settings`);
    const checkbox = page.locator('#gamification-enabled');
    await expect(checkbox).toBeVisible();

    // Enable gamification
    await checkbox.check();
    await page.locator('#gamification-frequency').fill('2');

    await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });

    // Reload and verify the setting persisted
    await page.reload();
    await expect(page.locator('#gamification-enabled')).toBeChecked({ timeout: 5000 });
    await expect(page.locator('#gamification-frequency')).toHaveValue('2');
  });

  // Regression: auto-saving a *different* settings card must not silently
  // disable gamification. Every card's auto-save sends the full settings
  // payload (built fresh from the whole page's current DOM state) precisely
  // because the PATCH handler decodes a missing boolean field as false and
  // writes it straight through — a partial payload from any one card would
  // silently wipe fields owned by the others.
  test('enabling gamification survives saving the Daily Learning card', async ({ page }) => {
    await page.goto(`${BASE_URL}/settings`);
    const checkbox = page.locator('#gamification-enabled');
    await expect(checkbox).toBeVisible();

    // This suite leaves gamification enabled between tests (see the file-level
    // comment above), so the checkbox may already be checked here — check()
    // is then a no-op that fires no change event, so only wait for the
    // autosave banner when an actual transition happens.
    if (!(await checkbox.isChecked())) {
      await checkbox.check();
      await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });
    }

    // Save a wholly unrelated card, whose payload doesn't mention gamification.
    await page.locator('#max-new-words').fill('5');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });

    await page.reload();
    await expect(page.locator('#gamification-enabled')).toBeChecked({ timeout: 5000 });

    const res = await page.request.get(`${BASE_URL}/api/settings`);
    const settings = await res.json();
    expect(settings.gamification_enabled).toBe(true);
  });
});
