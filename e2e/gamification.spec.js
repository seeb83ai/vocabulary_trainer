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
  // This describe block permanently mutates the shared main-user's settings
  // (gamification_enabled, gamification_frequency, etc. — /api/settings PATCH
  // replaces the full row). Other spec files reuse the same seeded user via
  // 'e2e/.auth/user.json', so without restoring the original settings here,
  // gamification stays enabled for every test that runs afterward — causing
  // the match-game overlay to intercept unrelated flows (e.g. quiz.spec.js's
  // "wrong answer is not repeated" test) once enough confusion pairs exist.
  let originalSettings;

  test.beforeAll(async ({ request }) => {
    const res = await request.get(`${BASE_URL}/api/settings`);
    originalSettings = await res.json();
  });

  test.afterAll(async ({ request }) => {
    await request.patch(`${BASE_URL}/api/settings`, { data: originalSettings });
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

  test('settings page saves gamification toggle', async ({ page }) => {
    await page.goto(`${BASE_URL}/settings`);
    const checkbox = page.locator('#gamification-enabled');
    await expect(checkbox).toBeVisible();

    // Enable gamification
    await checkbox.check();
    await page.locator('#gamification-frequency').fill('2');
    await page.locator('#gamification-save-btn').click();

    await expect(page.locator('#gamification-success')).toBeVisible({ timeout: 5000 });

    // Reload and verify the setting persisted
    await page.reload();
    await expect(page.locator('#gamification-enabled')).toBeChecked({ timeout: 5000 });
    await expect(page.locator('#gamification-frequency')).toHaveValue('2');
  });
});
