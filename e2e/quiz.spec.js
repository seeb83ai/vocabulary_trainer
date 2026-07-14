// @ts-check
import { test, expect } from '@playwright/test';

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
    // Use a fresh isolated user with exactly 3 due words so the test is not
    // affected by state left behind by earlier tests on the shared main user.
    const email = `e2e-norepeat-${Date.now()}@test.local`;
    const regRes = await page.request.post('/api/register', {
      data: { email, password: 'NoRepeat123!' },
    });
    expect(regRes.ok()).toBeTruthy();

    for (const [zh, pinyin, en] of [
      ['你好', 'nǐ hǎo', 'hello'],
      ['谢谢', 'xiè xiè', 'thank you'],
      ['再见', 'zài jiàn', 'goodbye'],
    ]) {
      const r = await page.request.post('/api/words', {
        data: { zh_text: zh, pinyin, translations: { en: [en] }, tags: [], start_training: true },
      });
      expect(r.ok()).toBeTruthy();
    }

    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });

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
  async function setupAmbiguousResult(page) {
    const email = `e2e-ambig-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
    const regRes = await page.request.post('/api/register', {
      data: { email, password: 'AmbigTest123!' },
    });
    expect(regRes.ok()).toBeTruthy();

    const seedRes1 = await page.request.post('/api/words', {
      data: { zh_text: '知道', pinyin: 'zhīdào', translations: { en: ['know'] }, tags: [], start_training: true },
    });
    expect(seedRes1.ok()).toBeTruthy();
    const seed1 = await seedRes1.json();
    const seedRes2 = await page.request.post('/api/words', {
      data: { zh_text: '认识', pinyin: 'rènshi', translations: { en: ['know', 'recognize'] }, tags: [], start_training: true },
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
    await expect(page.locator('#word-breakdown')).toContainText('CORRECT', { ignoreCase: true });

    // A second click on Next now actually advances.
    await page.locator('#next-btn').click();
    await expect(page.locator('#result-area')).not.toBeVisible({ timeout: 8_000 });
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
// Group: Show pinyin in answer boxes (issue #205)
//
// When a user types a Chinese word as a wrong answer in transl_to_zh mode,
// the red "Your Answer" box should show the pinyin alongside the typed word.
// ─────────────────────────────────────────────────────────────────────────────
test.describe('Quiz – pinyin shown in Your Answer box (issue #205)', () => {
  // Seed data pinyin map (matches global-setup.js seed)
  const SEED_PINYIN = {
    '你好': 'nǐ hǎo',
    '谢谢': 'xiè xiè',
    '再见': 'zài jiàn',
  };

  test('wrong Chinese answer in transl_to_zh mode shows pinyin in the Your Answer box', async ({ page }) => {
    // Register a fresh isolated user so card selection is deterministic.
    const email = `e2e-pinyin205-${Date.now()}@test.local`;
    const password = 'PinyinTest123!';
    const regRes = await page.request.post('/api/register', {
      data: { email, password },
    });
    expect(regRes.ok()).toBeTruthy();

    // Seed two words with pinyin and start_training: true
    const seedRes1 = await page.request.post('/api/words', {
      data: { zh_text: '你好', pinyin: 'nǐ hǎo', translations: { en: ['hello'] }, tags: [], start_training: true },
    });
    expect(seedRes1.ok()).toBeTruthy();
    await page.request.post('/api/words', {
      data: { zh_text: '谢谢', pinyin: 'xiè xiè', translations: { en: ['thank you'] }, tags: [], start_training: true },
    });

    // Set transl_to_zh mode
    await page.request.patch('/api/training-filters', {
      data: { mode: 'transl_to_zh', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'transl_to_zh');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });

    // Find out which card will be served next
    const cardRes = await page.request.get('/api/quiz/next?mode=transl_to_zh&langs=en');
    expect(cardRes.ok()).toBeTruthy();
    const card = await cardRes.json();
    expect(card.mode).toBe('transl_to_zh');

    // Pick the OTHER seeded word to type as a wrong answer
    const wrongZh = card.zh_text === '你好' ? '谢谢' : '你好';
    const expectedPinyin = SEED_PINYIN[wrongZh];
    expect(expectedPinyin).toBeTruthy();

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // Submit the wrong Chinese word
    await page.locator('#answer-input').fill(wrongZh);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

    // The red "Your Answer" box must show the pinyin of the wrong word typed
    await expect(page.locator('#word-breakdown .bg-red-50')).toBeVisible();
    await expect(page.locator('#word-breakdown .bg-red-50')).toContainText(expectedPinyin);
  });
});
