// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// ─────────────────────────────────────────────────────────────────────────────
// Group 1: Main user — 3 acknowledged words (total_attempts=1, learning_new_word=1)
//
// Seed (from global-setup.js, start_training: true):
//   你好 → hello, hi
//   谢谢 → thank you, thanks
//   再见 → goodbye, bye
//
// AcknowledgeWord() sets total_attempts=1, first_seen_date=today, due_date=now.
// Expected quiz state: #card-area is shown (NOT #new-word-area) because
// total_attempts > 0.
//
// We force mode=zh_to_transl via localStorage so the prompt is the Chinese
// word and the expected answer is the English translation.
//
// In zh_to_transl mode, the /api/quiz/next response does NOT include
// translations (only the zh prompt). We derive the correct answer from the
// known seed mapping below.
// ─────────────────────────────────────────────────────────────────────────────

/** Translations keyed by zh word — derived from the seed in global-setup.js. */
const SEED_TRANSLATIONS = {
  '你好': ['hello', 'hi'],
  '谢谢': ['thank you', 'thanks'],
  '再见': ['goodbye', 'bye'],
};

test.describe('Quiz – acknowledged words (main user)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  // Set zh_to_transl mode both server-side and in localStorage before the page
  // scripts run. Server-side is required because _settingsPromise now restores
  // server-persisted training filters on load (issue #161), overriding localStorage.
  async function useZhToTranslMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  test('card-area is shown (not new-word-area) for acknowledged words', async ({ page }) => {
    // Pre-fetch the card the server will return for zh_to_transl mode.
    // page.request shares session cookies with the browser context (storageState).
    // This call is read-only and does not modify any SM-2 state.
    const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl&langs=en');
    expect(cardRes.ok()).toBe(true);
    const card = await cardRes.json();

    // The API must return one of the three seeded zh words
    expect(card.mode).toBe('zh_to_transl');
    expect(['你好', '谢谢', '再见']).toContain(card.prompt);

    await useZhToTranslMode(page);
    await page.goto('/train');

    // card-area appears; new-word-area must NOT be visible
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#new-word-area')).not.toBeVisible();

    // The prompt shown in the UI must exactly match what the API returned
    await expect(page.locator('#prompt-word')).toHaveText(card.prompt);
  });

  test('correct answer (exact translation) shows ✓ Correct!', async ({ page }) => {
    // Fetch the card so we know which zh word will be shown as the prompt.
    // In zh_to_transl mode the card response does NOT include translations
    // (the user must supply the translation from memory), so we look up the
    // correct answer from the known seed mapping.
    const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl&langs=en');
    expect(cardRes.ok()).toBe(true);
    const card = await cardRes.json();

    // card.prompt is one of the seeded zh words; derive the correct answer from seed data
    const correctAnswer = SEED_TRANSLATIONS[card.prompt]?.[0]; // e.g. 'hello' for 你好
    expect(correctAnswer).toBeTruthy(); // guard: prompt must be a known seeded word

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // The prompt shown must be the zh word we fetched
    await expect(page.locator('#prompt-word')).toHaveText(card.prompt);

    // Submit the exact correct answer
    await page.locator('#answer-input').fill(correctAnswer);
    await page.locator('#answer-form button[type="submit"]').click();

    // Result must show ✓ Correct!
    await expect(page.locator('#result-icon')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!');
  });

  test('wrong answer shows ✗ Wrong and reveals the correct answer in the breakdown', async ({ page }) => {
    // Fetch the card so we can verify the breakdown shows the correct zh word
    const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl&langs=en');
    expect(cardRes.ok()).toBe(true);
    const card = await cardRes.json();

    // Also derive the first correct EN translation from seed data, for completeness
    const correctAnswer = SEED_TRANSLATIONS[card.prompt]?.[0];
    expect(correctAnswer).toBeTruthy();

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // Confirm the shown prompt matches the pre-fetched card
    await expect(page.locator('#prompt-word')).toHaveText(card.prompt);

    // Submit a clearly wrong answer (not any known translation)
    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();

    // Result must show ✗ Wrong
    await expect(page.locator('#result-icon')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong');

    // The breakdown must contain the correct zh word (card.prompt) so the
    // user can see what they should have answered.
    // The server returns zh_text=card.prompt in the AnswerResponse which is
    // rendered in the green "Correct" box within #word-breakdown.
    await expect(page.locator('#word-breakdown')).toBeVisible();
    await expect(page.locator('#word-breakdown')).toContainText(card.prompt);
    // And the correct EN translation must also appear
    await expect(page.locator('#word-breakdown')).toContainText(correctAnswer);
  });

  test('Next button after answer hides result-area and loads the next state', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // Submit any answer to get to result-area
    await page.locator('#answer-input').fill('hello');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    // Click Next — result-area must be hidden (we moved on to the next card / success state)
    await page.locator('#next-btn').click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });
  });

  test('"add as correct" button auto-advances to next card after adding translation', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // Submit a clearly wrong answer (far from any correct answer) so only the
    // add-translation-btn appears, not the accept-correct-btn (typo threshold).
    await page.locator('#answer-input').fill('notananswer');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong');

    // The "add as correct" button must be visible
    await expect(page.locator('#add-translation-btn')).toBeVisible();

    // Clicking it should auto-advance (load next card), not stay on result-area
    await page.locator('#add-translation-btn').click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });
  });

  test('play button is visible in result area after answering (issue #158)', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // Submit a wrong answer so the word stays due and doesn't disrupt later tests
    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    // Play button is now inline inside the correct-answer box in #word-breakdown
    await expect(page.locator('#word-breakdown .result-inline-play')).toBeVisible();
  });

  test('translation text font size matches across your-answer, confused-with, and correct boxes (issue #199)', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    const prompt = await page.locator('#prompt-word').textContent();
    // Answer with a translation belonging to a DIFFERENT seeded word so the
    // server detects a confusion pair and renders the yellow "belongs to" box.
    const otherZh = Object.keys(SEED_TRANSLATIONS).find(zh => zh !== prompt);
    const confusingAnswer = SEED_TRANSLATIONS[otherZh][0];

    await page.locator('#answer-input').fill(confusingAnswer);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

    const yellowBox = page.locator('#word-breakdown .bg-yellow-50');
    await expect(yellowBox).toBeVisible();
    await captureForPR(page, 'train-mismatch');

    const redSize = await page.locator('#word-breakdown .bg-red-50 .text-red-700')
      .evaluate(el => getComputedStyle(el).fontSize);
    const yellowSize = await yellowBox.locator('.text-sm').first()
      .evaluate(el => getComputedStyle(el).fontSize);
    const greenSize = await page.locator('#word-breakdown .bg-green-50 .text-sm').first()
      .evaluate(el => getComputedStyle(el).fontSize);

    expect(redSize).toBe(yellowSize);
    expect(greenSize).toBe(yellowSize);
  });

  test('issue-report button z-index is above gamification overlay (issue #152)', async ({ page }) => {
    await page.goto('/train');
    const cls = await page.locator('#issue-report-btn').getAttribute('style');
    expect(cls).toContain('z-index:60');
  });

  test('wrong answer is not repeated in the next two cards', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // Record the first card shown, then answer it wrong
    const firstPrompt = await page.locator('#prompt-word').textContent();
    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    // Go to the second card — must NOT be the same word we just answered wrong
    await page.locator('#next-btn').click();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    const secondPrompt = await page.locator('#prompt-word').textContent();
    expect(secondPrompt).not.toBe(firstPrompt);

    // Answer the second card (wrong to keep things simple)
    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    // Go to the third card — must still NOT be the originally wrong-answered word
    await page.locator('#next-btn').click();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    const thirdPrompt = await page.locator('#prompt-word').textContent();
    expect(thirdPrompt).not.toBe(firstPrompt);
  });

  // Regression test for issue #156: when a user has two active learning
  // languages, "Add as correct answer" must let them pick which language the
  // new translation belongs to, defaulting to their primary language.
  test('choosing a language when adding a wrong answer as a correct translation', async ({ page }) => {
    const settingsRes = await page.request.get('/api/settings');
    expect(settingsRes.ok()).toBe(true);
    const originalSettings = await settingsRes.json();

    const patchRes = await page.request.patch('/api/settings', {
      data: { ...originalSettings, secondary_lang: 'de' },
    });
    expect(patchRes.ok()).toBe(true);

    try {
      // Pre-fetch the card so we know which word_id to verify afterwards.
      const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl&langs=en,de');
      expect(cardRes.ok()).toBe(true);
      const card = await cardRes.json();

      // GET /api/quiz/langs (called on page load) only reports languages that
      // have at least one translation — give this word a DE translation so
      // 'de' shows up as an active language and the picker is not pruned away.
      const seedDeRes = await page.request.post(`/api/words/${card.word_id}/translations`, {
        data: { text: `seed-de-${card.word_id}`, lang: 'de' },
      });
      expect(seedDeRes.ok()).toBe(true);

      // Server-side is required because _settingsPromise now restores
      // server-persisted training filters on load (issue #161), overriding localStorage.
      await page.request.patch('/api/training-filters', {
        data: { mode: 'zh_to_transl', langs: ['en', 'de'], bucket: '', mnemonics: true, components: true, tags: [] },
      });
      await page.addInitScript(() => {
        localStorage.setItem('quizMode', 'zh_to_transl');
        localStorage.setItem('quizLangs', JSON.stringify(['en', 'de']));
      });

      await page.goto('/train');
      await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
      await expect(page.locator('#prompt-word')).toHaveText(card.prompt);

      const wrongAnswer = 'definitely-not-a-translation';
      await page.locator('#answer-input').fill(wrongAnswer);
      await page.locator('#answer-form button[type="submit"]').click();
      await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

      // Two languages are active → the picker must appear, defaulting to
      // the user's primary language (en).
      const langSelect = page.locator('#add-translation-lang-select');
      await expect(langSelect).toBeVisible();
      await expect(langSelect).toHaveValue('en');

      // Switch to German and add the wrong answer as a correct DE translation.
      await langSelect.selectOption('de');
      await page.locator('#add-translation-btn').click();
      await expect(page.locator('#add-translation-btn')).toHaveText(/added/i, { timeout: 8_000 });

      // The backend must have stored it under DE, not EN.
      const wordRes = await page.request.get(`/api/words/${card.word_id}`);
      expect(wordRes.ok()).toBe(true);
      const word = await wordRes.json();
      expect(word.translations?.de || []).toContain(wrongAnswer);
      expect(word.translations?.en || []).not.toContain(wrongAnswer);
    } finally {
      await page.request.patch('/api/settings', { data: originalSettings });
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Group 2: New-word user — one unseen word (total_attempts=0)
//
// Seed: 水/shuǐ/['water']  with start_training: false
//       total_attempts=0 → GetNextCard returns mode='new_word', prompt='水'
//
// Expected quiz state: #new-word-area shown (NOT #card-area).
// After "Got it!" → AcknowledgeWord() → total_attempts=1 → #card-area shown.
// ─────────────────────────────────────────────────────────────────────────────
// ─────────────────────────────────────────────────────────────────────────────
// Group: UI – Chinese character size and play button (issue #158)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – Chinese character size and play button (issue #158)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useZhToTranslMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  async function useTranslToZhMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'transl_to_zh', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'transl_to_zh');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  test('prompt-word has large font size (text-5xl or bigger) when Chinese is shown', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    const fontSize = await page.locator('#prompt-word').evaluate(el =>
      parseInt(window.getComputedStyle(el).fontSize)
    );
    // text-5xl = 3rem = 48px at 16px base font size
    expect(fontSize).toBeGreaterThanOrEqual(48);
  });

  test('play button is visible when Chinese is the prompt (zh_to_transl mode)', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#play-btn')).toBeVisible();
  });

  test('play button is hidden in transl_to_zh mode to avoid revealing the answer (issue #168)', async ({ page }) => {
    await useTranslToZhMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#play-btn')).not.toBeVisible();
  });

  test('transl_to_zh card response includes zh_text field for audio', async ({ page }) => {
    const cardRes = await page.request.get('/api/quiz/next?mode=transl_to_zh&langs=en');
    expect(cardRes.ok()).toBe(true);
    const card = await cardRes.json();
    expect(card.mode).toBe('transl_to_zh');
    expect(card.zh_text).toBeTruthy();
    expect(['你好', '谢谢', '再见']).toContain(card.zh_text);
  });

  test('mnemonic toggle is hidden completely on wrong answer (issue #212)', async ({ page }) => {
    // Mock the answer endpoint to inject a scene_text into a wrong response,
    // so we can verify the mnemonic toggle is not shown regardless of scene presence.
    await page.route('**/api/quiz/answer', async (route) => {
      const response = await route.fetch();
      const json = await response.json();
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ...json, correct: false, scene_text: 'Test mnemonic scene' }),
      });
    });

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

    // The mnemonic toggle must NOT appear on a wrong answer
    await expect(page.locator('#hmm-toggle-btn')).not.toBeVisible();
    await expect(page.locator('#result-hmm')).not.toBeVisible();
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Group: Blur pinyin setting (issue #201)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – blur pinyin (issue #201)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useZhPinyinToTranslMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_pinyin_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_pinyin_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  test('pinyin hint is NOT blurred by default', async ({ page }) => {
    await useZhPinyinToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#pinyin-hint')).toBeVisible();
    await expect(page.locator('#pinyin-hint')).not.toHaveClass(/blur/);
  });

  test('pinyin hint is blurred on load and reveals on click when blur_pinyin is enabled', async ({ page }) => {
    const settingsRes = await page.request.get('/api/settings');
    const originalSettings = await settingsRes.json();
    await page.request.patch('/api/settings', { data: { ...originalSettings, blur_pinyin: true } });

    try {
      await useZhPinyinToTranslMode(page);
      await page.goto('/train');
      await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
      await expect(page.locator('#pinyin-hint')).toBeVisible();
      await expect(page.locator('#pinyin-hint')).toHaveClass(/blur/);

      await page.locator('#pinyin-hint').click();
      await expect(page.locator('#pinyin-hint')).not.toHaveClass(/blur/);
    } finally {
      await page.request.patch('/api/settings', { data: originalSettings });
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – new word introduction (new-word user)', () => {
  test.use({ storageState: 'e2e/.auth/new-word-user.json' });

  test('new-word-area shown with correct zh word and translation for unseen word', async ({ page }) => {
    // Confirm the API returns mode=new_word for the unseen word.
    // This is a read-only call; it does NOT change any state.
    const cardRes = await page.request.get('/api/quiz/next?langs=en');
    expect(cardRes.ok()).toBe(true);
    const card = await cardRes.json();
    expect(card.mode).toBe('new_word');
    expect(card.prompt).toBe('水');
    expect(card.translations?.en).toContain('water');

    await page.goto('/train');

    // new-word-area must appear; card-area must NOT appear
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#card-area')).not.toBeVisible();

    // The zh word and its translation must be shown
    await expect(page.locator('#new-word-zh')).toHaveText('水');
    await expect(page.locator('#new-word-area')).toContainText('water');
  });

  test('stats-due counter shows 1 while new word introduction is displayed (issue #206)', async ({ page }) => {
    await page.goto('/train');

    // The unseen word triggers new-word-area
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 12_000 });

    // stats-due must show 1: the word being introduced counts toward today's work
    // even though it hasn't been acknowledged yet (first_seen_at IS NULL) and
    // therefore isn't included in stats.due_today from the backend.
    await expect(page.locator('#stats-due')).toHaveText('1', { timeout: 8_000 });
  });

  test('"Got it!" acknowledges the word and transitions to card-area', async ({ page }) => {
    // Use zh_to_transl mode so that after acknowledgement the card shows a
    // deterministic prompt (the zh word '水') rather than a randomly selected mode.
    // Patch server-side first because _settingsPromise restores server-persisted filters
    // on load, overriding localStorage values.
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });

    await page.goto('/train');

    // The unseen word triggers new-word-area first (mode=new_word overrides
    // the requested mode when total_attempts=0)
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#new-word-zh')).toHaveText('水');

    // Fill the required input fields (require_zh and require_trans default to true)
    await page.locator('#new-word-zh-input').fill('水');
    await page.locator('#new-word-zh-input').dispatchEvent('input');
    await page.locator('#new-word-trans-input').fill('water');
    await page.locator('#new-word-trans-input').dispatchEvent('input');

    // Click "Got it!" to acknowledge the word
    // POST /api/quiz/acknowledge → AcknowledgeWord() → total_attempts=1, due_date=now
    await page.locator('#new-word-got-it-btn').click();

    // After acknowledgement: new-word-area hides, card-area appears in zh_to_transl mode
    // Prompt = '水' (zh word), expected answer = 'water'
    await expect(page.locator('#new-word-area')).not.toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#prompt-word')).toHaveText('水');
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Group 3: Ambiguous answer — two words sharing the same translation
//
// A fresh user is registered with only two words: 知道 and 认识, both with EN
// "know".  When the user types 认识 in transl_to_zh mode for the 知道 card the
// server returns ambiguous=true and the frontend shows the disambiguation input.
//
// Using a fresh user (registered inside the test) guarantees only these two
// words are in the quiz queue, making the card selection deterministic.
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – ambiguous answer (shared translation)', () => {
  // Registers a fresh isolated user with exactly the two ambiguous words 知道/认识
  // (both mean "know"), submits the OTHER zh word for the quizzed card so the
  // server responds ambiguous=true, and leaves the disambiguation panel showing.
  // Returns the correct zh word (quizZh) so callers can resolve it if needed.
  async function setupAmbiguousResult(page, translations = { '知道': ['know'], '认识': ['know', 'recognize'] }) {
    const email = `e2e-ambig-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
    const regRes = await page.request.post('/api/register', {
      data: { email, password: 'AmbigTest123!' },
    });
    expect(regRes.ok()).toBeTruthy();

    const seedRes1 = await page.request.post('/api/words', {
      data: { zh_text: '知道', pinyin: 'zhīdào', translations: { en: translations['知道'] }, tags: [], start_training: true },
    });
    expect(seedRes1.ok()).toBeTruthy();
    const seed1 = await seedRes1.json();
    const seedRes2 = await page.request.post('/api/words', {
      data: { zh_text: '认识', pinyin: 'rènshi', translations: { en: translations['认识'] }, tags: [], start_training: true },
    });
    expect(seedRes2.ok()).toBeTruthy();
    const seed2 = await seedRes2.json();

    const idToZh = { [seed1.id]: '知道', [seed2.id]: '认识' };
    const zhToOpposite = { '知道': '认识', '认识': '知道' };

    const cardRes = await page.request.get('/api/quiz/next?mode=transl_to_zh&langs=en');
    expect(cardRes.ok()).toBeTruthy();
    const card = await cardRes.json();
    const quizZh = idToZh[card.word_id];
    expect(quizZh).toBeTruthy();
    const wrongAnswer = zhToOpposite[quizZh];

    await page.request.patch('/api/training-filters', {
      data: { mode: 'transl_to_zh', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'transl_to_zh');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#answer-input').fill(wrongAnswer);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-icon')).toHaveText('~ Ambiguous');
    await expect(page.locator('#disambig-input')).toBeVisible();

    return { quizZh, wrongAnswer };
  }

  // Issue #245: after the user resolves an ambiguous answer correctly, the inline
  // play button (🔊) in the green correct-answer box must trigger audio playback.
  test('inline play button fires audio request after disambiguation resolves to Correct (issue #245)', async ({ page }) => {
    const { quizZh } = await setupAmbiguousResult(page);

    // Intercept audio requests so we can detect if the button wires up playAudio.
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    // Resolve the disambiguation correctly.
    await page.locator('#disambig-input').fill(quizZh);
    await page.locator('#disambig-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 5_000 });

    // The inline play button must exist in the green box.
    const playBtn = page.locator('#word-breakdown .result-inline-play');
    await expect(playBtn).toBeVisible();

    // Clicking it must fire an audio request — proving the event listener is attached.
    await playBtn.click();
    await page.waitForTimeout(300);
    expect(audioRequests.length).toBeGreaterThan(0);
  });

  test('ambiguous answer shows disambiguation input and accepts correct re-type', async ({ page }) => {
    const { quizZh, wrongAnswer } = await setupAmbiguousResult(page);

    // Type the wrong word again — should show "Not quite" feedback and let the user retry.
    await page.locator('#disambig-input').fill(wrongAnswer);
    await page.locator('#disambig-form button[type="submit"]').click();
    await expect(page.locator('#disambig-feedback')).toContainText('Not quite');

    // Type the correct zh word — result must flip to Correct.
    await page.locator('#disambig-input').fill(quizZh);
    await page.locator('#disambig-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 5_000 });
    // Disambiguation input disappears after a successful answer.
    await expect(page.locator('#disambig-input')).not.toBeVisible();
  });

  // Issue #244: gray question-recap box must be hidden after disambiguation resolves to Correct.
  test('gray question-recap box is hidden after disambiguation resolves to Correct (issue #244)', async ({ page }) => {
    const { quizZh } = await setupAmbiguousResult(page);

    // Type the correct zh word to resolve the disambiguation.
    await page.locator('#disambig-input').fill(quizZh);
    await page.locator('#disambig-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 5_000 });

    // The gray question-recap box must NOT remain visible on the success screen.
    await expect(page.locator('#result-question')).not.toBeVisible();
  });

  // Issue #194: a wrong word typed into the disambiguation form only shows
  // "Not quite" and lets the user retry. The normal Wrong screen is shown only
  // once the user gives up and clicks Next without having resolved it.
  test('wrong word in disambiguation form allows retry; Next shows Wrong screen (issue #194)', async ({ page }) => {
    const { wrongAnswer } = await setupAmbiguousResult(page);

    await page.locator('#disambig-input').fill(wrongAnswer);
    await page.locator('#disambig-form button[type="submit"]').click();

    // Still on the disambiguation form — no Wrong screen yet.
    await expect(page.locator('#disambig-feedback')).toContainText('Not quite');
    await expect(page.locator('#disambig-input')).toBeVisible();
    await expect(page.locator('#result-icon')).toHaveText('~ Ambiguous');

    // Clicking Next without resolving reveals the normal Wrong screen first.
    await page.locator('#next-btn').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 5_000 });
    await expect(page.locator('#disambig-input')).not.toBeVisible();

    // A second Next click then actually advances.
    await page.locator('#next-btn').click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });
  });

  // Issue #194: continuing past an unresolved ambiguous result must fall back
  // to the normal wrong-answer screen instead of silently advancing.
  test('clicking Next on an unresolved ambiguous result shows the normal Wrong screen first', async ({ page }) => {
    await setupAmbiguousResult(page);

    // Click Next WITHOUT resolving the disambiguation.
    await page.locator('#next-btn').click();

    // Still on the result screen — now showing the normal wrong-answer state.
    await expect(page.locator('#result-area')).toBeVisible();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong');
    await expect(page.locator('#disambig-input')).not.toBeVisible();
    await expect(page.locator('#word-breakdown .bg-green-50')).toBeVisible();
    // The gray question-recap box is ambiguous-only; the fallback Wrong
    // screen must not show it (issue #231 follow-up).
    await expect(page.locator('#result-question')).not.toBeVisible();

    // A second click on Next now actually advances.
    await page.locator('#next-btn').click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });
  });

  // Issue #231: the "TO CHINESE / <prompt>" gray recap box must sit between
  // the yellow "belongs to" box and the orange disambiguation box, not above
  // both of them.
  test('question recap box sits between the confused-with and disambiguation boxes (issue #231)', async ({ page }) => {
    await setupAmbiguousResult(page);

    const disambigArea = page.locator('#disambig-area');
    const boxes = await disambigArea.locator(':scope > div').all();
    const classes = await Promise.all(boxes.map((box) => box.getAttribute('class')));

    const yellowIndex = classes.findIndex((c) => c && c.includes('bg-yellow-50'));
    const grayIndex = classes.findIndex((c) => c && c.includes('bg-gray-50'));
    const orangeIndex = classes.findIndex((c) => c && c.includes('bg-orange-50'));

    expect(yellowIndex).toBeGreaterThanOrEqual(0);
    expect(grayIndex).toBeGreaterThanOrEqual(0);
    expect(orangeIndex).toBeGreaterThanOrEqual(0);
    expect(grayIndex).toBeGreaterThan(yellowIndex);
    expect(grayIndex).toBeLessThan(orangeIndex);
  });

  // Issue #231 (follow-up): the recap box must list every other translation
  // of the quizzed word, same as the original question does before answering.
  test('question recap box shows all translations like the original question (issue #231)', async ({ page }) => {
    const translations = { '知道': ['know', 'understand'], '认识': ['know', 'recognize'] };
    const { quizZh } = await setupAmbiguousResult(page, translations);

    const promptText = (await page.locator('#result-question-word').textContent()).trim();
    const others = translations[quizZh].filter((w) => w !== promptText);
    expect(others.length).toBeGreaterThan(0);

    const transEl = page.locator('#result-question-translations');
    await expect(transEl).toBeVisible();
    const transText = await transEl.textContent();
    for (const other of others) {
      expect(transText).toContain(other);
    }
  });

  // Issue #193: on a small viewport the disambiguation input must stay inside
  // its containing card, not overflow past the right edge.
  test('disambiguation input fits inside its card on a small viewport', async ({ page }) => {
    await page.setViewportSize({ width: 389, height: 694 });
    await setupAmbiguousResult(page);

    const cardBox = await page.locator('#result-area > div').boundingBox();
    const inputBox = await page.locator('#disambig-input').boundingBox();
    const btnBox = await page.locator('#disambig-form button[type="submit"]').boundingBox();
    expect(cardBox).toBeTruthy();
    expect(inputBox).toBeTruthy();
    expect(btnBox).toBeTruthy();
    // Neither the text input nor the "Check" submit button (the two children of
    // the disambiguation form) may extend past the right edge of the white
    // result card — that's the overflow reported in issue #193.
    expect(inputBox.x + inputBox.width).toBeLessThanOrEqual(cardBox.x + cardBox.width + 0.5);
    expect(btnBox.x + btnBox.width).toBeLessThanOrEqual(cardBox.x + cardBox.width + 0.5);
  });

  // Below the `sm` breakpoint (640px), the app already switches to a stacked
  // mobile layout elsewhere (nav menu collapses). The disambiguation input and
  // "Check" button must follow the same pattern: input full width on its own
  // line, button full width on the line below — not side by side.
  test('disambiguation input and Check button stack full-width below the sm breakpoint', async ({ page }) => {
    await page.setViewportSize({ width: 639, height: 800 });
    await setupAmbiguousResult(page);

    const formBox = await page.locator('#disambig-form').boundingBox();
    const inputBox = await page.locator('#disambig-input').boundingBox();
    const btnBox = await page.locator('#disambig-form button[type="submit"]').boundingBox();
    expect(formBox).toBeTruthy();
    expect(inputBox).toBeTruthy();
    expect(btnBox).toBeTruthy();

    // Button sits on the line below the input (stacked, not side by side).
    expect(btnBox.y).toBeGreaterThanOrEqual(inputBox.y + inputBox.height - 0.5);
    // Both the input and the button span the full width of the form.
    expect(inputBox.width).toBeGreaterThanOrEqual(formBox.width - 0.5);
    expect(btnBox.width).toBeGreaterThanOrEqual(formBox.width - 0.5);
  });

  // The character breakdown toggle would reveal the correct word's characters,
  // undermining the point of asking the user to disambiguate. Mock the decompose
  // API so it always has data to show, isolating this from whether the test DB
  // happens to have hanzi decomposition data seeded for these particular words.
  async function mockDecomposeResponse(page) {
    await page.route('**/api/hanzi/decompose*', route =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([{ character: '知', radical: '矢', definition: 'know', components: [] }]),
      })
    );
  }

  test('character breakdown toggle is hidden while an ambiguous result is unresolved', async ({ page }) => {
    await mockDecomposeResponse(page);
    await setupAmbiguousResult(page);
    await expect(page.locator('#result-decompose')).not.toBeVisible();
  });

  test('character breakdown toggle appears once the ambiguous result resolves to Wrong', async ({ page }) => {
    await mockDecomposeResponse(page);
    await setupAmbiguousResult(page);
    await page.locator('#next-btn').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong');
    await expect(page.locator('#result-decompose')).toBeVisible();
  });

});

// ─────────────────────────────────────────────────────────────────────────────
// Group: Show pinyin beside zh words in answer boxes (issue #205)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – pinyin in answer boxes (issue #205)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  const SEED_PINYINS = {
    '你好': 'nǐ hǎo',
    '谢谢': 'xiè xiè',
    '再见': 'zài jiàn',
  };

  test('transl_to_zh wrong answer box shows pinyin beside the typed zh word', async ({ page }) => {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'transl_to_zh', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'transl_to_zh');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });

    // Pre-fetch the card to know which zh word is the correct answer.
    const cardRes = await page.request.get('/api/quiz/next?mode=transl_to_zh&langs=en');
    expect(cardRes.ok()).toBe(true);
    const card = await cardRes.json();
    expect(card.mode).toBe('transl_to_zh');
    const correctZh = card.zh_text;

    // Pick a different seeded word as wrong answer (it's in the user's vocab so pinyin is known).
    const wrongZh = Object.keys(SEED_PINYINS).find(zh => zh !== correctZh);
    const expectedPinyin = SEED_PINYINS[wrongZh];

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#answer-input').fill(wrongZh);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

    // The red "your answer" box must show the typed zh word AND its pinyin.
    const redBox = page.locator('#word-breakdown .bg-red-50');
    await expect(redBox).toBeVisible();
    await expect(redBox).toContainText(wrongZh);
    await expect(redBox).toContainText(expectedPinyin);
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Group: Green result box title shows what was learned (issue #246)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – green result box title (issue #246)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  const SEED_FIRST_ANSWERS = { '你好': 'hello', '谢谢': 'thank you', '再见': 'goodbye' };

  async function useZhToTranslMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  test('correct vocabulary answer shows "Word" label in green box, not "Correct"', async ({ page }) => {
    const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl&langs=en');
    expect(cardRes.ok()).toBe(true);
    const card = await cardRes.json();
    const correctAnswer = SEED_FIRST_ANSWERS[card.prompt];
    expect(correctAnswer).toBeTruthy();

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText(card.prompt);

    await page.locator('#answer-input').fill(correctAnswer);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 8_000 });

    const greenLabel = page.locator('#word-breakdown .bg-green-50 .text-green-500').first();
    await expect(greenLabel).toBeVisible();
    await expect(greenLabel).toContainText('Word', { ignoreCase: true });
    await expect(greenLabel).not.toContainText('Correct', { ignoreCase: true });
  });

  test('wrong vocabulary answer shows "Word" label in green correct-answer box, not "Correct"', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

    const greenLabel = page.locator('#word-breakdown .bg-green-50 .text-green-500').first();
    await expect(greenLabel).toBeVisible();
    await expect(greenLabel).toContainText('Word', { ignoreCase: true });
    await expect(greenLabel).not.toContainText('Correct', { ignoreCase: true });
  });

  test('component answer shows "Component" label in green box, not "Character"', async ({ page }) => {
    await page.route('**/api/quiz/next*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          card_type: 'component',
          prompt: '木',
          pinyin: 'mù',
          is_new: false,
          is_also_word: false,
          definitions: { en: 'wood, tree' },
        }),
      });
    });
    await page.route('**/api/component/answer', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          correct: true,
          correct_answers: { en: 'wood, tree' },
          interval_days: 1,
          total_correct: 1,
          total_attempts: 1,
        }),
      });
    });

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText('木');

    await page.locator('#answer-input').fill('wood');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    const greenLabel = page.locator('#word-breakdown .bg-green-50 .text-green-500').first();
    await expect(greenLabel).toBeVisible();
    await expect(greenLabel).toContainText('Component', { ignoreCase: true });
    await expect(greenLabel).not.toContainText('Character', { ignoreCase: true });
  });

  test('component-also-word answer shows "Component & Word" label in green box', async ({ page }) => {
    await page.route('**/api/quiz/next*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          card_type: 'component',
          prompt: '木',
          pinyin: 'mù',
          is_new: false,
          is_also_word: true,
          definitions: { en: 'wood, tree' },
        }),
      });
    });
    await page.route('**/api/component/answer', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          correct: true,
          correct_answers: { en: 'wood, tree' },
          interval_days: 1,
          total_correct: 1,
          total_attempts: 1,
        }),
      });
    });

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#answer-input').fill('wood');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    const greenLabel = page.locator('#word-breakdown .bg-green-50 .text-green-500').first();
    await expect(greenLabel).toBeVisible();
    await expect(greenLabel).toContainText('Component', { ignoreCase: true });
    await expect(greenLabel).not.toContainText('Character', { ignoreCase: true });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Group: Auto-play sound toggle (issue: auto-play-sound)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – auto-play sound toggle', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useZhToTranslMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  async function useTranslToZhMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'transl_to_zh', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'transl_to_zh');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  test('auto-play toggle is visible and off by default', async ({ page }) => {
    await page.goto('/train');
    await expect(page.locator('#autoplay-toggle-btn')).toBeVisible();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'false');
  });

  test('clicking the toggle arms auto-play and updates aria-pressed', async ({ page }) => {
    await page.goto('/train');
    const btn = page.locator('#autoplay-toggle-btn');
    await btn.click();
    await expect(btn).toHaveAttribute('aria-pressed', 'true');
  });

  test('enabling auto-play triggers an audio request when the next zh_to_transl card loads', async ({ page }) => {
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    // No audio yet — enabling the toggle must not play the already-shown card.
    expect(audioRequests.length).toBe(0);

    // Answer to advance to the next card, which should auto-play.
    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
    await page.locator('#next-btn').click();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });

    await expect.poll(() => audioRequests.length, { timeout: 5_000 }).toBeGreaterThan(0);
  });

  test('auto-play never fires for the transl_to_zh prompt, even when enabled (would reveal the answer)', async ({ page }) => {
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    await useTranslToZhMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    // Give any (incorrect) auto-play a moment to fire before asserting silence.
    await page.waitForTimeout(500);
    expect(audioRequests.length).toBe(0);
  });

  // issue #259: transl_to_zh never auto-plays the prompt (would reveal the
  // answer), but once the card is solved the result screen shows the Chinese
  // answer anyway — so with auto-play on, it should be read out there.
  test('auto-play reads out the Chinese answer on the transl_to_zh result screen after answering (issue #259)', async ({ page }) => {
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    await useTranslToZhMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    await expect.poll(() => audioRequests.length, { timeout: 5_000 }).toBeGreaterThan(0);
  });

  test('auto-play does not reveal the answer while a transl_to_zh result is still ambiguous', async ({ page }) => {
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    await useTranslToZhMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    if (await page.locator('#disambig-form').isVisible().catch(() => false)) {
      await page.waitForTimeout(500);
      expect(audioRequests.length).toBe(0);
    }
  });

  // issue #272: when question-screen autoplay is suppressed (blur guard fires
  // for a component card with no pinyin + no_auto_voice_on_blur setting), the
  // result screen must pick up the audio instead.
  test('auto-play fires on the component result screen when question-screen audio was suppressed (issue #272)', async ({ page }) => {
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    // Component card with no pinyin → blur guard suppresses question-screen autoplay
    // when no_auto_voice_on_blur is enabled.
    await page.route('**/api/quiz/next*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          card_type: 'component',
          prompt: '木',
          pinyin: null,
          is_new: false,
          is_also_word: false,
          definitions: { en: 'wood, tree' },
        }),
      });
    });
    await page.route('**/api/component/answer', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          correct: false,
          correct_answers: { en: 'wood, tree' },
          interval_days: 1,
          total_correct: 0,
          total_attempts: 1,
        }),
      });
    });

    // Enable blur suppression so autoPlayCard returns early without setting questionAutoPlayed.
    await page.request.patch('/api/settings', { data: { no_auto_voice_on_blur: true } });

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    // Question screen: no audio should fire (blur guard suppressed it).
    await page.waitForTimeout(400);
    expect(audioRequests.length).toBe(0);

    // Submit a wrong answer to get to the result screen.
    await page.locator('#answer-input').fill('xxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    // Result screen should now fire the audio that was skipped on the question screen.
    await expect.poll(() => audioRequests.length, { timeout: 5_000 }).toBeGreaterThan(0);
    expect(audioRequests[0]).toContain('/api/audio/component/');

    // Restore default setting.
    await page.request.patch('/api/settings', { data: { no_auto_voice_on_blur: false } });
  });

  test('auto-play toggle resets to off after a page reload', async ({ page }) => {
    await page.goto('/train');
    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    await page.reload();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'false');
  });
});

test.describe('Quiz – fullscreen toggle', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('fullscreen toggle button is visible and off by default', async ({ page }) => {
    await page.goto('/train');
    await expect(page.locator('#fullscreen-toggle-btn')).toBeVisible();
    await expect(page.locator('#fullscreen-toggle-btn')).toHaveAttribute('aria-pressed', 'false');
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Group: Chinese (no sound) → Translation mode
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – Chinese (no sound) mode', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useNoSoundMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl_no_sound', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl_no_sound');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  test('shows the Chinese prompt with the play button hidden', async ({ page }) => {
    const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl_no_sound&langs=en');
    expect(cardRes.ok()).toBe(true);
    const card = await cardRes.json();
    expect(card.mode).toBe('zh_to_transl_no_sound');
    expect(['你好', '谢谢', '再见']).toContain(card.prompt);

    await useNoSoundMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText(card.prompt);
    await expect(page.locator('#play-btn')).not.toBeVisible();
  });

  test('auto-play never fires on the question screen in this mode, even when the toggle is enabled', async ({ page }) => {
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    await useNoSoundMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    // Question screen must stay silent — no audio before the answer is submitted.
    await page.waitForTimeout(400);
    expect(audioRequests.length).toBe(0);
  });

  // issue #272: the "no sound" applies only to the question screen (hiding the
  // prompt audio). Once the answer is revealed on the result screen, autoplay
  // should fire just like any other mode.
  test('auto-play fires on the result screen for zh_to_transl_no_sound (issue #272)', async ({ page }) => {
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    await useNoSoundMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#autoplay-toggle-btn').click();
    await expect(page.locator('#autoplay-toggle-btn')).toHaveAttribute('aria-pressed', 'true');

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    await expect.poll(() => audioRequests.length, { timeout: 5_000 }).toBeGreaterThan(0);
  });

  test('result screen play button still works after answering (no-sound only applies to the prompt phase)', async ({ page }) => {
    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    await useNoSoundMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    const playBtn = page.locator('#word-breakdown .result-inline-play');
    await expect(playBtn).toBeVisible();
    await playBtn.click();
    await page.waitForTimeout(300);
    expect(audioRequests.length).toBeGreaterThan(0);
  });

  test('flat mode button on the training page selects the mode and persists it', async ({ page }) => {
    await page.goto('/train');
    await page.locator('.mode-btn[data-mode="zh_to_transl_no_sound"]').click();

    await expect.poll(() =>
      page.evaluate(() => localStorage.getItem('quizMode'))
    ).toBe('zh_to_transl_no_sound');

    await page.reload();
    await expect(page.locator('.mode-btn[data-mode="zh_to_transl_no_sound"]')).toHaveClass(/bg-blue-600/);
  });

  test('answering correctly grades the same as zh_to_transl', async ({ page }) => {
    const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl_no_sound&langs=en');
    const card = await cardRes.json();
    const correctAnswer = SEED_TRANSLATIONS[card.prompt]?.[0];
    expect(correctAnswer).toBeTruthy();

    await useNoSoundMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText(card.prompt);

    await page.locator('#answer-input').fill(correctAnswer);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 8_000 });
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Group: Bucket-change growth indicator + celebration interstitial
//
// Mocks the /api/quiz/answer response to inject a tier change (Learning →
// Practicing) so the test doesn't depend on the real graduation/accuracy
// timing needed to organically cross a tier boundary. This isolates the
// frontend behaviour under test: the growth-icon indicator on the result
// screen, and the celebration interstitial gated by celebrate_bucket_change.
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – celebrate bucket change setting', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useZhToTranslMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  async function mockTierChangeAnswer(page, correct = true) {
    await page.route('**/api/quiz/answer', async (route) => {
      const response = await route.fetch();
      const json = await response.json();
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ...json, correct, tier: 'Practicing', prev_tier: 'Learning' }),
      });
    });
  }

  async function submitAnswerAndWait(page, answer, card = null) {
    if (!card) {
      const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl&langs=en');
      card = await cardRes.json();
    }

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText(card.prompt);

    await page.locator('#answer-input').fill(answer);
    await page.locator('#answer-form button[type="submit"]').click();
  }

  async function submitCorrectAnswer(page) {
    const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl&langs=en');
    const card = await cardRes.json();
    const correctAnswer = SEED_TRANSLATIONS[card.prompt]?.[0];
    expect(correctAnswer).toBeTruthy();
    await submitAnswerAndWait(page, correctAnswer, card);
  }

  test('celebration screen appears before the result screen, then reveals it after Continue', async ({ page }) => {
    const settingsRes = await page.request.get('/api/settings');
    const originalSettings = await settingsRes.json();
    await page.request.patch('/api/settings', { data: { ...originalSettings, celebrate_bucket_change: true } });

    try {
      await mockTierChangeAnswer(page);
      await submitCorrectAnswer(page);

      // Celebration appears immediately, BEFORE the correct/wrong result screen.
      await expect(page.locator('#celebration-screen')).toBeVisible({ timeout: 8_000 });
      await expect(page.locator('#result-area')).not.toBeVisible();
      await expect(page.locator('#celebration-transition')).toContainText('Learning');
      await expect(page.locator('#celebration-transition')).toContainText('Practicing');

      // Continuing reveals the result screen, with a single tier icon (not the full ladder).
      await page.locator('#celebration-continue-btn').click();
      await expect(page.locator('#celebration-screen')).not.toBeVisible();
      await expect(page.locator('#result-area')).toBeVisible();
      await expect(page.locator('#result-icon')).toHaveText('✓ Correct!');
      await expect(page.locator('#bucket-info .tier-icon')).toHaveCount(1);
      await expect(page.locator('#bucket-info .tier-icon')).toHaveAttribute('title', 'Practicing');

      // Next now behaves normally — straight to the next card, no second celebration.
      await page.locator('#next-btn').click();
      await expect(page.locator('#result-area')).not.toBeVisible();
      await expect(page.locator('#celebration-screen')).not.toBeVisible();
    } finally {
      await page.request.patch('/api/settings', { data: originalSettings });
    }
  });

  test('celebration screen never appears when the setting is disabled (default)', async ({ page }) => {
    await mockTierChangeAnswer(page);
    await submitCorrectAnswer(page);

    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 8_000 });
    await expect(page.locator('#celebration-screen')).not.toBeVisible();
  });

  test('the tier icon also shows on a wrong answer, and celebration never fires for a tier drop', async ({ page }) => {
    const settingsRes = await page.request.get('/api/settings');
    const originalSettings = await settingsRes.json();
    await page.request.patch('/api/settings', { data: { ...originalSettings, celebrate_bucket_change: true } });

    try {
      await mockTierChangeAnswer(page, false);
      await submitAnswerAndWait(page, 'xxxxxxxxxxx');

      // Result appears directly — no celebration screen in between, even
      // though the mocked response carries a (downward) tier change.
      await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });
      await expect(page.locator('#celebration-screen')).not.toBeVisible();

      // The icon still appears on a wrong-answer result screen.
      await expect(page.locator('#bucket-info')).toBeVisible();
      await expect(page.locator('#bucket-info .tier-icon')).toHaveCount(1);
    } finally {
      await page.request.patch('/api/settings', { data: originalSettings });
    }
  });
});

// ─────────────────────────────────────────────────────────────────────────────
// Group: Retype required after a wrong answer (retype_on_wrong setting)
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – retype on wrong answer', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useZhToTranslMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  test('retype gate is not shown when the setting is disabled (default)', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

    await expect(page.locator('#wrong-retype-area')).not.toBeVisible();
    await expect(page.locator('#next-btn')).toBeEnabled();
  });

  test('wrong answer requires retyping the Chinese word and translation before Next is enabled', async ({ page }) => {
    const settingsRes = await page.request.get('/api/settings');
    const originalSettings = await settingsRes.json();
    await page.request.patch('/api/settings', { data: { ...originalSettings, retype_on_wrong: true } });

    try {
      await useZhToTranslMode(page);

      const cardRes = await page.request.get('/api/quiz/next?mode=zh_to_transl&langs=en');
      expect(cardRes.ok()).toBe(true);
      const card = await cardRes.json();
      const correctAnswer = SEED_TRANSLATIONS[card.prompt]?.[0];
      expect(correctAnswer).toBeTruthy();

      await page.goto('/train');
      await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
      await expect(page.locator('#prompt-word')).toHaveText(card.prompt);

      await page.locator('#answer-input').fill('xxxxxxxxxxx');
      await page.locator('#answer-form button[type="submit"]').click();
      await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

      // The retype gate must appear and block Next until resolved.
      await expect(page.locator('#wrong-retype-area')).toBeVisible();
      await expect(page.locator('#next-btn')).toBeDisabled();

      // Typing a wrong word/translation must not unlock it.
      await page.locator('#wrong-retype-zh-input').fill('xxx');
      await page.locator('#wrong-retype-trans-input').fill('xxx');
      await expect(page.locator('#next-btn')).toBeDisabled();

      // Typing the correct Chinese word and translation unlocks Next.
      await page.locator('#wrong-retype-zh-input').fill(card.prompt);
      await page.locator('#wrong-retype-trans-input').fill(correctAnswer);
      await expect(page.locator('#next-btn')).toBeEnabled({ timeout: 8_000 });

      await page.locator('#next-btn').click();
      await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });
    } finally {
      await page.request.patch('/api/settings', { data: originalSettings });
    }
  });

  // Regression test for issue #348: words whose canonical zh text carries a
  // parenthetical part-of-speech annotation (e.g. "花（动词）") show both
  // retype fields as correct (✓) once the user retypes the bare word/
  // translation, but the Next button stayed disabled — the checkmarks and
  // the button-enable check used two different normalisation functions, and
  // only the checkmark logic stripped the parenthesised annotation.
  // A fresh isolated user guarantees this is the only due word in the queue.
  test('Next button enables after correctly retyping a word with a parenthetical annotation (issue #348)', async ({ page }) => {
    const email = `e2e-retype-annot-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
    const regRes = await page.request.post('/api/register', {
      data: { email, password: 'RetypeAnnotTest123!' },
    });
    expect(regRes.ok()).toBeTruthy();

    const seedRes = await page.request.post('/api/words', {
      data: {
        zh_text: '花（动词）',
        pinyin: 'huā',
        translations: { en: ['spend'], de: ['ausgeben'] },
        tags: [],
        start_training: true,
      },
    });
    expect(seedRes.ok()).toBeTruthy();

    const settingsRes = await page.request.get('/api/settings');
    const originalSettings = await settingsRes.json();
    await page.request.patch('/api/settings', { data: { ...originalSettings, retype_on_wrong: true } });

    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en', 'de'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en', 'de']));
    });

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText('花（动词）');

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

    await expect(page.locator('#wrong-retype-area')).toBeVisible();
    await expect(page.locator('#next-btn')).toBeDisabled();

    // Retype the bare word (no parenthetical annotation) and a correct
    // translation — both checkmarks must turn green ...
    await page.locator('#wrong-retype-zh-input').fill('花');
    await page.locator('#wrong-retype-trans-input').fill('ausgeben');
    await expect(page.locator('#wrong-retype-zh-check')).toHaveText('✓');
    await expect(page.locator('#wrong-retype-trans-check')).toHaveText('✓');

    // ... and Next must become enabled (this is the part that regressed).
    await expect(page.locator('#next-btn')).toBeEnabled({ timeout: 8_000 });
    await captureForPR(page, 'train-retype-annotation-next-enabled');

    await page.locator('#next-btn').click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });
  });
});
