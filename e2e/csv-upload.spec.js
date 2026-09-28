// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

const PASSWORD = 'E2eCsvUploadPass123!';

/**
 * Register a brand-new user, so the uploaded words do not leak into the
 * seeded user's vocabulary that the other specs (quiz, stats) rely on.
 * @param {import('@playwright/test').Page} page
 */
async function registerFreshUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => {
    route.fulfill({ status: 200, body: '' });
  });
  const email = `e2e-csv-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
}

/**
 * Upload a CSV through the "Upload CSV" dialog.
 * @param {import('@playwright/test').Page} page
 * @param {string} csv
 * @param {string} tag
 * @param {string} defaultSource - value of the "translations are" select
 */
async function uploadCsv(page, csv, tag, defaultSource) {
  await registerFreshUser(page);
  await page.goto('/vocab');
  await page.locator('#csv-upload-btn').click();
  await page.locator('#csv-upload-file').setInputFiles({
    name: 'words.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from(csv),
  });
  await page.locator('#csv-upload-tag-input').fill(tag);
  await page.locator('#csv-upload-tag-input').press('Enter');
  await page.locator('#csv-upload-default-source').selectOption(defaultSource);
  return page.locator('#csv-upload-submit-btn');
}

/** @returns {Promise<Record<string, Record<string, string[]>>>} zh text → lang → sources */
async function sourcesByWord(page, tag) {
  const res = await page.evaluate(t => fetch(`/api/words/?per_page=50&tags=${t}`).then(r => r.json()), tag);
  return Object.fromEntries(res.words.map(w => [w.zh_text, w.translation_sources]));
}

test.describe('CSV upload: translation source', () => {
  test('the dialog default marks translations of a CSV without a source column', async ({ page }) => {
    const submit = await uploadCsv(page, 'chinese,pinyin,en\n茶,chá,tea\n酒,jiǔ,liquor; wine\n', 'csv-src-default', 'cedict');
    await expect(page.locator('#csv-upload-default-source')).toHaveValue('cedict');
    await captureForPR(page, 'vocab-csv-upload-default-source');
    await submit.click();
    await expect(page.locator('#csv-upload-status')).toContainText('imported: 2', { timeout: 10_000 });

    expect(await sourcesByWord(page, 'csv-src-default')).toEqual({
      '茶': { en: ['cedict'] },
      '酒': { en: ['cedict', 'cedict'] },
    });
  });

  test('a source column overrides the default per row; empty cells use the default; invalid rows are skipped', async ({ page }) => {
    const csv = [
      'chinese,pinyin,en,source',
      '汤,tāng,soup,cedict',
      '饭,fàn,rice,user',
      '面,miàn,noodles,',
      '肉,ròu,meat,bogus',
      '',
    ].join('\n');
    const submit = await uploadCsv(page, csv, 'csv-src-column', 'user');
    await submit.click();
    await expect(page.locator('#csv-upload-status')).toContainText('imported: 3', { timeout: 10_000 });
    await expect(page.locator('#csv-upload-status')).toContainText('skipped: 1');

    expect(await sourcesByWord(page, 'csv-src-column')).toEqual({
      '汤': { en: ['cedict'] },
      '饭': { en: ['user'] },
      '面': { en: ['user'] },
    });
  });
});
