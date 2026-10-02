// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';

// Redesigned Train page (app redesign handoff): sticky session bar with a
// session sheet, tier chip on the question card, result screens with a
// "More info" box and mix-up layout, and the all-done week grid.
// Every test registers its own user so the shared e2e user is untouched.

const PASSWORD = 'E2eTrainRedesign123!';

async function registerUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
  const email = `e2e-train-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
}

async function seed(page, zh, pinyin, en, tags = []) {
  const res = await page.request.post('/api/words', {
    data: { zh_text: zh, pinyin, translations: { en }, tags, start_training: true },
  });
  expect(res.ok()).toBe(true);
  return (await res.json()).id;
}

async function enableGamification(page) {
  const st = await (await page.request.get('/api/settings')).json();
  const res = await page.request.patch('/api/settings', { data: { ...st, gamification_enabled: true } });
  expect(res.ok()).toBe(true);
}

async function useMode(page, mode) {
  await page.request.patch('/api/training-filters', {
    data: { mode, langs: ['en'], bucket: '', mnemonics: true, components: false, tags: [] },
  });
  await syncNewWordMode(page, mode);
}

test.describe('Train redesign – desktop', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('session bar shows progress and opens the session sheet with every filter', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello', 'hi'], ['greetings']);
    await seed(page, '再见', 'zài jiàn', ['bye', 'goodbye'], ['greetings']);
    await useMode(page, 'zh_to_transl');
    // The tier chip is a gamification element.
    await enableGamification(page);
    await page.goto('/train');

    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    const bar = page.locator('#session-bar');
    await expect(bar).toBeVisible();
    await expect(page.locator('#session-progress-label')).toHaveText(/^0 of \d+ today$/);
    await expect(bar.locator('#autoplay-toggle-btn')).toBeVisible();
    await expect(page.locator('#session-chip-label')).toContainText('Chinese');
    await expect(page.locator('#card-tier')).toBeVisible();
    await captureForPR(page, 'train-question');

    await page.locator('#open-filter-overlay').click();
    const sheet = page.locator('#filter-overlay');
    await expect(sheet).toBeVisible();
    await expect(sheet.locator('.overlay-mode-btn')).toHaveCount(8);
    await expect(sheet.locator('.overlay-tier-btn')).toHaveCount(6);
    await expect(page.locator('#overlay-tags-section')).toBeVisible();
    await expect(page.locator('#overlay-tag-chips')).toContainText('greetings');
    await captureForPR(page, 'train-session-sheet');

    await page.locator('#filter-overlay-close').click();
    await expect(sheet).toBeHidden();
  });

  test('a correct answer shows the word, a folded More info box with measure word and example', async ({ page }) => {
    await registerUser(page);
    await seed(page, '客人', 'kèrén', ['guest', 'CL:位[wei4]', 'Bsp.: 新来的客人 -- newly arrived guest']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');

    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.locator('#answer-input').fill('guest');
    await page.locator('#answer-form button[type="submit"]').click();

    await expect(page.locator('#result-icon')).toHaveText('Correct', { timeout: 8_000 });
    const toggle = page.locator('#result-more-info-toggle');
    await expect(toggle).toBeVisible();
    await expect(toggle).toContainText('Measure word');
    await expect(page.locator('#result-more-info-body')).toBeHidden();

    await toggle.click();
    const body = page.locator('#result-more-info-body');
    await expect(body).toBeVisible();
    await expect(body).toContainText('位');
    await expect(body).toContainText('wèi');
    await expect(body).toContainText('新来的客人');
    await expect(body).toContainText('newly arrived guest');
    await captureForPR(page, 'train-result-correct-more-info');
  });

  test('typing the meaning of another word shows the mix-up layout with the typed meaning highlighted', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello', 'hi']);
    await seed(page, '再见', 'zài jiàn', ['bye', 'goodbye']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');

    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    const prompt = (await page.locator('#prompt-word').textContent())?.trim();
    const typed = prompt === '你好' ? 'bye' : 'hello';
    await page.locator('#answer-input').fill(typed);
    await page.locator('#answer-form button[type="submit"]').click();

    await expect(page.locator('#result-icon')).toHaveText('That’s a different word', { timeout: 8_000 });
    const mixup = page.locator('#result-mixup');
    await expect(mixup).toBeVisible();
    await expect(mixup.locator('.mixup-asked')).toContainText(prompt || '');
    await expect(mixup.locator('.mixup-typed')).toHaveText(typed);
    await captureForPR(page, 'train-result-mixup');
  });

  test('all done screen shows the week grid and the learn-more options', async ({ page }) => {
    await registerUser(page);
    await seed(page, '好', 'hǎo', ['good']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');

    for (let i = 0; i < 3; i++) {
      await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
      await page.locator('#answer-input').fill('good');
      await page.locator('#answer-form button[type="submit"]').click();
      await expect(page.locator('#result-icon')).toHaveText('Correct', { timeout: 8_000 });
      await page.locator('#next-btn').click();
    }
    await expect(page.locator('#success-state')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#success-week > *')).toHaveCount(7);
    await expect(page.locator('.advance-btn')).toHaveCount(3);
    await captureForPR(page, 'train-all-done');
  });

  test('a failed card load shows the error card and Try again reloads', async ({ page }) => {
    await registerUser(page);
    await seed(page, '好', 'hǎo', ['good']);
    await useMode(page, 'zh_to_transl');
    await page.route('**/api/quiz/next**', route =>
      route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"boom"}' }));
    await page.goto('/train');

    await expect(page.locator('#error-state')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#error-retry-btn')).toBeVisible();
    await captureForPR(page, 'train-error');

    await page.unroute('**/api/quiz/next**');
    await page.locator('#error-retry-btn').click();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
  });
});

test.describe('Train redesign – new word and match game', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('a new word shows numbered meanings and folds the rest', async ({ page }) => {
    await registerUser(page);
    const res = await page.request.post('/api/words', {
      data: {
        zh_text: '年', pinyin: 'nián',
        translations: { en: ['year', 'annual', 'age', 'New Year', 'Bsp.: 我学了一年中文。 -- I studied Chinese for a year.'], de: ['Jahr'] },
        tags: [], start_training: false,
      },
    });
    expect(res.ok()).toBe(true);
    await page.request.patch('/api/training-filters', {
      data: { mode: 'progressive', langs: ['en', 'de'], bucket: '', mnemonics: true, components: false, tags: [] },
    });
    const st = await (await page.request.get('/api/settings')).json();
    await page.request.patch('/api/settings', { data: { ...st, secondary_lang: 'de' } });
    await page.goto('/train');

    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 12_000 });
    const rows = page.locator('#new-word-en .tr-meaning-row');
    await expect(rows).toHaveCount(3);
    await expect(rows.first()).toContainText('year · Jahr');
    const tail = page.locator('#new-word-en [data-disclosure="new-word-tail"]');
    await expect(tail).toContainText('1 meaning · 1 example');
    await tail.click();
    await expect(page.locator('#new-word-tail')).toContainText('我学了一年中文。');
    await captureForPR(page, 'train-new-word');
  });

  // Issue #507: the match game shows the same first translation language as
  // the quiz card (primary language first), not the alphabetically-first one.
  test('the match game shows the primary-language translation first', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.evaluate(() => {
      // @ts-ignore
      window.showMatchGame([
        { zh_word_id: 9611, zh_text: '塔', pinyin: 'tǎ', translations: { de: ['Turm'], en: ['tower'] } },
        { zh_word_id: 9612, zh_text: '狗', pinyin: 'gǒu', translations: { de: ['Hund'], en: ['dog'] } },
      ]);
    });
    const game = page.locator('#match-game-overlay');
    await expect(game.locator('.mg-tile', { hasText: 'tower' })).toBeVisible();
    await expect(game.locator('.mg-tile', { hasText: 'Turm' })).toHaveCount(0);
    await captureForPR(page, 'train-match-game-primary-lang');
  });

  test('the match game renders inline and shows a done view', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.evaluate(() => {
      // @ts-ignore
      window.__mgDone = false;
      // @ts-ignore
      window.showMatchGame([
        { zh_word_id: 9601, zh_text: '猫', pinyin: 'māo', translations: { en: ['cat'] } },
        { zh_word_id: 9602, zh_text: '狗', pinyin: 'gǒu', translations: { en: ['dog'] } },
      ]).then(() => { window.__mgDone = true; });
    });
    const game = page.locator('#match-game-overlay');
    await expect(game).toBeVisible();
    await expect(game.locator('.mg-eyebrow')).toHaveText('Match game');
    await captureForPR(page, 'train-match-game');

    // Either column can be tapped first.
    await game.locator('.mg-tile', { hasText: 'dog' }).click();
    await game.locator('.mg-tile', { hasText: '狗' }).click();
    await game.locator('.mg-tile', { hasText: '猫' }).click();
    await game.locator('.mg-tile', { hasText: 'cat' }).click();
    await expect(page.locator('#match-continue-btn')).toBeVisible();
    await captureForPR(page, 'train-match-game-done');
    await page.locator('#match-continue-btn').click();
    await expect(game).toBeHidden();
    // @ts-ignore
    expect(await page.evaluate(() => window.__mgDone)).toBe(true);
  });

  async function playMatchGame(page, settings) {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'zh_to_transl');
    const st = await (await page.request.get('/api/settings')).json();
    await page.request.patch('/api/settings', { data: { ...st, ...settings } });
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.evaluate(() => {
      // @ts-ignore
      window.__mgDone = false;
      // @ts-ignore
      window.showMatchGame([
        { zh_word_id: 9601, zh_text: '猫', pinyin: 'māo', translations: { en: ['cat'] } },
        { zh_word_id: 9602, zh_text: '狗', pinyin: 'gǒu', translations: { en: ['dog'] } },
      ]).then(() => { window.__mgDone = true; });
    });
    const game = page.locator('#match-game-overlay');
    await game.locator('.mg-tile', { hasText: 'dog' }).click();
    await game.locator('.mg-tile', { hasText: '狗' }).click();
    await game.locator('.mg-tile', { hasText: 'cat' }).click();
    await game.locator('.mg-tile', { hasText: '猫' }).click();
    return game;
  }

  test('the done note says only wrong matches count when progress is "only wrong answers"', async ({ page }) => {
    await playMatchGame(page, { match_game_sm2_update: 'wrong_only' });
    await expect(page.locator('#match-continue-btn')).toBeVisible();
    await expect(page.locator('#match-game-overlay')).toContainText('Only wrong matches count as a training answer');
    await captureForPR(page, 'train-match-game-done-wrong-only');
  });

  test('the done screen is skipped when "show round summary" is off', async ({ page }) => {
    const game = await playMatchGame(page, { match_game_show_summary: false });
    await expect(game).toBeHidden();
    await expect(page.locator('#match-continue-btn')).toHaveCount(0);
    // @ts-ignore
    expect(await page.evaluate(() => window.__mgDone)).toBe(true);
  });
});

test.describe('Train redesign – phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('session chip and the session sheet work on a phone', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello', 'hi']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');

    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#open-filter-overlay')).toBeVisible();
    await captureForPR(page, 'train-question-phone');
    await page.locator('#open-filter-overlay').click();
    await expect(page.locator('#filter-overlay')).toBeVisible();
    await page.locator('#filter-overlay .overlay-mode-btn[data-mode="progressive"]').click();
    await page.locator('#filter-overlay-close').click();
    await expect(page.locator('#session-chip-label')).toContainText('Progressive');
  });

  test('session chip, sound and fullscreen buttons live in the top bar and the chip shows only the mode', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'cycle');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    const topbar = page.locator('#app-topbar');
    await expect(topbar.locator('#open-filter-overlay')).toBeVisible();
    await expect(topbar.locator('#autoplay-toggle-btn')).toBeVisible();
    await expect(topbar.locator('#fullscreen-toggle-btn')).toHaveCount(1);
    await expect(page.locator('#session-bar #open-filter-overlay')).toHaveCount(0);
    await expect(page.locator('#session-chip-label')).toContainText('Cycle');
    await expect(page.locator('#session-chip-label .tr-chip-extra')).toBeHidden();
    await captureForPR(page, 'train-topbar-phone');

    // The chip still opens the session sheet from the top bar.
    await topbar.locator('#open-filter-overlay').click();
    await expect(page.locator('#filter-overlay')).toBeVisible();
    await page.locator('#filter-overlay-close').click();

    // Back on a wide screen the buttons return to the session bar.
    await page.setViewportSize({ width: 1280, height: 900 });
    await expect(page.locator('#session-bar #open-filter-overlay')).toBeVisible();
    await expect(topbar.locator('#open-filter-overlay')).toHaveCount(0);
    await expect(page.locator('#session-chip-label .tr-chip-extra')).toBeVisible();
  });
});

// Issue #510: on a phone the session bar scrolls past the sticky top bar.
// The top bar must stay above it instead of being painted over.
test.describe('Train – top bar stacking', () => {
  test.use({ viewport: { width: 360, height: 641 } });

  test('top bar stays above the session bar while scrolling', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    // Scroll so the session bar overlaps the sticky top bar.
    await page.evaluate(() => {
      document.body.style.minHeight = '3000px';
      const bar = document.getElementById('session-bar');
      window.scrollTo(0, bar.getBoundingClientRect().top + window.scrollY + 10);
    });
    const topbar = await page.locator('#app-topbar').boundingBox();
    const bar = await page.locator('#session-bar').boundingBox();
    expect(bar.y, 'session bar must overlap the top bar for this test').toBeLessThan(topbar.y + topbar.height);
    const topIsTopbar = await page.evaluate(() => {
      const el = document.elementFromPoint(180, 32);
      return !!el && !!el.closest('#app-topbar');
    });
    expect(topIsTopbar).toBe(true);
    await captureForPR(page, 'train-topbar-above-session-bar');
});
  
// Issue #506: the voice card hides the Chinese text, so its label must say
// that the answer is the translation (not Chinese).
test.describe('Train – voice card label', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('voice card label says the answer is the translation', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await page.route('**/api/quiz/next*', route => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ word_id: 1, mode: 'voice_to_transl', prompt: '你好', pinyin: 'nǐ hǎo' }),
    }));
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toBeHidden();
    await expect(page.locator('#mode-label')).toHaveText('Voice → Translation');
    await captureForPR(page, 'train-voice-label');
  });
});
