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

test.describe('One-button onboarding', () => {
  test('new user sees the quick-start level chooser and reaches the first card in one click', async ({ page }) => {
    await registerFreshUser(page);

    await expect(page.locator('#empty-state')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#ob-quickstart')).toBeVisible();
    // The detailed tag picker stays hidden behind the "choose myself" option.
    await expect(page.locator('#ob-step1')).toBeHidden();

    await page.locator('#ob-qs-hsk1').click();

    // Import runs, then the first card appears — for a fresh user that is
    // the new-word introduction screen.
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#new-word-zh')).not.toBeEmpty();
    await expect(page.locator('#empty-state')).toBeHidden();
  });

  test('the custom option reveals the existing tag picker', async ({ page }) => {
    await registerFreshUser(page);

    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#ob-qs-custom').click();

    await expect(page.locator('#ob-step1')).toBeVisible();
    await expect(page.locator('#ob-quickstart')).toBeHidden();
  });

  // Issue #344: bulk-importing a large word list (e.g. 20+ HSK1 words) used
  // to flood the very first session by force-acknowledging every imported
  // word as immediately due/already-seen, instead of introducing them
  // gradually like manually-added words. This imports 25 words at once and
  // asserts that only the normal daily new-word cap's worth show up as due,
  // with the rest introduced gradually across later sessions/days.
  test('bulk import respects the daily new-word pacing cap', async ({ page }) => {
    const tag = `e2e_pacing_${Date.now()}`;
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
    await expect(page.locator('#ob-step1')).toBeVisible();

    const tagPill = page.locator('#ob-tag-list button', { hasText: tag });
    await expect(tagPill).toBeVisible({ timeout: 10_000 });
    await tagPill.click();
    await expect(page.locator('#ob-next-btn')).toBeEnabled({ timeout: 10_000 });
    await page.locator('#ob-next-btn').click();

    await expect(page.locator('#ob-step2')).toBeVisible();
    await page.locator('#ob-next2-btn').click();

    await expect(page.locator('#ob-step3')).toBeVisible();
    // No more "how many words to start with" bypass — importing 25 words no
    // longer offers to force-acknowledge them all at once (issue #344).
    await captureForPR(page, 'onboarding-custom-import-step3');
    await page.locator('#ob-submit-btn').click();

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
    // whichever the SM2 due-date progression reaches first.
    await waitForCardOrNewWord();
    for (let i = 0; i < 3 && !(await page.locator('#new-word-area').isVisible()); i++) {
      await expect(page.locator('#card-area')).toBeVisible();
      await page.locator('#answer-input').fill('I');
      await page.locator('#answer-form button[type="submit"]').click();
      await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 8_000 });
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
