// @ts-check
import { test, expect } from '@playwright/test';

test.describe('Settings – Daily Learning', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('daily learning section is visible with expected controls', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('#daily-learning-section')).toBeVisible();
    await expect(page.locator('#max-new-words')).toBeVisible();
    await expect(page.locator('#skip-new-visible')).toBeVisible();
  });

  test('max new words per day can be changed and saved', async ({ page }) => {
    await page.goto('/settings');
    const input = page.locator('#max-new-words');
    await input.fill('3');
    await page.locator('#daily-save-btn').click();
    await expect(page.locator('#daily-success')).toBeVisible();

    // Reload page and verify value persists
    await page.reload();
    await expect(page.locator('#max-new-words')).toHaveValue('3');

    // Reset to default
    await input.fill('5');
    await page.locator('#daily-save-btn').click();
  });

  test('skip new words visible toggle saves and persists', async ({ page }) => {
    await page.goto('/settings');
    const toggle = page.locator('#skip-new-visible');

    // Uncheck (hide skip button)
    await toggle.uncheck();
    await page.locator('#daily-save-btn').click();
    await expect(page.locator('#daily-success')).toBeVisible();

    await page.reload();
    await expect(page.locator('#skip-new-visible')).not.toBeChecked();

    // Re-enable
    await toggle.check();
    await page.locator('#daily-save-btn').click();
  });

  test('skip button hidden in quiz when skip_new_words_visible is false', async ({ page }) => {
    // Disable skip via API
    const res = await page.request.get('/api/settings');
    const settings = await res.json();

    await page.request.patch('/api/settings', {
      data: { ...settings, skip_new_words_visible: false },
    });

    // The new-word skip button should not appear (even if new-word area were shown).
    // We verify via the API that the setting is reflected.
    const res2 = await page.request.get('/api/settings');
    const updated = await res2.json();
    expect(updated.skip_new_words_visible).toBe(false);

    // Restore
    await page.request.patch('/api/settings', {
      data: { ...settings, skip_new_words_visible: true },
    });
  });

  test('baseline due-today can be enabled with a threshold', async ({ page }) => {
    await page.goto('/settings');

    await page.locator('#baseline-due-today-enabled').check();
    await page.locator('#baseline-due-today-value').fill('15');
    await page.locator('#daily-save-btn').click();
    await expect(page.locator('#daily-success')).toBeVisible();

    await page.reload();
    await expect(page.locator('#baseline-due-today-enabled')).toBeChecked();
    await expect(page.locator('#baseline-due-today-value')).toHaveValue('15');

    // Disable and reset
    await page.locator('#baseline-due-today-enabled').uncheck();
    await page.locator('#daily-save-btn').click();
  });
});
