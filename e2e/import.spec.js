// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

const PASSWORD = 'E2eImportPass123!';

test.describe('Vocabulary → Import', () => {
  test('imports several library lists at once and tags words the user already has', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const email = `e2e-import-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
    await page.goto('/#register');
    await page.locator('#reg-email').fill(email);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    await page.locator('#register-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    // The user already owns 时间 from the HSK 2.0 list.
    const res = await page.request.post('/api/import', { data: { tag: 'hsk2-2', apply_tags: ['hsk2-2'] } });
    expect(res.ok()).toBe(true);

    await page.goto('/vocab');
    await page.locator('#open-import-btn').click();
    await page.locator('#import-tag-list button', { hasText: /^hsk3-2$/ }).click();
    await page.locator('#import-tag-list button', { hasText: /^hsk3-3$/ }).click();
    await expect(page.locator('#import-tag-list button[aria-pressed="true"]')).toHaveCount(2);
    // 大, 时间 + 已经
    await expect(page.locator('#import-preview-stats')).toContainText('3', { timeout: 10_000 });
    await captureForPR(page, 'vocab-import-multi-select');

    await page.locator('#import-next-btn').click();
    await page.locator('#import-next2-btn').click();
    await expect(page.locator('#import-apply-tags')).toContainText('hsk3-2');
    await expect(page.locator('#import-apply-tags')).toContainText('hsk3-3');
    await page.locator('#import-submit-btn').click();

    await expect(page.locator('#import-status')).toHaveClass(/text-green-600/, { timeout: 15_000 });
    await captureForPR(page, 'vocab-import-multi-select-done');

    const words = await page.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
    const tagsByWord = Object.fromEntries(words.words.map(w => [w.zh_text, w.tags]));
    expect(tagsByWord).toEqual({
      '大': ['hsk3-2'],
      '时间': ['hsk2-2', 'hsk3-2'],
      '已经': ['hsk2-2', 'hsk3-3'],
    });
  });
});
