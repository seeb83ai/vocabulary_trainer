// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// These tests use the auth state created in global-setup (test user + seeded words)
test.use({ storageState: 'e2e/.auth/user.json' });

// The UI language is chosen in Settings → Languages → App language and stored
// on the server. The e2e user is shared, so every test switches back.
async function useGerman(page) {
  const res = await page.request.put('/api/settings/ui-lang', { data: { ui_lang: 'de' } });
  expect(res.ok()).toBe(true);
}

test.afterEach(async ({ page }) => {
  await page.request.put('/api/settings/ui-lang', { data: { ui_lang: 'en' } });
});

test.describe('German UI language', () => {
  test('the app language setting offers German and switches the nav to German', async ({ page }) => {
    await page.goto('/settings');
    await captureForPR(page, 'lang-select-before');

    await expect(page.locator('#ui-lang-de')).toHaveCount(1);
    await page.locator('#ui-lang-de').click();

    await expect(page.locator('#app-sidebar a[href="/vocab"]')).toHaveText('Vokabeln');
    await expect(page.locator('#app-sidebar a[href="/stats"]')).toHaveText('Statistik');
    await page.goto('/train');
    await expect(page.locator('#app-sidebar a[href="/vocab"]')).toHaveText('Vokabeln');
    await captureForPR(page, 'train-page-german');
  });

  test('the settings page translates previously English-only text into German', async ({ page }) => {
    await useGerman(page);
    await page.goto('/settings');

    await expect(page.getByRole('heading', { name: 'Konto' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Passwort ändern' })).toBeVisible();
    await captureForPR(page, 'settings-page-german');
  });

  test('the stats page translates previously English-only tab labels into German', async ({ page }) => {
    await useGerman(page);
    await page.goto('/stats');

    await expect(page.locator('#tab-components')).toHaveText('Bestandteile');
    await expect(page.locator('#tab-mnemonics')).toHaveText('Eselsbrücken');
    await captureForPR(page, 'stats-page-german');
  });
});
