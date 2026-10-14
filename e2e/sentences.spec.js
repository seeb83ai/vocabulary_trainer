// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// Vocabulary → ⋯ → Generate sentences: pick tags and a count, let the LLM
// write example sentences from known words, pick the good ones and add them
// with a sentence tag (`_s` is added to the tag name). The LLM call is mocked;
// adding the sentences uses the real word API.

const PASSWORD = 'E2eSentences123!';

async function registerUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
  const email = `e2e-sent-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
  const st = await (await page.request.get('/api/settings')).json();
  await page.request.patch('/api/settings', { data: { ...st, primary_lang: 'en', secondary_lang: 'de' } });
}

async function seed(page, zh, pinyin, translations, tags = []) {
  const res = await page.request.post('/api/words', {
    data: { zh_text: zh, pinyin, translations, tags, start_training: true },
  });
  expect(res.ok()).toBe(true);
}

async function mockConfig(page, llmAvailable) {
  await page.route('**/api/config', r => r.fulfill({
    json: { deepl_configured: false, deepl_available: false, llm_configured: llmAvailable, llm_available: llmAvailable },
  }));
}

const GENERATED = {
  focus_words: ['我', '吃', '米饭'],
  sentences: [
    { zh_text: '我吃米饭。', pinyin: 'wǒ chī mǐfàn.', translations: { en: ['I eat rice.'], de: ['Ich esse Reis.'] } },
    { zh_text: '你吃米饭吗？', pinyin: 'nǐ chī mǐfàn ma?', translations: { en: ['Do you eat rice?'], de: ['Isst du Reis?'] } },
  ],
};

test.describe('Generate sentences', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('generates sentences for the selected tags and adds the picked ones with a _s tag', async ({ page }) => {
    await registerUser(page);
    await seed(page, '米饭', 'mǐfàn', { en: ['rice'] }, ['food']);
    await seed(page, '你好', 'nǐ hǎo', { en: ['hello'] }, ['greetings']);
    await mockConfig(page, true);
    let generateBody = null;
    await page.route('**/api/sentences/generate', r => {
      generateBody = r.request().postDataJSON();
      return r.fulfill({ json: GENERATED });
    });

    await page.goto('/vocab');
    await page.locator('#vocab-menu-btn').click();
    await page.locator('#open-sentences-btn').click();
    await expect(page.locator('#vocab-sheet')).toBeVisible();
    await expect(page.locator('#panel-sentences')).toBeVisible();

    await page.locator('#sentences-tag-groups button', { hasText: 'food' }).click();
    await page.locator('#sentences-count').selectOption('5');
    await captureForPR(page, 'vocab-sentences-setup');
    await page.locator('#sentences-generate-btn').click();

    const rows = page.locator('#sentences-results .sentence-row');
    await expect(rows).toHaveCount(2);
    expect(generateBody).toEqual({ tags: ['food'], count: 5 });
    await expect(page.locator('#panel-sentences')).not.toContainText('Practised words');
    await expect(rows.nth(0)).toContainText('我吃米饭。');
    await expect(rows.nth(0).locator('input[data-lang=en]')).toHaveValue('I eat rice.');
    await expect(rows.nth(0).locator('input[data-lang=de]')).toHaveValue('Ich esse Reis.');
    await expect(rows.nth(0).locator('input[data-field=pinyin]')).toHaveValue('wǒ chī mǐfàn.');
    await expect(rows.nth(0).locator('input[type=checkbox]')).toBeChecked();

    await rows.nth(1).locator('input[type=checkbox]').uncheck();
    await page.locator('#sentences-tag-input').fill('daily');
    await expect(page.locator('#sentences-tag-hint')).toContainText('daily_s');
    await expect(page.locator('#sentences-add-btn')).toBeEnabled();
    await page.waitForTimeout(300); // let the button's background transition end for the screenshot
    await captureForPR(page, 'vocab-sentences-results');
    await page.locator('#sentences-add-btn').click();

    await expect(page.locator('#sentences-status')).toContainText('1');
    const res = await page.request.get('/api/words?tags=daily_s&per_page=50');
    const data = await res.json();
    const words = data.words || data;
    expect(words.map(w => w.zh_text)).toEqual(['我吃米饭。']);
    expect(words[0].tags).toContain('daily_s');
    expect(words[0].translations.en).toEqual(['I eat rice.']);
    expect(words[0].translations.de).toEqual(['Ich esse Reis.']);
  });

  test('generating waits for the slow-call timeout, then shows the timeout message', async ({ page }) => {
    await registerUser(page);
    await seed(page, '米饭', 'mǐfàn', { en: ['rice'] }, ['food']);
    await mockConfig(page, true);
    await page.route('**/api/sentences/generate', () => {}); // the LLM never answers
    await page.clock.install();

    await page.goto('/vocab');
    await page.locator('#vocab-menu-btn').click();
    await page.locator('#open-sentences-btn').click();
    await page.locator('#sentences-generate-btn').click();
    await expect(page.locator('#sentences-loading')).toBeVisible();
    // The default 15 s timeout doesn't apply: the LLM may take up to a minute.
    await page.clock.fastForward(16_000);
    await expect(page.locator('#sentences-loading')).toBeVisible();
    await page.clock.fastForward(45_000);
    await expect(page.locator('#sentences-status')).toContainText('The server did not answer. Try again.');
    await expect(page.locator('#sentences-generate-btn')).toBeEnabled();
  });

  test('without LLM access the tab explains why and cannot generate', async ({ page }) => {
    await registerUser(page);
    await seed(page, '米饭', 'mǐfàn', { en: ['rice'] }, ['food']);
    await mockConfig(page, false);

    await page.goto('/vocab');
    await page.locator('#vocab-menu-btn').click();
    await page.locator('#open-sentences-btn').click();
    await expect(page.locator('#sentences-no-llm')).toBeVisible();
    await expect(page.locator('#sentences-generate-btn')).toBeDisabled();
  });
});

test.describe('Generate sentences – phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('the tab bar and the results fit a phone', async ({ page }) => {
    await registerUser(page);
    await seed(page, '米饭', 'mǐfàn', { en: ['rice'] }, ['food']);
    await mockConfig(page, true);
    await page.route('**/api/sentences/generate', r => r.fulfill({ json: GENERATED }));

    await page.goto('/vocab');
    await page.locator('#vocab-menu-btn').click();
    await page.locator('#open-sentences-btn').click();
    await page.locator('#sentences-generate-btn').click();
    await expect(page.locator('#sentences-results .sentence-row')).toHaveCount(2);

    const tabs = page.locator('#word-form-panel .vb-tabs');
    const tabsBox = await tabs.boundingBox();
    const lastTab = await page.locator('#tab-comp').boundingBox();
    expect(lastTab.x + lastTab.width).toBeLessThanOrEqual(tabsBox.x + tabsBox.width + 1);
    const overflow = await page.locator('#word-form-panel').evaluate(el => el.scrollWidth - el.clientWidth);
    expect(overflow).toBeLessThanOrEqual(0);
    await captureForPR(page, 'vocab-sentences-phone');
  });
});
