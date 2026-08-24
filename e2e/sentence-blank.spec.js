// @ts-check
import { test, expect } from '@playwright/test';

// ─────────────────────────────────────────────────────────────────────────────
// Sentence fill-in-the-blank training mode.
//
// A fresh isolated user is registered with three acknowledged component words
// (我/买/牛奶, all total_attempts=1 via start_training=true) and one sentence
// word (我买牛奶, tagged `s_test`, NOT started so it doesn't clutter the normal
// due queue). sentence_blank_enabled + sentence_blank_ratio=100 make the
// server deterministically attempt a sentence-blank card on every /api/quiz/next
// call. Since all three component words are freshly acknowledged
// (total_correct=0), SelectNewWordMode's Step0 default (transl_to_zh) applies
// to every one of them, so the direction is deterministic: the zh word is
// blanked, and the sentence's English translation is shown as context.
// ─────────────────────────────────────────────────────────────────────────────

async function registerUserWithSentence(page) {
  const email = `e2e-sentence-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', {
    data: { email, password: 'SentenceTest123!' },
  });
  expect(regRes.ok()).toBeTruthy();

  const words = [
    { zh: '我', pinyin: 'wǒ', en: ['I'] },
    { zh: '买', pinyin: 'mǎi', en: ['buy'] },
    { zh: '牛奶', pinyin: 'niú nǎi', en: ['milk'] },
  ];
  const idToZh = {};
  for (const w of words) {
    const res = await page.request.post('/api/words', {
      data: { zh_text: w.zh, pinyin: w.pinyin, translations: { en: w.en }, tags: [], start_training: true },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    idToZh[body.id] = w.zh;
  }

  const sentenceRes = await page.request.post('/api/words', {
    data: {
      zh_text: '我买牛奶',
      pinyin: 'wǒ mǎi niú nǎi',
      translations: { en: ['I buy milk'] },
      tags: ['s_test'],
      start_training: false,
    },
  });
  expect(sentenceRes.ok()).toBeTruthy();

  const settingsRes = await page.request.get('/api/settings');
  expect(settingsRes.ok()).toBeTruthy();
  const settings = await settingsRes.json();
  const patchRes = await page.request.patch('/api/settings', {
    data: { ...settings, sentence_blank_enabled: true, sentence_blank_ratio: 100 },
  });
  expect(patchRes.ok()).toBeTruthy();

  return { email, idToZh };
}

test.describe('Sentence-blank training mode', () => {
  test('quiz/next serves a sentence-blank card once its words have all been seen', async ({ page }) => {
    const { idToZh } = await registerUserWithSentence(page);

    const cardRes = await page.request.get('/api/quiz/next?langs=en');
    expect(cardRes.ok()).toBeTruthy();
    const card = await cardRes.json();

    expect(card.card_type).toBe('sentence');
    expect(Object.keys(idToZh).map(Number)).toContain(card.word_id);
    expect(card.mode).toBe('transl_to_zh');
    expect(card.sentence_blank).toContain('___');
    expect(card.sentence_blank).not.toContain(idToZh[card.word_id]);
    expect(card.sentence_context).toContain('I buy milk');
  });

  test('a sentence-blank card renders in /train and can be answered correctly', async ({ page }) => {
    const { idToZh } = await registerUserWithSentence(page);

    const cardRes = await page.request.get('/api/quiz/next?langs=en');
    expect(cardRes.ok()).toBeTruthy();
    const card = await cardRes.json();
    expect(card.card_type).toBe('sentence');
    const correctZh = idToZh[card.word_id];
    expect(correctZh).toBeTruthy();

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // The context (full English translation) and the blanked zh sentence are shown.
    await expect(page.locator('#sentence-context')).toBeVisible();
    await expect(page.locator('#sentence-context')).toContainText('I buy milk');
    await expect(page.locator('#prompt-word')).toContainText('___');

    await page.locator('#answer-input').fill(correctZh);
    await page.locator('#answer-form button[type="submit"]').click();

    await expect(page.locator('#result-icon')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-icon')).toHaveText('✓ Correct!');
  });

  test('a wrong answer to a sentence-blank card shows Wrong and the correct word', async ({ page }) => {
    await registerUserWithSentence(page);

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toContainText('___');

    await page.locator('#answer-input').fill('xxxxxxxxxxx');
    await page.locator('#answer-form button[type="submit"]').click();

    await expect(page.locator('#result-icon')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong');
  });

  test('sentence-blank mode is never attempted when disabled in settings', async ({ page }) => {
    const { idToZh } = await registerUserWithSentence(page);

    const settingsRes = await page.request.get('/api/settings');
    const settings = await settingsRes.json();
    await page.request.patch('/api/settings', { data: { ...settings, sentence_blank_enabled: false } });

    const cardRes = await page.request.get('/api/quiz/next?langs=en');
    expect(cardRes.ok()).toBeTruthy();
    const card = await cardRes.json();

    expect(card.card_type).not.toBe('sentence');
    expect(Object.keys(idToZh).map(Number)).toContain(card.word_id);
  });
});
