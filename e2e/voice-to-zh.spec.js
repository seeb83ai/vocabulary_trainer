// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';

// Issue #571: "Voice → Chinese" plays the word and asks for the hanzi. A
// wrong word with the same pinyin (homophone) shows the meaning and lets the
// user type again. Skipping a voice card with "voice not available" asks the
// same word again in the fallback mode.
//
// Each test registers a fresh user, because answers and skips change the
// card order for the shared users.
async function setupUser(page, mode, words) {
  const email = `e2e-voicezh-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'VoiceZhTest123!' } });
  expect(regRes.ok()).toBeTruthy();
  const ids = {};
  for (const w of words) {
    const res = await page.request.post('/api/words', {
      data: { zh_text: w.zh, pinyin: w.pinyin, translations: { en: w.en }, tags: [], start_training: true },
    });
    expect(res.ok()).toBeTruthy();
    ids[(await res.json()).id] = w;
  }
  await page.request.patch('/api/training-filters', {
    data: { mode, langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
  });
  await syncNewWordMode(page, mode);
  await page.addInitScript((m) => {
    localStorage.setItem('quizMode', m);
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  }, mode);
  return ids;
}

async function nextCard(page, mode, ids) {
  const res = await page.request.get(`/api/quiz/next?mode=${mode}&langs=en`);
  expect(res.ok()).toBeTruthy();
  const card = await res.json();
  return ids[card.word_id];
}

async function submit(page, answer) {
  await page.locator('#answer-input').fill(answer);
  await page.locator('#answer-form button[type="submit"]').click();
  await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
}

test.describe('Voice → Chinese (issue #571)', () => {
  test('the card hides the word and the meaning, plays the sound, and accepts the hanzi', async ({ page }) => {
    await setupUser(page, 'voice_to_zh', [{ zh: '水', pinyin: 'shuǐ', en: ['water'] }]);
    await page.route('**/api/audio/**', route => route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) }));

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#mode-label')).toHaveText('Voice → Chinese');
    await expect(page.locator('#prompt-word')).not.toBeVisible();
    await expect(page.locator('#translations-hint')).not.toBeVisible();
    await expect(page.locator('#play-btn')).toBeVisible();
    await captureForPR(page, 'train-voice-to-zh-card');

    await submit(page, '水');
    await expect(page.locator('#result-icon')).toHaveText('Correct');
  });

  test('a homophone shows the meaning and lets the user type again', async ({ page }) => {
    const ids = await setupUser(page, 'voice_to_zh', [
      { zh: '他', pinyin: 'tā', en: ['he'] },
      { zh: '她', pinyin: 'tā', en: ['she'] },
    ]);
    const word = await nextCard(page, 'voice_to_zh', ids);
    const other = word.zh === '他' ? '她' : '他';

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await submit(page, other);

    await expect(page.locator('#result-icon')).toHaveText('Same sound, other word');
    await expect(page.locator('#result-question')).toContainText(word.en[0]);
    await expect(page.locator('#disambig-input')).toBeVisible();
    await captureForPR(page, 'train-voice-to-zh-homophone');

    await page.locator('#disambig-input').fill(word.zh);
    await page.locator('#disambig-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('Correct', { timeout: 5_000 });
  });

  // Issue #624: on a Voice → Translation card, the meaning of another word
  // with the same sound shows the homophone screen with only a replay button.
  test('Voice → Translation: a homophone\'s meaning shows a replay button and lets the user type again', async ({ page }) => {
    const ids = await setupUser(page, 'voice_to_transl', [
      { zh: '角', pinyin: 'jiǎo', en: ['corner'] },
      { zh: '脚', pinyin: 'jiǎo', en: ['foot'] },
    ]);
    const word = await nextCard(page, 'voice_to_transl', ids);
    const other = word.zh === '角' ? 'foot' : 'corner';
    await page.route('**/api/audio/**', route => route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) }));

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await submit(page, other);

    await expect(page.locator('#result-icon')).toHaveText('Same sound, other word');
    await expect(page.locator('#result-subtitle')).not.toContainText(word.en[0]);
    await expect(page.locator('#result-question')).not.toContainText(word.en[0]);
    await expect(page.locator('#result-question .result-question-play')).toBeVisible();
    await expect(page.locator('#disambig-input')).toHaveAttribute('placeholder', 'Type English');
    await captureForPR(page, 'train-voice-to-transl-homophone');

    const audio = page.waitForRequest(req => req.url().includes('/api/audio/'));
    await page.locator('#result-question .result-question-play').click();
    await audio;

    await page.locator('#disambig-input').fill(other);
    await page.locator('#disambig-form button[type="submit"]').click();
    await expect(page.locator('#disambig-feedback')).toBeVisible();

    await page.locator('#disambig-input').fill(word.en[0]);
    await page.locator('#disambig-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('Correct', { timeout: 5_000 });
  });

  test('a wrong word with other pinyin is a normal wrong answer', async ({ page }) => {
    await setupUser(page, 'voice_to_zh', [{ zh: '水', pinyin: 'shuǐ', en: ['water'] }]);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await submit(page, '火');
    await expect(page.locator('#result-icon')).toHaveText('Not quite');
    await expect(page.locator('#disambig-input')).toHaveCount(0);
  });

  test('skip with "voice not available" asks the same word as Translation → Chinese', async ({ page }) => {
    await setupUser(page, 'voice_to_zh', [{ zh: '水', pinyin: 'shuǐ', en: ['water'] }]);
    let skipCalls = 0;
    page.on('request', req => { if (req.url().includes('/api/quiz/skip')) skipCalls += 1; });
    page.on('dialog', dialog => dialog.accept());

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.locator('#skip-today-btn').click();

    await expect(page.locator('#mode-label')).toHaveText('To Chinese');
    await expect(page.locator('#prompt-word')).toHaveText('water');
    await expect(page.locator('#play-btn')).not.toBeVisible();
    await captureForPR(page, 'train-voice-to-zh-skip-fallback');
    await submit(page, '水');
    await expect(page.locator('#result-icon')).toHaveText('Correct');
    expect(skipCalls).toBe(0);
  });

  test('skip with "voice not available" asks a Voice → Translation word as Chinese → Translation', async ({ page }) => {
    await setupUser(page, 'voice_to_transl', [{ zh: '水', pinyin: 'shuǐ', en: ['water'] }]);
    let skipCalls = 0;
    page.on('request', req => { if (req.url().includes('/api/quiz/skip')) skipCalls += 1; });
    page.on('dialog', dialog => dialog.accept());

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).not.toBeVisible();
    await page.locator('#skip-today-btn').click();

    await expect(page.locator('#mode-label')).toHaveText('Chinese');
    await expect(page.locator('#prompt-word')).toHaveText('水');
    await submit(page, 'water');
    await expect(page.locator('#result-icon')).toHaveText('Correct');
    expect(skipCalls).toBe(0);
  });

  test('Settings offers Voice → Chinese, by default for Practicing to Mastered', async ({ page }) => {
    await setupUser(page, 'random', [{ zh: '水', pinyin: 'shuǐ', en: ['water'] }]);
    await page.goto('/settings');
    const row = page.locator('[data-random-mode-row="voice_to_zh"]');
    await expect(row).toContainText('Voice → Chinese');
    await expect(page.locator('#random-mode-voice_to_zh-from')).toHaveValue('70-84');
    await expect(page.locator('#random-mode-voice_to_zh-to')).toHaveValue('85-100');
    await expect(page.locator('#random-mode-voice_to_zh-off')).not.toBeChecked();
    await expect(page.locator('#cycle-step-1 option[value="voice_to_zh"]')).toHaveCount(1);
    await row.scrollIntoViewIfNeeded();
    await captureForPR(page, 'settings-voice-to-zh-range');
  });
});
