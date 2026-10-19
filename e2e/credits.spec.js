// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// The credits page names every data source, font and library with its license.
// CC BY-SA data (CC-CEDICT, HanDeDict, FrequencyWords) needs this attribution,
// so a visitor must reach the page without an account.
test.describe('Credits and licenses (signed out)', () => {
  test('the landing footer links to the credits page', async ({ page }) => {
    await page.goto('/');
    const link = page.locator('footer a[href="/credits"]');
    await expect(link).toBeVisible();
    await link.scrollIntoViewIfNeeded();
    await captureForPR(page, 'credits-landing-footer');

    await link.click();
    await expect(page).toHaveURL(/\/credits$/);
    await expect(page.locator('#credits-title')).toHaveText('Credits and licenses');
  });

  test('the credits page lists the sources with their licenses', async ({ page }) => {
    await page.goto('/credits');

    const cedict = page.locator('[data-credit="cc-cedict"]');
    await expect(cedict).toContainText('CC-CEDICT');
    await expect(cedict).toContainText('CC BY-SA 4.0');
    await expect(page.locator('[data-credit="handedict"]')).toContainText('CC BY-SA 2.0 DE');
    await expect(page.locator('[data-credit="frequencywords"]')).toContainText('CC BY-SA 4.0');
    await expect(page.locator('[data-credit="makemeahanzi"]')).toContainText('LGPL-3.0');
    await expect(page.locator('[data-credit="chartjs"]')).toContainText('MIT');
    await expect(page.locator('[data-credit="noto-sans-sc"]')).toContainText('OFL-1.1');
    await captureForPR(page, 'credits-page');
  });

  test('a license link opens the full license text', async ({ page }) => {
    await page.goto('/credits');
    await page.locator('[data-credit="chartjs"] a.license-link').click();

    await expect(page).toHaveURL(/\/licenses\/chartjs-mit\.txt$/);
    await expect(page.locator('body')).toContainText('Chart.js Contributors');
    await expect(page.locator('body')).toContainText('Permission is hereby granted');
  });

  test('the credits page follows the chosen UI language', async ({ page }) => {
    await page.goto('/credits');
    await page.locator('#lang-select').selectOption('de');
    await expect(page.locator('#credits-title')).toHaveText('Quellen und Lizenzen');
    await captureForPR(page, 'credits-page-german');

    await page.locator('#lang-select').selectOption('zh');
    await expect(page.locator('#credits-title')).toHaveText('致谢与许可');
    await page.locator('#lang-select').selectOption('en');
  });
});

test.describe('Credits and licenses (signed in)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('Settings links to the credits page and the Impressum', async ({ page }) => {
    await page.goto('/settings');
    const credits = page.locator('#settings-legal a[href="/credits"]');
    await expect(credits).toBeVisible();
    await expect(page.locator('#settings-legal a[href="/impressum"]')).toBeVisible();
    await credits.scrollIntoViewIfNeeded();
    await captureForPR(page, 'credits-settings-link');

    await credits.click();
    await expect(page).toHaveURL(/\/credits$/);
    await expect(page.locator('#credits-title')).toBeVisible();
  });
});
