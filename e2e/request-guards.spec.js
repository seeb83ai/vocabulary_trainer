// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';
import { syncNewWordMode } from './helpers/mode.js';

// Request guards (follow-up to issue #563): every client request has a
// timeout, an action can't run twice while its request is open, and a reload
// started by a filter change cancels the older one, so nothing renders twice
// or out of order. Timeouts use Playwright's fake clock, so a 15 s timeout
// costs no real time. Every test registers its own user.

const PASSWORD = 'E2eRequestGuards123!';
const TIMEOUT_TEXT = 'The server did not answer. Try again.';

async function registerUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
  const email = `e2e-guards-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
}

async function seed(page, zh, pinyin, en, startTraining = true) {
  const res = await page.request.post('/api/words', {
    data: { zh_text: zh, pinyin, translations: { en }, tags: [], start_training: startTraining },
  });
  expect(res.ok()).toBe(true);
  return (await res.json()).id;
}

async function useMode(page, mode) {
  await page.request.patch('/api/training-filters', {
    data: { mode, langs: ['en'], bucket: '', mnemonics: true, components: false, tags: [] },
  });
  await syncNewWordMode(page, mode);
}

// Counts requests to a URL pattern and holds each one for delayMs before it
// goes to the server.
async function countAndDelay(page, pattern, delayMs) {
  const counter = { n: 0 };
  await page.route(pattern, async route => {
    counter.n++;
    await new Promise(r => setTimeout(r, delayMs));
    await route.continue();
  });
  return counter;
}

// A request that never answers. The page's own timeout must end it.
async function hang(page, pattern) {
  await page.route(pattern, () => {});
}

test.describe('Request guards – Train', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('fast filter changes show only the newest card', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await seed(page, '再见', 'zài jiàn', ['bye']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // The older request (mnemonics off) answers late; the newer one at once.
    await page.route('**/api/quiz/next**', async route => {
      const slow = route.request().url().includes('mnemonics=false');
      const res = await route.fetch();
      const card = await res.json();
      if (slow) await new Promise(r => setTimeout(r, 1_500));
      await route.fulfill({ response: res, json: { ...card, prompt: slow ? '旧旧' : '新新' } });
    });
    await page.locator('#open-filter-overlay').click();
    await page.locator('#overlay-mnemonics-pill').click();
    await page.locator('#overlay-mnemonics-pill').click();
    await page.locator('#filter-overlay-close').click();

    await expect(page.locator('#prompt-word')).toHaveText('新新', { timeout: 8_000 });
    await page.waitForTimeout(2_000);
    await expect(page.locator('#prompt-word')).toHaveText('新新');
  });

  test('a double tap on Skip sends one request', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await seed(page, '再见', 'zài jiàn', ['bye']);
    await useMode(page, 'zh_to_transl');
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    const skips = await countAndDelay(page, '**/api/quiz/skip', 600);
    await page.locator('#skip-today-btn').dblclick();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    await page.waitForTimeout(800);
    expect(skips.n).toBe(1);
  });

  test('a double tap on a new word\'s Skip sends one request', async ({ page }) => {
    await registerUser(page);
    await seed(page, '年', 'nián', ['year'], false);
    await page.request.patch('/api/training-filters', {
      data: { mode: 'progressive', langs: ['en'], bucket: '', mnemonics: true, components: false, tags: [] },
    });
    await page.goto('/train');
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 12_000 });

    const skips = await countAndDelay(page, '**/api/quiz/skip', 600);
    await page.locator('#new-word-skip-btn').dblclick();
    await page.waitForTimeout(1_500);
    expect(skips.n).toBe(1);
  });

  test('an action that does not answer shows a toast and can be tried again', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'zh_to_transl');
    await page.clock.install();
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await hang(page, '**/api/quiz/skip');
    await page.locator('#skip-today-btn').click();
    await expect(page.locator('#skip-today-btn')).toBeDisabled();
    await page.clock.fastForward(16_000);

    const toast = page.locator('[data-testid="toast"]');
    await expect(toast).toBeVisible();
    await expect(toast).toContainText(TIMEOUT_TEXT);
    await expect(page.locator('#skip-today-btn')).toBeEnabled();
    await captureForPR(page, 'request-guards-timeout-toast');
  });

  test('a card load that does not answer shows the error card', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await useMode(page, 'zh_to_transl');
    await page.clock.install();
    await hang(page, '**/api/quiz/next**');
    await page.goto('/train');

    await page.waitForTimeout(500);
    await page.clock.fastForward(16_000);
    await expect(page.locator('#error-state')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#error-msg')).toHaveText(TIMEOUT_TEXT);
  });
});

test.describe('Request guards – Vocabulary', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('a double tap on Save adds the word once', async ({ page }) => {
    await registerUser(page);
    await page.goto('/vocab');
    await page.locator('#open-add-btn').click();
    await page.locator('#form-zh').fill('水');
    await page.locator('#en-inputs-container .en-input').first().fill('water');
    await page.locator('#form-start-training').check();

    let posts = 0;
    await page.route('**/api/words', async route => {
      if (route.request().method() === 'POST') {
        posts++;
        await new Promise(r => setTimeout(r, 600));
      }
      await route.continue();
    });
    await page.locator('#word-form button[type="submit"]').dblclick();

    await expect(page.locator('#vocab-sheet')).toBeHidden({ timeout: 8_000 });
    await expect(page.locator('#words-tbody .vb-row', { hasText: '水' })).toHaveCount(1);
    expect(posts).toBe(1);
  });

  test('fast search typing shows only the newest results', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', ['hello']);
    await seed(page, '你们', 'nǐmen', ['you']);
    await page.goto('/vocab');
    await expect(page.locator('#words-tbody .vb-row')).toHaveCount(2, { timeout: 10_000 });

    // The older search ("你") answers late, with a marked first row.
    await page.route('**/api/words?**', async route => {
      const q = new URL(route.request().url()).searchParams.get('q');
      if (q !== '你') return route.continue();
      const res = await route.fetch();
      const data = await res.json();
      await new Promise(r => setTimeout(r, 1_500));
      data.words[0].zh_text = '旧旧';
      await route.fulfill({ response: res, json: data });
    });
    await page.locator('#search-input').fill('你');
    await page.waitForTimeout(500);
    await page.locator('#search-input').fill('你好');

    const rows = page.locator('#words-tbody .vb-row');
    await expect(rows).toHaveCount(1, { timeout: 8_000 });
    await expect(rows.first()).toContainText('你好');
    await page.waitForTimeout(2_000);
    await expect(rows).toHaveCount(1);
    await expect(page.locator('#words-tbody')).not.toContainText('旧旧');
  });

  test('Translate waits for the slow-call timeout, then shows a toast', async ({ page }) => {
    await registerUser(page);
    await page.clock.install();
    await page.goto('/vocab');
    await page.locator('#open-add-btn').click();
    await page.locator('#form-zh').fill('水');
    await expect(page.locator('#translate-btn')).toBeVisible();

    await hang(page, '**/api/translate');
    await page.locator('#translate-btn').click();
    await expect(page.locator('#translate-btn')).toBeDisabled();
    // The default 15 s timeout doesn't apply: DeepL may take longer.
    await page.clock.fastForward(16_000);
    await expect(page.locator('#translate-btn')).toBeDisabled();
    await page.clock.fastForward(45_000);
    await expect(page.locator('[data-testid="toast"]')).toContainText(TIMEOUT_TEXT);
    await expect(page.locator('#translate-btn')).toBeEnabled();
  });
});

test.describe('Request guards – Settings and Stats', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('two quick setting changes are saved in order', async ({ page }) => {
    await registerUser(page);
    await page.goto('/settings');
    const select = page.locator('#match-game-pinyin-reveal');
    await expect(select).toHaveValue('always');

    // The first save reaches the server late. Without ordering it would
    // overwrite the second one.
    let patches = 0;
    await page.route('**/api/settings', async route => {
      if (route.request().method() === 'PATCH' && ++patches === 1) {
        await new Promise(r => setTimeout(r, 1_500));
      }
      await route.continue();
    });
    await select.selectOption('off');
    await page.waitForTimeout(600);
    await select.selectOption('after_correct');
    await page.waitForTimeout(3_000);

    const st = await (await page.request.get('/api/settings')).json();
    expect(st.match_game_pinyin_reveal).toBe('after_correct');
  });

  test('a setting save that does not answer shows a toast', async ({ page }) => {
    await registerUser(page);
    await page.clock.install();
    await page.goto('/settings');
    const select = page.locator('#match-game-pinyin-reveal');
    await expect(select).toHaveValue('always');

    await page.route('**/api/settings', route =>
      route.request().method() === 'PATCH' ? undefined : route.continue());
    const saving = page.waitForRequest(req => req.url().endsWith('/api/settings') && req.method() === 'PATCH');
    await select.selectOption('off');
    await saving;
    await page.clock.fastForward(16_000);
    await expect(page.locator('[data-testid="toast"]')).toContainText(TIMEOUT_TEXT);
  });

  test('fast tag changes on Stats show only the newest word stats', async ({ page }) => {
    await registerUser(page);
    const res = await page.request.post('/api/words', {
      data: { zh_text: '你好', pinyin: 'nǐ hǎo', translations: { en: ['hello'] }, tags: ['greetings'], start_training: true },
    });
    expect(res.ok()).toBe(true);
    await page.goto('/stats');
    const chip = page.locator('#bucket-tag-chips button', { hasText: 'greetings' });
    await expect(chip).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#word-stats-section')).toBeVisible();

    // The older request (tag selected) answers late with no words at all,
    // which would hide the section.
    await page.route('**/api/quiz/word-stats**', async route => {
      if (!route.request().url().includes('tags=')) return route.continue();
      const res = await route.fetch();
      const ws = await res.json();
      await new Promise(r => setTimeout(r, 1_500));
      await route.fulfill({ response: res, json: { ...ws, total_seen: 0, accuracy_buckets: { ...ws.accuracy_buckets, unseen: 0 } } });
    });
    await chip.click();
    await chip.click();
    await page.waitForTimeout(2_500);
    await expect(page.locator('#word-stats-section')).toBeVisible();
  });
});

test.describe('Request guards – Pinyin and Mnemonics', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  const pinyinCard = labels => ({
    sound_id: 11, audio_file: 'ma3.mp3', mode: 'multiple_choice',
    options: labels.map((label, i) => ({ sound_id: 10 + i, label })),
  });

  test('fast group changes on Pinyin show only the newest card', async ({ page }) => {
    await registerUser(page);
    await page.route('**/api/pinyin-quiz/audio/**', r => r.fulfill({ status: 204, body: '' }));
    await page.route('**/api/pinyin-quiz/tags', r => r.fulfill({ json: ['tones', 'initials'] }));
    await page.route('**/api/pinyin-quiz/stats**', r => r.fulfill({ json: { due_today: 4, total: 1600 } }));
    // The "tones" card answers late, after the "initials" card.
    await page.route('**/api/pinyin-quiz/next**', async r => {
      const tags = new URL(r.request().url()).searchParams.get('tags');
      if (tags === 'tones') {
        await new Promise(res => setTimeout(res, 1_500));
        return r.fulfill({ json: pinyinCard(['旧', '旧', '旧', '旧']) });
      }
      return r.fulfill({ json: pinyinCard(['mā', 'má', 'mǎ', 'mà']) });
    });
    await page.goto('/pinyin');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 10_000 });

    await page.locator('#tag-chips .tag-btn', { hasText: 'tones' }).click();
    await page.locator('#tag-chips .tag-btn', { hasText: 'initials' }).click();
    await page.waitForTimeout(2_500);
    await expect(page.locator('#mc-options .mc-btn').first()).toHaveText(/m/);
    await expect(page.locator('#mc-options')).not.toContainText('旧');
  });

  test('a double tap on Add prop sends one request', async ({ page }) => {
    await registerUser(page);
    for (const path of ['actors', 'locations', 'tone-rooms']) {
      await page.route(`**/api/hmm/${path}`, r => r.fulfill({ json: [] }));
    }
    let puts = 0;
    await page.route('**/api/hmm/props', async r => {
      if (r.request().method() === 'PUT') {
        puts++;
        await new Promise(res => setTimeout(res, 600));
        return r.fulfill({ json: { ok: true } });
      }
      return r.fulfill({ json: [] });
    });
    await page.goto('/mnemonics');
    await page.locator('#mn-tab-props').click();
    await page.locator('#new-prop-radical').fill('木');
    await page.locator('#new-prop-name').fill('tree');
    await page.locator('#add-prop-btn').dblclick();
    await page.waitForTimeout(1_500);
    expect(puts).toBe(1);
  });
});

test.describe('Request guards – Landing page', () => {
  test('a double tap on the demo Check sends one answer', async ({ page }) => {
    let answers = 0;
    await page.route('**/api/demo/answer', async route => {
      answers++;
      await new Promise(r => setTimeout(r, 600));
      await route.continue();
    });
    await page.goto('/');
    await expect(page.locator('#demo-card')).toBeVisible();
    await page.locator('#demo-input').fill('hello');
    await page.locator('#demo-submit').dblclick();

    await expect(page.locator('#demo-icon')).toContainText(/correct/i);
    await page.waitForTimeout(800);
    expect(answers).toBe(1);
  });

  test('a registration that does not answer shows the timeout and can be tried again', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
    await hang(page, '**/api/register');
    await page.clock.install();
    await page.goto('/#register');
    await page.locator('#reg-email').fill(`e2e-guards-hang-${Date.now()}@test.local`);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    const registering = page.waitForRequest('**/api/register');
    await page.locator('#register-btn').click();
    await registering;
    await page.clock.fastForward(16_000);

    await expect(page.locator('#register-error')).toHaveText(TIMEOUT_TEXT);
    await expect(page.locator('#register-btn')).toBeEnabled();
  });
});
