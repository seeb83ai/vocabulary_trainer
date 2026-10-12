// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// Tag groups: tags with the same prefix (text before the first "-") form a
// group. All hsk… tags are one group, tags without "-" are one group.
// Train and the Vocabulary filter match any tag inside a group (OR) and every
// selected group (AND). An ⓘ button next to each tag field explains this.

const PASSWORD = 'E2eTagGroups123!';

async function registerUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
  const email = `e2e-taggroups-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
  const st = await (await page.request.get('/api/settings')).json();
  await page.request.patch('/api/settings', { data: { ...st, primary_lang: 'en', secondary_lang: 'de' } });
}

async function seed(page, zh, pinyin, en, tags) {
  const res = await page.request.post('/api/words', {
    data: { zh_text: zh, pinyin, translations: { en }, tags, start_training: true },
  });
  expect(res.ok()).toBe(true);
}

// 米饭 and 面条 are in an HSK level AND topic-food; 你好 is HSK only,
// 苹果 is topic-food only, 鞋 has a custom tag only.
async function seedWords(page) {
  await seed(page, '米饭', 'mǐfàn', ['rice'], ['hsk3-1', 'topic-food']);
  await seed(page, '面条', 'miàntiáo', ['noodles'], ['hsk2-2', 'topic-food']);
  await seed(page, '你好', 'nǐ hǎo', ['hello'], ['hsk3-1']);
  await seed(page, '苹果', 'píngguǒ', ['apple'], ['topic-food']);
  await seed(page, '鞋', 'xié', ['shoe'], ['lesson1']);
}

test.describe('Tag groups – desktop', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('Train session sheet groups tags per row and combines groups with AND', async ({ page }) => {
    await registerUser(page);
    await seedWords(page);
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: false, tags: [] },
    });
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    await page.locator('#open-filter-overlay').click();
    const chips = page.locator('#overlay-tag-chips');
    await expect(chips.locator('.tag-group')).toHaveCount(3);
    await expect(chips.locator('.tag-group[data-group="hsk"] .tag-group-label')).toHaveText('HSK');
    await expect(chips.locator('.tag-group[data-group="hsk"] .overlay-tag-btn')).toHaveText(['hsk2-2', 'hsk3-1']);
    await expect(chips.locator('.tag-group[data-group="topic"] .tag-group-label')).toHaveText('topic');
    await expect(chips.locator('.tag-group[data-group=""] .tag-group-label')).toHaveText('Other');
    await expect(chips.locator('.tag-group[data-group=""] .overlay-tag-btn')).toHaveText(['lesson1']);

    // The ⓘ button opens the explanation; a click outside closes it.
    const help = page.locator('#overlay-tags-section .tag-help-pop');
    await expect(help).toBeHidden();
    await page.locator('#overlay-tags-section .tag-help-btn').click();
    await expect(help).toBeVisible();
    await expect(help).toContainText('same prefix');
    await expect(help).toContainText('Example');
    await captureForPR(page, 'train-tag-groups-help');
    await page.locator('#overlay-tags-section .tr-label').click();
    await expect(help).toBeHidden();

    await chips.getByRole('button', { name: 'hsk3-1' }).click();
    await chips.getByRole('button', { name: 'hsk2-2' }).click();
    await chips.getByRole('button', { name: 'topic-food' }).click();
    await captureForPR(page, 'train-tag-groups');
    await page.locator('#filter-overlay-close').click();

    // (hsk3-1 OR hsk2-2) AND topic-food → 米饭 and 面条.
    await expect(page.locator('#stats-due')).toHaveText('2', { timeout: 8_000 });
  });

  test('Vocabulary filter uses the same groups', async ({ page }) => {
    await registerUser(page);
    await seedWords(page);
    await page.goto('/vocab');
    const rows = page.locator('#words-tbody .vb-row');
    await expect(rows).toHaveCount(5, { timeout: 10_000 });

    await page.locator('#vocab-more-filters-toggle').click();
    const bar = page.locator('#filter-tags-bar');
    await expect(bar.locator('.tag-group')).toHaveCount(3);
    await expect(bar.locator('.tag-group[data-group="hsk"] .tag-group-label')).toHaveText('HSK');

    await bar.getByRole('button', { name: 'hsk3-1' }).click();
    await expect(rows).toHaveCount(2);
    await bar.getByRole('button', { name: 'topic-food' }).click();
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toContainText('米饭');
    await bar.getByRole('button', { name: 'hsk2-2' }).click();
    await expect(rows).toHaveCount(2);
    await expect(rows.filter({ hasText: '面条' })).toHaveCount(1);
    await captureForPR(page, 'vocab-tag-groups');

    await bar.locator('.tag-help-btn').click();
    await expect(bar.locator('.tag-help-pop')).toBeVisible();
  });

  test('every place that adds or changes tags has the ⓘ explanation', async ({ page }) => {
    await registerUser(page);
    await seedWords(page);
    await page.goto('/vocab');
    await expect(page.locator('#words-tbody .vb-row')).toHaveCount(5, { timeout: 10_000 });

    // Edit sheet.
    await page.locator('#words-tbody .vb-row').filter({ hasText: '鞋' }).click();
    const formHelp = page.locator('#form-tags-help');
    await expect(formHelp.locator('.tag-help-pop')).toBeHidden();
    await formHelp.locator('.tag-help-btn').click();
    await expect(formHelp.locator('.tag-help-pop')).toBeVisible();
    await expect(formHelp.locator('.tag-help-pop')).toContainText('hsk3-1');
    await captureForPR(page, 'vocab-edit-tag-help');

    // The other three places carry the same button.
    await expect(page.locator('#import-tags-help .tag-help-btn')).toHaveCount(1);
    await expect(page.locator('#csv-upload-tags-help .tag-help-btn')).toHaveCount(1);
    await expect(page.locator('#panel-tags-help .tag-help-btn')).toHaveCount(1);
  });
});

test.describe('Tag groups – phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('the ⓘ explanation fits on a phone screen', async ({ page }) => {
    await registerUser(page);
    await seedWords(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await page.locator('#open-filter-overlay').click();
    await page.locator('#overlay-tags-section').scrollIntoViewIfNeeded();
    await page.locator('#overlay-tags-section .tag-help-btn').click();
    const pop = page.locator('#overlay-tags-section .tag-help-pop');
    await expect(pop).toBeVisible();
    const box = await pop.boundingBox();
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(390);
    await captureForPR(page, 'train-tag-groups-help-phone');
  });
});
