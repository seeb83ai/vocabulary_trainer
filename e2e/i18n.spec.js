// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// These tests use the auth state created in global-setup (test user + seeded words)
test.use({ storageState: 'e2e/.auth/user.json' });

test.describe('German UI language', () => {
  test('the language selector offers German and switches the nav to German', async ({ page }) => {
    await page.goto('/train');
    await captureForPR(page, 'lang-select-before');

    const langSelect = page.locator('#lang-select');
    await expect(langSelect.locator('option[value="de"]')).toHaveCount(1);

    await langSelect.selectOption('de');

    await expect(page.locator('a[href="/vocab"]')).toHaveText('Vokabeln');
    await expect(page.locator('a[href="/stats"]')).toHaveText('Statistik');
    await captureForPR(page, 'train-page-german');
  });
});
