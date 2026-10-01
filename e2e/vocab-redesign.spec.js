// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// Redesigned Vocabulary page (app redesign handoff): header with summary,
// Import / Add word / ⋯ menu, a toolbar with Words/Components, search and
// filter chips, "More filters" (level, tags, missing language, sort), a row
// list, and an Add/Edit sheet. Each test registers its own user.

const PASSWORD = 'E2eVocabRedesign123!';

async function registerUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
  const email = `e2e-vocab-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
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

test.describe('Vocabulary redesign – desktop', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('header, toolbar and row list', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', { en: ['hello', 'hi'], de: ['hallo'] }, ['greetings']);
    await seed(page, '鞋', 'xié', { en: ['shoe'] });
    await page.goto('/vocab');

    await expect(page.locator('#vocab-summary')).toHaveText(/^2 words · \d+ due today$/, { timeout: 10_000 });
    const rows = page.locator('#words-tbody .vb-row');
    await expect(rows).toHaveCount(2);
    const shoe = rows.filter({ hasText: '鞋' });
    await expect(shoe).toContainText('xié');
    await expect(shoe.locator('.vb-missing')).toHaveText('missing');
    await expect(shoe.locator('.tier-chip')).toBeVisible();
    await captureForPR(page, 'vocab-list');

    // Filter chips: Missing DE keeps only the word without a German meaning.
    await page.locator('#missing-de-chip').click();
    await expect(page.locator('#missing-de-chip')).toHaveAttribute('aria-pressed', 'true');
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText('鞋');
    await page.locator('#filter-all-btn').click();
    await expect(rows).toHaveCount(2);

    // More filters: level chips, tags and sort.
    await page.locator('#vocab-more-filters-toggle').click();
    await expect(page.locator('#vocab-more-filters')).toBeVisible();
    await expect(page.locator('#filter-tags-bar')).toContainText('greetings');
    await page.locator('#sort-select').selectOption('pinyin');
    await expect(rows.first()).toContainText('你好');
    await page.locator('#sort-dir-btn').click();
    await expect(rows.first()).toContainText('鞋');
    await captureForPR(page, 'vocab-more-filters');
  });

  test('Add word opens the sheet; saving closes it and lists the word', async ({ page }) => {
    await registerUser(page);
    await page.goto('/vocab');

    await expect(page.locator('#vocab-sheet')).toBeHidden();
    await page.locator('#open-add-btn').click();
    await expect(page.locator('#vocab-sheet')).toBeVisible();
    await expect(page.locator('#panel-add')).toBeVisible();
    await page.locator('#form-zh').fill('水');
    await page.locator('#en-inputs-container .en-input').first().fill('water');
    await page.locator('#form-start-training').check();
    await captureForPR(page, 'vocab-add-sheet');
    await page.locator('#word-form button[type="submit"]').click();

    await expect(page.locator('#vocab-sheet')).toBeHidden({ timeout: 8_000 });
    await expect(page.locator('#words-tbody')).toContainText('水');
  });

  test('tapping a row opens the edit sheet with known, reset and delete', async ({ page }) => {
    await registerUser(page);
    await seed(page, '鞋', 'xié', { en: ['shoe'] });
    await page.goto('/vocab');

    await page.locator('#words-tbody .vb-row', { hasText: '鞋' }).click();
    await expect(page.locator('#vocab-sheet')).toBeVisible();
    await expect(page.locator('#form-zh')).toHaveValue('鞋');
    await expect(page.locator('#form-known-btn')).toBeVisible();
    await expect(page.locator('#form-delete-btn')).toBeVisible();
    await captureForPR(page, 'vocab-edit-sheet');

    page.once('dialog', d => d.accept());
    await page.locator('#form-delete-btn').click();
    await expect(page.locator('#vocab-sheet')).toBeHidden({ timeout: 8_000 });
    await expect(page.locator('#words-tbody .vb-row')).toHaveCount(0);
  });

  test('the ⋯ menu offers download, CSV upload and tag management', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', { en: ['hello'] }, ['greetings']);
    await page.goto('/vocab');

    await page.locator('#vocab-menu-btn').click();
    await expect(page.locator('#download-btn')).toBeVisible();
    await expect(page.locator('#csv-upload-btn')).toBeVisible();
    await captureForPR(page, 'vocab-menu');
    await page.locator('#open-tags-btn').click();
    await expect(page.locator('#vocab-sheet')).toBeVisible();
    await expect(page.locator('#panel-tags')).toBeVisible();
    await expect(page.locator('#tags-list')).toContainText('greetings');
  });
});

test.describe('Vocabulary redesign – phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('rows and the add sheet fit a phone', async ({ page }) => {
    await registerUser(page);
    await seed(page, '你好', 'nǐ hǎo', { en: ['hello', 'hi'], de: ['hallo'] });
    await page.goto('/vocab');

    await expect(page.locator('#words-tbody .vb-row')).toHaveCount(1, { timeout: 10_000 });
    await captureForPR(page, 'vocab-list-phone');
    await page.locator('#open-add-btn').click();
    await expect(page.locator('#vocab-sheet')).toBeVisible();
    const sheet = await page.locator('#vocab-sheet .vb-sheet-panel').boundingBox();
    expect(sheet).toBeTruthy();
    expect(sheet.x).toBeGreaterThanOrEqual(0);
    expect(sheet.x + sheet.width).toBeLessThanOrEqual(390 + 0.5);
  });
});
