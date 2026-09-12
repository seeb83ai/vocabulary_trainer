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

  test('the settings page translates previously English-only text into German', async ({ page }) => {
    await page.goto('/settings');
    await page.locator('#lang-select').selectOption('de');

    await expect(page.getByRole('heading', { name: 'Konto' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Passwort ändern' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Passwort aktualisieren' })).toBeVisible();
    await captureForPR(page, 'settings-page-german');
  });

  test('the stats page translates previously English-only tab labels into German', async ({ page }) => {
    await page.goto('/stats');
    await page.locator('#lang-select').selectOption('de');

    await expect(page.locator('#tab-components')).toHaveText('Bestandteile');
    await expect(page.locator('#tab-mnemonics')).toHaveText('Eselsbrücken');
    await captureForPR(page, 'stats-page-german');
  });
});
