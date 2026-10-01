// @ts-check
import { test, expect } from '@playwright/test';
import { BASE_URL, ADMIN_EMAIL, ADMIN_PASSWORD } from './global-setup.js';
import { parseSetCookieHeaders, seedWord } from './helpers/api.js';
import { captureForPR } from './helpers/screenshot.js';

const PASSWORD = 'E2eOnboardingPass123!';

/** Register a brand-new user in the browser and land on /train. */
async function registerFreshUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => {
    route.fulfill({ status: 200, body: '' });
  });
  const email = `e2e-onboard-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
}

test.describe('First vocabulary setup wizard', () => {
  test('a new user walks through the 4 steps and reaches the first card', async ({ page }) => {
    await registerFreshUser(page);

    await expect(page.locator('#empty-state')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#ob-quickstart')).toBeVisible();

    // Step 1: HSK 3.0 is preselected and shows the live word count.
    await expect(page.locator('#wz-step')).toContainText('1 of 4');
    await expect(page.locator('#wz-version-3')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#wz-version-2')).toHaveAttribute('aria-pressed', 'false');
    await captureForPR(page, 'wizard-step1-version');
    await page.locator('#wz-next').click();

    // Step 2: the beginner band starts at its first level.
    await expect(page.locator('#wz-band-beg')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#wz-start-1')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#wz-below')).toBeHidden();
    await captureForPR(page, 'wizard-step2-level');
    await page.locator('#wz-next').click();

    // Step 3: topics are optional; nothing picked means Skip.
    await expect(page.locator('#wz-step')).toContainText('3 of 4');
    await expect(page.locator('#wz-next')).toHaveText('Skip');
    await captureForPR(page, 'wizard-step3-topics');
    await page.locator('#wz-next').click();

    // Step 4: defaults, then import.
    await expect(page.locator('#wz-pace-10')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#wz-lang-en')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#wz-lang-de')).toHaveAttribute('aria-pressed', 'false');
    await captureForPR(page, 'wizard-step4-pace');
    await expect(page.locator('#wz-next')).toContainText('5');
    await page.locator('#wz-next').click();

    await expect(page.locator('#wz-done')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#wz-done')).toContainText('5');
    await captureForPR(page, 'wizard-done');
    await page.locator('#wz-start-training').click();

    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#empty-state')).toBeHidden();

    const tags = await page.evaluate(() => fetch('/api/tags').then(r => r.json()));
    expect(tags.sort()).toEqual(['hsk3-1', 'hsk3-2', 'hsk3-3']);
    const st = await page.evaluate(() => fetch('/api/settings').then(r => r.json()));
    expect(st.max_new_words_per_day).toBe(10);
    expect(st.train_langs).toEqual(['en']);
  });

  test('words below the start level are marked as known and never quizzed', async ({ page }) => {
    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });

    await page.locator('#wz-version-2').click();
    await page.locator('#wz-next').click();
    await page.locator('#wz-start-2').click();
    await expect(page.locator('#wz-below')).toBeVisible();
    await expect(page.locator('#wz-below-known')).toHaveAttribute('aria-pressed', 'true');
    await captureForPR(page, 'wizard-step2-below-start');
    await page.locator('#wz-next').click();
    await page.locator('#wz-next').click();
    await page.locator('#wz-next').click();
    await expect(page.locator('#wz-done')).toBeVisible({ timeout: 15_000 });

    const res = await page.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
    const known = Object.fromEntries(res.words.map(w => [w.zh_text, w.known]));
    expect(known).toEqual({ '一': true, '人': true, '大': true, '时间': false, '已经': false });

    await page.locator('#wz-start-training').click();
    await expect(page.locator('#new-word-zh')).toHaveText(/时间|已经/, { timeout: 15_000 });
  });

  test('pace, languages, gamification and audio are saved as settings', async ({ page }) => {
    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });

    await page.locator('#wz-next').click();
    await page.locator('#wz-next').click();
    await page.locator('#wz-next').click();
    await page.locator('#wz-pace-20').click();
    await page.locator('#wz-lang-de').click();
    await page.locator('#wz-game-off').click();
    await page.locator('#wz-audio-on').click();
    // The last selected meaning language cannot be switched off.
    await page.locator('#wz-lang-en').click();
    await page.locator('#wz-lang-de').click();
    await expect(page.locator('#wz-lang-de')).toHaveAttribute('aria-pressed', 'true');
    await page.locator('#wz-lang-en').click();
    await expect(page.locator('#wz-lang-en')).toHaveAttribute('aria-pressed', 'true');
    await page.locator('#wz-next').click();
    await expect(page.locator('#wz-done')).toBeVisible({ timeout: 15_000 });
    // Let the debounced filter save from page load fire (500 ms) before reading.
    await page.waitForTimeout(1000);

    const st = await page.evaluate(() => fetch('/api/settings').then(r => r.json()));
    expect(st.max_new_words_per_day).toBe(20);
    expect(st.train_langs.sort()).toEqual(['de', 'en']);
    expect(st.gamification_enabled).toBe(false);
    expect(st.autoplay_always).toBe(true);

    await page.locator('#wz-start-training').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');
  });

  test('picked topics are added on top of the HSK list', async ({ page }) => {
    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#wz-next').click();
    await page.locator('#wz-next').click();

    await expect(page.locator('#wz-topics-label')).toHaveText('Topics');
    await page.locator('#wz-topic-food').click();
    await expect(page.locator('#wz-topic-food')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#wz-topics-label')).toHaveText('1 topic selected');
    await expect(page.locator('#wz-next')).toHaveText('Continue');

    // The search matches the English or the Chinese name.
    await page.locator('#wz-topic-search').fill('旅');
    await expect(page.locator('#wz-topic-travel')).toBeVisible();
    await expect(page.locator('#wz-topic-food')).toBeHidden();
    await page.locator('#wz-topic-search').fill('nomatch');
    await expect(page.locator('#wz-topic-none')).toContainText('nomatch');
    await page.locator('#wz-topic-search').fill('');
    await captureForPR(page, 'wizard-step3-topics-selected');

    await page.locator('#wz-next').click();
    await page.locator('#wz-next').click();
    await expect(page.locator('#wz-done')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#wz-done')).toContainText('Topics: Food');
    await captureForPR(page, 'wizard-done-with-topics');

    const res = await page.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
    const tagsByWord = Object.fromEntries(res.words.map(w => [w.zh_text, w.tags]));
    expect(tagsByWord['苹果']).toEqual(['topic-food']);
    expect(tagsByWord['面包']).toEqual(['topic-food']);
    expect(tagsByWord['一']).toEqual(['hsk3-1']);
    expect(res.words).toHaveLength(7);
  });

  test('a topic word below the start level stays known', async ({ page }) => {
    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#wz-next').click();
    await page.locator('#wz-start-2').click();
    await page.locator('#wz-next').click();
    await page.locator('#wz-topic-family').click();
    await page.locator('#wz-next').click();
    await page.locator('#wz-next').click();
    await expect(page.locator('#wz-done')).toBeVisible({ timeout: 15_000 });

    const res = await page.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
    const known = Object.fromEntries(res.words.map(w => [w.zh_text, w.known]));
    // 人 is in HSK 1 (marked known) and in the picked topic: it stays known.
    expect(known['人']).toBe(true);
    expect(known['大']).toBe(false);
  });

  test('"Import my own list instead" opens the library and the guided setup link returns', async ({ page }) => {
    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#ob-qs-custom').click();

    await expect(page.locator('#wz-lib-import')).toBeDisabled();
    await expect(page.locator('#wz-lib-count')).toHaveText('Nothing selected yet');
    await expect(page.locator('#wz-step')).toBeHidden();
    await captureForPR(page, 'library-empty');

    await page.locator('#wz-lib-back').click();
    await expect(page.locator('#wz-step')).toContainText('1 of 4');
  });

  test('the library imports HSK lists and topics together, each word keeping its own tags', async ({ page }) => {
    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#ob-qs-custom').click();

    await page.locator('#wz-sel-hsk3-1').click();
    await page.locator('#wz-sel-hsk2-2').click();
    await page.locator('#wz-sel-topic-food').click();
    await expect(page.locator('#wz-lib-count')).toHaveText('3 tags selected');
    await expect(page.locator('#wz-match-hint')).toContainText('at least one selected tag');
    await captureForPR(page, 'library-selection');

    await page.locator('#wz-lib-import').click();
    await expect(page.locator('#wz-lib-done')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#wz-lib-done')).toContainText('HSK 3.0 · 1');
    await expect(page.locator('#wz-lib-done')).toContainText('Food');
    await captureForPR(page, 'library-done');

    const res = await page.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
    const tagsByWord = Object.fromEntries(res.words.map(w => [w.zh_text, w.tags]));
    expect(tagsByWord).toEqual({
      '一': ['hsk3-1'],
      '人': ['hsk3-1'],
      '时间': ['hsk2-2'],
      '已经': ['hsk2-2'],
      '苹果': ['topic-food'],
      '面包': ['topic-food'],
    });

    await page.locator('#wz-lib-train').click();
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 15_000 });
  });

  test('"All tags" imports only words that have every selected tag', async ({ page }) => {
    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#ob-qs-custom').click();

    await page.locator('#wz-match-all').click();
    await expect(page.locator('#wz-match-hint')).toContainText('every selected tag');
    await page.locator('#wz-sel-hsk3-2').click();
    await page.locator('#wz-sel-topic-time').click();
    await captureForPR(page, 'library-match-all');
    await page.locator('#wz-lib-import').click();
    await expect(page.locator('#wz-lib-done')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#wz-lib-done')).toContainText('All tags');

    const res = await page.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
    expect(res.words.map(w => w.zh_text)).toEqual(['时间']);
    expect(res.words[0].tags.sort()).toEqual(['hsk3-2', 'topic-time']);
  });

  test('the library can clear the selection and change it after an import', async ({ page }) => {
    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#ob-qs-custom').click();

    await page.locator('#wz-sel-hsk3-1').click();
    await expect(page.locator('#wz-lib-import')).toBeEnabled();
    await page.locator('#wz-lib-clear').click();
    await expect(page.locator('#wz-lib-count')).toHaveText('Nothing selected yet');
    await expect(page.locator('#wz-lib-import')).toBeDisabled();

    await page.locator('#wz-sel-hsk3-1').click();
    await page.locator('#wz-lib-import').click();
    await expect(page.locator('#wz-lib-done')).toBeVisible({ timeout: 15_000 });
    await page.locator('#wz-lib-change').click();
    await expect(page.locator('#wz-sel-hsk3-1')).toHaveAttribute('aria-pressed', 'true');
  });

  // Issue #344: bulk-importing a large word list (e.g. 20+ HSK1 words) used
  // to flood the very first session by force-acknowledging every imported
  // word as immediately due/already-seen, instead of introducing them
  // gradually like manually-added words. This imports 25 words at once and
  // asserts that only the normal daily new-word cap's worth show up as due,
  // with the rest introduced gradually across later sessions/days.
  test('bulk import respects the daily new-word pacing cap', async ({ page }) => {
    const tag = `topic-e2epacing${Date.now()}`;
    const chars = ['二', '三', '四', '五', '六', '七', '八', '九', '十', '月',
      '日', '年', '水', '火', '山', '土', '木', '金', '风', '雨', '云', '雪', '星', '河', '湖'];

    const adminLoginRes = await fetch(`${BASE_URL}/api/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: ADMIN_EMAIL, password: ADMIN_PASSWORD }),
    });
    if (!adminLoginRes.ok) {
      throw new Error(`Admin login failed (${adminLoginRes.status}): ${await adminLoginRes.text()}`);
    }
    const adminCookies = parseSetCookieHeaders(adminLoginRes.headers.getSetCookie?.() ?? []);
    const adminCookieHeader = adminCookies.map(c => `${c.name}=${c.value}`).join('; ');

    for (const zh of chars) {
      await seedWord(BASE_URL, adminCookieHeader, { zh, pinyin: '', en: [`e2e-word-${zh}`], tags: [tag] }, false);
    }
    const tagRes = await fetch(`${BASE_URL}/api/tags/${tag}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Cookie: adminCookieHeader },
      body: JSON.stringify({ description: 'pacing test library', importable: true }),
    });
    if (!tagRes.ok) {
      throw new Error(`Marking ${tag} importable failed (${tagRes.status}): ${await tagRes.text()}`);
    }

    await registerFreshUser(page);
    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#ob-qs-custom').click();

    const tile = page.locator(`#wz-sel-${tag}`);
    await expect(tile).toBeVisible({ timeout: 10_000 });
    await tile.click();
    await page.locator('#wz-lib-import').click();
    await expect(page.locator('#wz-lib-done')).toBeVisible({ timeout: 15_000 });
    await page.locator('#wz-lib-train').click();

    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#empty-state')).toBeHidden();
    await captureForPR(page, 'onboarding-first-new-word-after-bulk-import');

    const stats = await page.evaluate(() => fetch('/api/quiz/stats').then(r => r.json()));
    expect(stats.due_today).toBeLessThanOrEqual(stats.max_new_per_day);
    expect(stats.due_today).toBeLessThan(20);
  });

  // Regression test for a reported bug: a brand-new account graduates its
  // very first word, then hits a dead end — the daily-new-word cooldown
  // (default 1 minute) blocks the next new word, the "learn more" advance
  // buttons stay disabled (they need 10+ already-seen words to ever enable),
  // and the "Introduce new words" button wasn't wired up on the code path
  // that renders after a failed /api/quiz/next fetch. New learners (few
  // introduced words) must not be cooldown-gated, and even if they were,
  // the success screen must always offer *some* way to keep going.
  test('a brand-new account can keep learning right after graduating its first word', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const email = `e2e-onboard-cooldown-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
    await page.goto('/#register');
    await page.locator('#reg-email').fill(email);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    await page.locator('#register-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    const word1Res = await page.request.post('/api/words', {
      data: { zh_text: '我', pinyin: 'wǒ', translations: { en: ['I'] }, tags: [], start_training: true },
    });
    expect(word1Res.ok()).toBe(true);
    const word2Res = await page.request.post('/api/words', {
      data: { zh_text: '你', pinyin: 'nǐ', translations: { en: ['you'] }, tags: [], start_training: false },
    });
    expect(word2Res.ok()).toBe(true);

    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });

    await page.goto('/train');

    // Wait for whichever of the due-word card or the new-word screen the app
    // lands on next (loadNextCard() is async after each transition).
    async function waitForCardOrNewWord() {
      await page.waitForFunction(() => {
        const card = document.getElementById('card-area');
        const nw = document.getElementById('new-word-area');
        return (card && !card.classList.contains('hidden')) || (nw && !nw.classList.contains('hidden'));
      }, { timeout: 12_000 });
    }

    // Answer the first (already-acknowledged) word correctly, repeating up to
    // 3 times (as it takes to graduate it out of today's queue per
    // session-end.spec.js) or until the second, never-seen word is offered —
    // whichever the SM2 due-date progression reaches first. While word1 is
    // still in the new-word intro phase, the configured zh_to_transl mode
    // doesn't apply (issue #435) — the intro ladder shows transl_to_zh first
    // (prompt is the translation "I", answer with the zh word), then
    // zh_to_transl once the word has enough correct answers to graduate.
    await waitForCardOrNewWord();
    for (let i = 0; i < 3 && !(await page.locator('#new-word-area').isVisible()); i++) {
      await expect(page.locator('#card-area')).toBeVisible();
      const prompt = await page.locator('#prompt-word').innerText();
      await page.locator('#answer-input').fill(prompt === '我' ? 'I' : '我');
      await page.locator('#answer-form button[type="submit"]').click();
      await expect(page.locator('#result-icon')).toHaveText('Correct', { timeout: 8_000 });
      await page.locator('#next-btn').click();
      await waitForCardOrNewWord();
    }

    // The second, never-seen word must be offered — not a dead-end success
    // screen with every button disabled.
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#new-word-zh')).toHaveText('你');
    await captureForPR(page, 'onboarding-second-word-after-first-graduation');
  });
});
