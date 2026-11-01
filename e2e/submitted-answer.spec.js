// @ts-check
import { test, expect } from '@playwright/test';
import { syncNewWordMode } from './helpers/mode.js';
import { seedDueComponent } from './helpers/db.js';
import { captureForPR } from './helpers/screenshot.js';

// The result screen shows the answer the server checked (your_answer in the
// answer response), not what the answer field holds when the result arrives.
// A user typed "Löffel", but Enter went through early: the server checked a
// shorter text and the screen showed "Löffel" as wrong.

async function setup(page) {
  const email = `e2e-submitted-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', {
    data: { email, password: 'SubmittedAnswer123!' },
  });
  expect(regRes.ok()).toBeTruthy();
  const settings = await (await page.request.get('/api/settings')).json();
  await page.request.patch('/api/settings', {
    data: { ...settings, wrong_answer_retry_mode: 'off' },
  });
  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_pinyin_to_transl', langs: ['en'], bucket: '', mnemonics: false, components: true, tags: [] },
  });
  await syncNewWordMode(page, 'zh_pinyin_to_transl');
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_pinyin_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  return email;
}

const componentCard = {
  card_type: 'component', prompt: '匕', pinyin: 'bǐ', is_new: false, is_also_word: false,
  definitions: { en: 'spoon, ladle; knife, dirk' },
};

test.describe('Result screen shows the answer the server checked', () => {
  test('component card: text typed while the answer is checked is not shown', async ({ page }) => {
    const email = await setup(page);
    const seedRes = await page.request.post('/api/words', {
      data: { zh_text: '苹果', pinyin: 'píngguǒ', translations: { en: ['apple'] }, tags: [] },
    });
    expect(seedRes.ok()).toBeTruthy();
    seedDueComponent(email, '匕', 'spoon, ladle; knife, dirk');
    await page.route('**/api/quiz/next*', route => route.fulfill({
      status: 200, contentType: 'application/json', body: JSON.stringify(componentCard),
    }));
    // Hold the answer request until the test has typed more text.
    let release = () => {};
    const released = new Promise(resolve => { release = resolve; });
    await page.route('**/api/component/answer', async route => {
      await released;
      await route.continue();
    });

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.locator('#answer-input').fill('spo');
    await page.locator('#answer-input').press('Enter');
    await page.locator('#answer-input').pressSequentially('on');
    await expect(page.locator('#answer-input')).toHaveValue('spoon');
    release();

    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('.result-your-answer .tr-struck')).toHaveText('spo');
    await captureForPR(page, 'submitted-answer-component');
  });

  test('word card: shows the answer the server received', async ({ page }) => {
    await setup(page);
    const seedRes = await page.request.post('/api/words', {
      data: { zh_text: '橙子', pinyin: 'chéngzi', translations: { en: ['orange'] }, tags: [], start_training: true },
    });
    expect(seedRes.ok()).toBeTruthy();
    await rewriteAnswer(page, '**/api/quiz/answer', 'lemon');

    await page.goto('/train');
    await expect(page.locator('#prompt-word')).toHaveText('橙子', { timeout: 12_000 });
    await page.locator('#answer-input').fill('orange');
    await page.locator('#answer-input').press('Enter');

    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('.result-your-answer .tr-struck')).toHaveText('lemon');
  });

  test('landing-page demo: shows the answer the server received', async ({ page }) => {
    await rewriteAnswer(page, '**/api/demo/answer', 'goodbye');

    await page.goto('/');
    await expect(page.locator('#demo-zh')).toHaveText('你好');
    await page.locator('#demo-input').fill('hello');
    await page.locator('#demo-submit').click();

    await expect(page.locator('#demo-icon')).toContainText(/not quite/i);
    await expect(page.locator('#demo-breakdown')).toContainText('goodbye');
  });
});

// Changes the answer in the request on its way to the server, so the server
// checks a different text from the one in the answer field.
function rewriteAnswer(page, url, answer) {
  return page.route(url, route => route.continue({
    postData: JSON.stringify({ ...route.request().postDataJSON(), answer }),
  }));
}
