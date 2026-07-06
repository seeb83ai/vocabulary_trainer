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

  test('result-play-btn is visible in result area after answering (issue #158)', async ({ page }) => {
    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // Submit a wrong answer so the word stays due and doesn't disrupt later tests
    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });

    await expect(page.locator('#result-play-btn')).toBeVisible();
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

  test('play button is visible in transl_to_zh mode (hear the word while recalling)', async ({ page }) => {
    await useTranslToZhMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#play-btn')).toBeVisible();
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
