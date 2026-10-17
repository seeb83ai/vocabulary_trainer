// @ts-check
import { test, expect } from '@playwright/test';
import { syncNewWordMode } from './helpers/mode.js';
import { seedDueComponent } from './helpers/db.js';

// The Train session bar "X of Y today" (issue #583): every answer counts as
// done, whether it was a word, a component or a mnemonic scene, and
// "Accept as correct" does not count the card a second time.

async function setup(page, { mnemonics = true, components = true, placeholderWord = true } = {}) {
  const email = `e2e-progress-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', {
    data: { email, password: 'SessionProgress123!' },
  });
  expect(regRes.ok()).toBeTruthy();
  const settings = await (await page.request.get('/api/settings')).json();
  await page.request.patch('/api/settings', {
    data: { ...settings, wrong_answer_retry_mode: 'off' },
  });
  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_pinyin_to_transl', langs: ['en'], bucket: '', mnemonics, components, tags: [] },
  });
  await syncNewWordMode(page, 'zh_pinyin_to_transl');
  // A word that is not in training yet, so the page shows the quiz and not
  // the first-vocabulary setup.
  if (placeholderWord) {
    const seedRes = await page.request.post('/api/words', {
      data: { zh_text: '苹果', pinyin: 'píngguǒ', translations: { en: ['apple'] }, tags: [] },
    });
    expect(seedRes.ok()).toBeTruthy();
  }
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_pinyin_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  return email;
}

// Serves the given cards in order from /api/quiz/next; answers still go to
// the real server, so the counters are real.
function serveCards(page, cards) {
  let i = 0;
  return page.route('**/api/quiz/next*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(cards[Math.min(i++, cards.length - 1)]),
  }));
}

async function progress(page) {
  const text = await page.locator('#session-progress-label').textContent();
  const m = /^(\d+) of (\d+) today$/.exec(text || '');
  expect(m, `unexpected progress label "${text}"`).not.toBeNull();
  return { done: Number(m?.[1]), total: Number(m?.[2]) };
}

// The stats the page requests next. The label can already show the expected
// text from before, so the tests wait for the reload.
function nextStats(page) {
  return page.waitForRequest(r => r.url().includes('/api/quiz/stats')).then(r => r.response());
}

async function expectDone(page, statsRes, done) {
  const res = await statsRes;
  expect(res).not.toBeNull();
  const stats = await res.json();
  expect(stats.today_attempts).toBe(done);
  await expect(page.locator('#session-progress-label')).toHaveText(new RegExp(`^${done} of \\d+ today$`));
}

async function answer(page, text) {
  await page.locator('#answer-input').fill(text);
  await page.locator('#answer-form button[type="submit"]').click();
  await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
}

const componentCard = {
  card_type: 'component', prompt: '木', pinyin: 'mù', is_new: false, is_also_word: false,
  definitions: { en: 'tree' },
};

test.describe('Session progress counts everything trained', () => {
  test('a component answer counts as done and keeps the total', async ({ page }) => {
    const email = await setup(page);
    seedDueComponent(email, '木', 'tree');
    await serveCards(page, [componentCard]);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#session-progress-label')).toHaveText(/^0 of [1-9]\d* today$/);
    const before = await progress(page);

    await answer(page, 'tree');
    await expect(page.locator('#session-progress-label')).toHaveText(`1 of ${before.total} today`);
  });

  test('a mnemonic scene answer counts as done', async ({ page }) => {
    await setup(page);
    const res = await page.request.put('/api/hmm/props', { data: { radical: '木', prop_name: 'Wooden chair' } });
    expect(res.ok()).toBeTruthy();
    await serveCards(page, [{ card_type: 'hmm', entity_type: 'prop', entity_key: '木', prompt: '木' }]);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#session-progress-label')).toHaveText(/^0 of [1-9]\d* today$/);

    // A new scene is in its learning steps and is due again later today, so
    // only "done" is checked here.
    await answer(page, 'Wooden chair');
    await expect(page.locator('#session-progress-label')).toHaveText(/^1 of \d+ today$/);
  });

  test('"Accept as correct" on a word counts the card once', async ({ page }) => {
    await setup(page, { mnemonics: false, components: false, placeholderWord: false });
    const seedRes = await page.request.post('/api/words', {
      data: { zh_text: '橙子', pinyin: 'chéngzi', translations: { en: ['orange'] }, tags: [], start_training: true },
    });
    expect(seedRes.ok()).toBeTruthy();
    await page.goto('/train');
    await expect(page.locator('#prompt-word')).toHaveText('橙子', { timeout: 12_000 });

    await answer(page, 'orang');
    await expect(page.locator('#accept-correct-btn')).toBeVisible();
    const acceptRes = page.waitForResponse(r => r.url().includes('/api/quiz/accept-correct'));
    const statsRes = nextStats(page);
    await page.locator('#accept-correct-btn').scrollIntoViewIfNeeded();
    await page.locator('#accept-correct-btn').click();
    expect((await acceptRes).status()).toBe(200);
    await expectDone(page, statsRes, 1);
  });

  test('"Accept as correct" on a component counts the card once', async ({ page }) => {
    const email = await setup(page);
    seedDueComponent(email, '木', 'tree');
    await serveCards(page, [componentCard]);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await answer(page, 'tre');
    await expect(page.locator('#accept-correct-btn')).toBeVisible();
    const acceptRes = page.waitForResponse(r => r.url().includes('/api/component/accept-correct'));
    const statsRes = nextStats(page);
    await page.locator('#accept-correct-btn').scrollIntoViewIfNeeded();
    await page.locator('#accept-correct-btn').click();
    expect((await acceptRes).status()).toBe(200);
    await expectDone(page, statsRes, 1);
  });
});
