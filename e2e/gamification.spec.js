// @ts-check
import { test, expect } from '@playwright/test';
import { BASE_URL } from './global-setup.js';
import { captureForPR } from './helpers/screenshot.js';

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
        gamification_hide_pinyin_from_bucket: '70-84',
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

  // Issue #349: a select letting the user choose the minimum bucket at and
  // above which the match game stops showing pinyin, default "Practicing".
  test('settings page shows and saves the hide-pinyin-from-bucket select, default Practicing', async ({ page }) => {
    await page.goto(`${BASE_URL}/settings`);
    const select = page.locator('#gamification-hide-pinyin-bucket');
    await expect(select).toBeVisible();
    await expect(select).toHaveValue('70-84'); // Practicing

    await captureForPR(page, 'settings-gamification-hide-pinyin');

    await select.selectOption('85-100');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });

    await page.reload();
    await expect(page.locator('#gamification-hide-pinyin-bucket')).toHaveValue('85-100');

    // Restore the default so later specs see the standard state.
    await select.selectOption('70-84');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });
  });

  // Issue #349: once a word's SM-2 bucket reaches the configured threshold
  // (default Practicing), the server flags the tile hide_pinyin so the match
  // game doesn't show its pinyin hint up front — but the pinyin value is
  // still sent (issue #375), so it can be revealed once that pair is
  // attempted, per the match_game_pinyin_reveal setting. A word still below
  // the threshold isn't flagged and keeps showing its pinyin from the start,
  // unaffected by that setting. The server-side tier classification/cutoff
  // itself is covered by the Go handler tests
  // (TestMatchGame_HidesPinyinAtOrAboveDefaultThreshold et al. in
  // quiz_matchgame_test.go); this renders the exact shape GET
  // /api/quiz/match-game now returns for an at/above-threshold word
  // (hide_pinyin: true) and a below-threshold word (hide_pinyin: false) to
  // verify the UI reflects that flag rather than re-deriving tier state
  // end-to-end.
  test('match game hides pinyin for a word at/above the threshold bucket, keeps it below', async ({ page }) => {
    await page.goto(`${BASE_URL}/train`);
    const words = [
      { zh_word_id: 9001, zh_text: '会', pinyin: 'huì', hide_pinyin: true, translations: { en: ['can'] } },
      { zh_word_id: 9002, zh_text: '去', pinyin: 'qù', hide_pinyin: false, translations: { en: ['go'] } },
    ];
    await page.evaluate((w) => {
      // @ts-ignore
      window.showMatchGame(w);
    }, words);
    await expect(page.locator('#match-game-overlay')).toBeVisible();

    const huiBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('会', { exact: true }) });
    const quBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('去', { exact: true }) });
    // The at/above-threshold tile (会) starts hidden; the below-threshold
    // tile (去) shows its pinyin immediately, unaffected by this feature.
    await expect(huiBox.getByText('huì')).toHaveCount(0);
    await expect(quBox.getByText('qù')).toBeVisible();

    await captureForPR(page, 'match-game-pinyin-hidden-above-threshold');

    // Matching 会 correctly still reveals its pinyin — hide_pinyin only
    // withholds it up front, per the default "always" reveal setting.
    await huiBox.click();
    await page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('can', { exact: true }) }).click();
    await expect(huiBox.getByText('huì')).toBeVisible();
  });

  // Issue #375: a new gamification setting controlling when the match game
  // reveals a word tile's pinyin hint — off / always (default, pre-existing
  // behaviour) / only after a correct match.
  test('settings page shows and saves the match-game pinyin-reveal select, default Always', async ({ page }) => {
    await page.goto(`${BASE_URL}/settings`);
    const select = page.locator('#match-game-pinyin-reveal');
    await expect(select).toBeVisible();
    await expect(select).toHaveValue('always');
    await captureForPR(page, 'settings-match-game-pinyin-reveal');

    await select.selectOption('after_correct');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });

    await page.reload();
    await expect(page.locator('#match-game-pinyin-reveal')).toHaveValue('after_correct');

    // Restore the default so later specs see the standard state.
    await select.selectOption('always');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible({ timeout: 5000 });
  });

  // Issue #375: with the setting set to "off", pinyin must never appear, even
  // after a correct match.
  test('match game never shows pinyin when the reveal setting is "off"', async ({ page, request }) => {
    const settingsRes = await request.get(`${BASE_URL}/api/settings`);
    const originalSettings = await settingsRes.json();
    await api(request, 'PATCH', '/api/settings', { ...originalSettings, match_game_pinyin_reveal: 'off' });
    try {
      await page.goto(`${BASE_URL}/train`);
      const words = [
        { zh_word_id: 9101, zh_text: '猫', pinyin: 'māo', hide_pinyin: true, translations: { en: ['cat'] } },
        { zh_word_id: 9102, zh_text: '狗', pinyin: 'gǒu', hide_pinyin: true, translations: { en: ['dog'] } },
      ];
      await page.evaluate((w) => {
        // @ts-ignore
        window.showMatchGame(w);
      }, words);
      await expect(page.locator('#match-game-overlay')).toBeVisible();

      const maoBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('猫', { exact: true }) });
      await expect(maoBox.getByText('māo')).toHaveCount(0);

      // Correctly match 猫 → cat; pinyin must still not appear afterwards.
      await maoBox.click();
      await page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('cat', { exact: true }) }).click();
      await expect(maoBox.getByText('māo')).toHaveCount(0);

      await page.locator('#match-game-overlay button', { hasText: 'Skip game' }).click();
    } finally {
      await api(request, 'PATCH', '/api/settings', originalSettings);
    }
  });

  // Issue #375: with the setting set to "after_correct", pinyin is hidden
  // until the pair is matched correctly, then appears.
  test('match game reveals pinyin only after a correct match when the reveal setting is "after_correct"', async ({ page, request }) => {
    const settingsRes = await request.get(`${BASE_URL}/api/settings`);
    const originalSettings = await settingsRes.json();
    await api(request, 'PATCH', '/api/settings', { ...originalSettings, match_game_pinyin_reveal: 'after_correct' });
    try {
      await page.goto(`${BASE_URL}/train`);
      const words = [
        { zh_word_id: 9201, zh_text: '猫', pinyin: 'māo', hide_pinyin: true, translations: { en: ['cat'] } },
        { zh_word_id: 9202, zh_text: '狗', pinyin: 'gǒu', hide_pinyin: true, translations: { en: ['dog'] } },
      ];
      await page.evaluate((w) => {
        // @ts-ignore
        window.showMatchGame(w);
      }, words);
      await expect(page.locator('#match-game-overlay')).toBeVisible();

      const maoBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('猫', { exact: true }) });
      await expect(maoBox.getByText('māo')).toHaveCount(0);

      await captureForPR(page, 'match-game-pinyin-reveal-after-correct-hidden');

      // Correctly match 猫 → cat; pinyin should now appear.
      await maoBox.click();
      await page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('cat', { exact: true }) }).click();
      await expect(maoBox.getByText('māo')).toBeVisible();

      await captureForPR(page, 'match-game-pinyin-reveal-after-correct-shown');

      await page.locator('#match-game-overlay button', { hasText: 'Skip game' }).click();
    } finally {
      await api(request, 'PATCH', '/api/settings', originalSettings);
    }
  });

  // Issue #375 (bugfix): with the setting set to "after_correct", a wrong
  // match must not reveal pinyin — only a correct one does.
  test('match game does not reveal pinyin on a wrong match when the reveal setting is "after_correct"', async ({ page, request }) => {
    const settingsRes = await request.get(`${BASE_URL}/api/settings`);
    const originalSettings = await settingsRes.json();
    await api(request, 'PATCH', '/api/settings', { ...originalSettings, match_game_pinyin_reveal: 'after_correct' });
    try {
      await page.goto(`${BASE_URL}/train`);
      const words = [
        { zh_word_id: 9301, zh_text: '猫', pinyin: 'māo', hide_pinyin: true, translations: { en: ['cat'] } },
        { zh_word_id: 9302, zh_text: '狗', pinyin: 'gǒu', hide_pinyin: true, translations: { en: ['dog'] } },
      ];
      await page.evaluate((w) => {
        // @ts-ignore
        window.showMatchGame(w);
      }, words);
      await expect(page.locator('#match-game-overlay')).toBeVisible();

      const maoBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('猫', { exact: true }) });
      await maoBox.click();
      await page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('dog', { exact: true }) }).click();
      await expect(maoBox.getByText('māo')).toHaveCount(0);

      await page.locator('#match-game-overlay button', { hasText: 'Skip game' }).click();
    } finally {
      await api(request, 'PATCH', '/api/settings', originalSettings);
    }
  });

  // Issue #375 (bugfix): with the setting set to "always" (the default),
  // pinyin is hidden until a pair is attempted, then revealed regardless of
  // whether that attempt was right or wrong.
  test('match game reveals pinyin after any attempt (right or wrong) when the reveal setting is "always"', async ({ page, request }) => {
    const settingsRes = await request.get(`${BASE_URL}/api/settings`);
    const originalSettings = await settingsRes.json();
    await api(request, 'PATCH', '/api/settings', { ...originalSettings, match_game_pinyin_reveal: 'always' });
    try {
      await page.goto(`${BASE_URL}/train`);
      const words = [
        { zh_word_id: 9401, zh_text: '猫', pinyin: 'māo', hide_pinyin: true, translations: { en: ['cat'] } },
        { zh_word_id: 9402, zh_text: '狗', pinyin: 'gǒu', hide_pinyin: true, translations: { en: ['dog'] } },
      ];
      await page.evaluate((w) => {
        // @ts-ignore
        window.showMatchGame(w);
      }, words);
      await expect(page.locator('#match-game-overlay')).toBeVisible();

      const maoBox = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('猫', { exact: true }) });
      // Not shown up front — only after the pair is attempted.
      await expect(maoBox.getByText('māo')).toHaveCount(0);

      // A wrong match still reveals the pinyin under "always".
      await maoBox.click();
      await page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('dog', { exact: true }) }).click();
      await expect(maoBox.getByText('māo')).toBeVisible();

      await page.locator('#match-game-overlay button', { hasText: 'Skip game' }).click();
    } finally {
      await api(request, 'PATCH', '/api/settings', originalSettings);
    }
  });

  // Issue #375 (bugfix): component tiles (used by Mismatch mode) are never
  // tier-blanked server-side (see hidePinyinAboveThreshold) and always show
  // their pinyin hint from the start, regardless of match_game_pinyin_reveal.
  test('match game always shows a component tile\'s pinyin from the start, even with the reveal setting "off"', async ({ page, request }) => {
    const settingsRes = await request.get(`${BASE_URL}/api/settings`);
    const originalSettings = await settingsRes.json();
    await api(request, 'PATCH', '/api/settings', { ...originalSettings, match_game_pinyin_reveal: 'off' });
    try {
      await page.goto(`${BASE_URL}/train`);
      const words = [
        { kind: 'component', character: '亻', zh_text: '亻', pinyin: 'rén', translations: { en: ['person radical'] } },
        { kind: 'component', character: '氵', zh_text: '氵', pinyin: 'shuǐ', translations: { en: ['water radical'] } },
      ];
      await page.evaluate((w) => {
        // @ts-ignore
        window.showMatchGame(w);
      }, words);
      await expect(page.locator('#match-game-overlay')).toBeVisible();

      const box = page.locator('#match-game-overlay .rounded-xl').filter({ has: page.getByText('亻', { exact: true }) });
      await expect(box.getByText('rén')).toBeVisible();

      await page.locator('#match-game-overlay button', { hasText: 'Skip game' }).click();
    } finally {
      await api(request, 'PATCH', '/api/settings', originalSettings);
    }
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
  // Issue #350: a word shown in "newest words" match-game mode must not
  // reappear until the user makes an error answering it in normal training —
  // it previously had no anti-repeat bookkeeping at all and could reappear
  // on the very next round. Proven deterministically against the running
  // server/DB (per CLAUDE.md's E2E-first cycle, preferred here over a
  // probabilistic UI-driven repro) rather than only at the DB-query layer,
  // since this exercises the real handler wiring end-to-end.
  test('newest-mode match game does not repeat a word until it is answered wrong (issue #350)', async ({ request }) => {
    // Isolate this test to only the "newest" mode so mismatch/hardest/
    // last_mistakes candidates can't interfere with the round contents.
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
        gamification_enabled: true,
        gamification_frequency: 1,
        game_mode_mismatch: false,
        game_mode_newest: true,
        game_mode_hardest: false,
        game_mode_last_mistakes: false,
      },
    });

    const wA = await api(request, 'POST', '/api/words', {
      zh_text: '买牛奶', pinyin: 'mǎi niú nǎi', translations: { en: ['buy milk'] }, tags: [], start_training: true,
    });
    const wB = await api(request, 'POST', '/api/words', {
      zh_text: '喝水', pinyin: 'hē shuǐ', translations: { en: ['drink water'] }, tags: [], start_training: true,
    });

    try {
      // First round: both freshly-created words are eligible and shown.
      const round1 = await api(request, 'GET', '/api/quiz/match-game');
      const ids1 = round1.words.map((w) => w.zh_word_id);
      expect(ids1).toContain(wA.id);
      expect(ids1).toContain(wB.id);

      // Second round, immediately after: both must now be suppressed.
      const round2 = await api(request, 'GET', '/api/quiz/match-game');
      const ids2 = round2.words.map((w) => w.zh_word_id);
      expect(ids2).not.toContain(wA.id);
      expect(ids2).not.toContain(wB.id);

      // Shown/wrong-answer timestamps have second granularity, so wait past
      // the second round1's markShown landed in — otherwise a wrong answer
      // recorded in the very same second would tie rather than strictly
      // exceed last_shown_in_game and the re-eligibility check would never
      // pass (mirrors the same granularity in hardest/last_mistakes mode).
      await new Promise((resolve) => setTimeout(resolve, 1100));

      // Answer wA wrong in *normal* training — the qualifying re-eligibility
      // event the issue asks for. wB is answered correctly and must stay
      // suppressed.
      await api(request, 'POST', '/api/quiz/answer', {
        word_id: wA.id, mode: 'zh_to_transl', answer: 'definitely not milk',
      });
      await api(request, 'POST', '/api/quiz/answer', {
        word_id: wB.id, mode: 'zh_to_transl', answer: 'drink water',
      });

      // wA alone would be a single eligible candidate at this point, below
      // matchGameMinCandidates (2) — the mode would never trigger and every
      // round would come back empty regardless of the fix. Add a second,
      // never-shown word so the mode can actually trigger, then poll a
      // bounded number of times until wA is drawn (re-eligible words re-enter
      // the weighted-random pool rather than being guaranteed on the very
      // next round) while asserting wB never appears in the meantime.
      const wC = await api(request, 'POST', '/api/words', {
        zh_text: '吃饭', pinyin: 'chī fàn', translations: { en: ['eat a meal'] }, tags: [], start_training: true,
      });
      try {
        let sawA = false;
        for (let i = 0; i < 30 && !sawA; i++) {
          const round = await api(request, 'GET', '/api/quiz/match-game');
          const ids = round.words.map((w) => w.zh_word_id);
          expect(ids).not.toContain(wB.id);
          if (ids.includes(wA.id)) sawA = true;
        }
        expect(sawA).toBe(true);
      } finally {
        await request.delete(`${BASE_URL}/api/words/${wC.id}`);
      }
    } finally {
      await request.delete(`${BASE_URL}/api/words/${wA.id}`);
      await request.delete(`${BASE_URL}/api/words/${wB.id}`);
    }
  });

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
