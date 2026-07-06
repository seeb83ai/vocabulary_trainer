// @ts-check
import { test, expect } from '@playwright/test';

// ─────────────────────────────────────────────────────────────────────────────
// Difficult-words drill (E2E)
//
// From the "All done for today!" success overlay the user can tick a checkbox to
// drill the words they struggle with most. Picking an amount flags that many of
// the user's hardest words (lowest accuracy / lowest ease factor) and serves them
// as a focused quiz — even though their due_date is in the future — until each is
// answered correctly. A temporary pill in the filter bar shows the drill is active.
//
// This spec registers its OWN isolated user (the global-setup DB is shared) and
// sets up two "difficult" words purely via the REST API. Each word is first
// graduated out of the new-word learning phase (3 correct answers), then made
// difficult by answering wrong several times and finally correct once. That
// leaves total_attempts well above the >=3 flag guard, accuracy well below 50 %,
// and due_date pushed to tomorrow — so nothing is due today and, with no unseen
// words, the success overlay appears.
// ─────────────────────────────────────────────────────────────────────────────

const DIFFICULT = [
  { zh: '山', pinyin: 'shān', en: ['mountain'] },
  { zh: '河', pinyin: 'hé', en: ['river'] },
];

test.describe('Difficult-words drill', () => {
  // No storageState → a fresh, isolated browser context / user.

  test('drill from success overlay serves flagged words with a filter-bar pill', async ({ page }) => {
    const email = `e2e-difficult-${Date.now()}@test.local`;
    const password = 'E2eDifficultPassword123!';
    const req = page.request;

    // ── Register (auto-login, no SMTP) — cookies stored on the context ──────────
    const reg = await req.post('/api/register', { data: { email, password } });
    expect(reg.ok()).toBeTruthy();

    // ── Seed words and capture their ids ────────────────────────────────────────
    /** @param {{zh:string,pinyin:string,en:string[]}} w */
    async function seed(w) {
      const res = await req.post('/api/words', {
        data: { zh_text: w.zh, pinyin: w.pinyin, translations: { en: w.en }, tags: [], start_training: true },
      });
      expect(res.ok()).toBeTruthy();
      return (await res.json()).id;
    }
    /** @param {number} id @param {string} answer */
    async function answer(id, ans) {
      const res = await req.post('/api/quiz/answer', {
        data: { word_id: id, mode: 'zh_to_transl', answer: ans, langs: ['en'] },
      });
      expect(res.ok()).toBeTruthy();
      return res.json();
    }

    for (const w of DIFFICULT) {
      const id = await seed(w);
      // Graduate out of the learning phase (3 correct in a row).
      await answer(id, w.en[0]);
      await answer(id, w.en[0]);
      await answer(id, w.en[0]);
      // Now make it difficult: several wrong answers drop accuracy well below 50 %.
      for (let i = 0; i < 5; i++) await answer(id, 'definitely-wrong');
      // One final correct answer pushes due_date to tomorrow (off today's queue).
      await answer(id, w.en[0]);
    }

    // ── Drive the UI: success overlay → difficult-words drill ───────────────────
    await page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
      localStorage.removeItem('quizDifficultDrill');
    });
    await page.goto('/train');

    // Nothing is due today → success overlay is shown.
    await expect(page.locator('#success-state')).toBeVisible({ timeout: 12_000 });

    // Tick "Learn difficult words" and pick an amount.
    await page.locator('#difficult-words-checkbox').check();
    await page.locator('.advance-btn[data-advance="10"]').click();

    // The temporary difficult-drill pill must appear in the filter bar.
    await expect(page.locator('#difficult-drill-pill')).toBeVisible({ timeout: 8_000 });

    // A flagged (difficult) word must now be served despite being due tomorrow.
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    const prompt = await page.locator('#prompt-word').textContent();
    expect(['山', '河']).toContain((prompt || '').trim());

    // Answering it correctly clears its flag and shrinks the drill pool.
    const correct = DIFFICULT.find(d => d.zh === (prompt || '').trim())?.en[0] || '';
    await page.locator('#answer-input').fill(correct);
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!', { timeout: 8_000 });

    // Move on — the second difficult word is served (drill still active).
    await page.locator('#next-btn').click();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    const prompt2 = await page.locator('#prompt-word').textContent();
    expect(['山', '河']).toContain((prompt2 || '').trim());
  });
});
