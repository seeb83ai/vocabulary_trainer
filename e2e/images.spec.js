// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// The e2e server runs with UNSPLASH_ACCESS_KEY unset (see global-setup.js),
// so /api/config reports images_configured: false. This spec verifies the
// settings checkbox exists and persists, and that no image element/error
// appears on the train page while the feature is unconfigured. Deeper "does
// an image actually render" coverage lives in the Go handler tests
// (service/handlers/images_test.go), which mock the Unsplash HTTP client.
test.describe('Show images with Chinese text setting', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('checkbox exists, saves, and persists across reload', async ({ page }) => {
    await page.goto('/settings');
    const toggle = page.locator('#show-images');
    await expect(toggle).toBeVisible();
    await captureForPR(page, 'settings-show-images-off');

    await toggle.check();
    await expect(page.locator('#mode-success')).toBeVisible();
    await captureForPR(page, 'settings-show-images-on');

    await page.reload();
    await expect(page.locator('#show-images')).toBeChecked();

    const res = await page.request.get('/api/settings');
    const settings = await res.json();
    expect(settings.show_images_with_chinese_text).toBe(true);

    // Reset to default for other specs.
    await toggle.uncheck();
    await expect(page.locator('#mode-success')).toBeVisible();
  });

  test('/api/config reports images as unconfigured in the e2e environment', async ({ page }) => {
    const res = await page.request.get('/api/config');
    const cfg = await res.json();
    expect(cfg.images_configured).toBe(false);
  });

  test('no card image or error appears on /train when the setting is on but the feature is unconfigured', async ({ page }) => {
    // Turn the setting on (server-side, so it applies without racing autosave).
    const before = await page.request.get('/api/settings');
    const beforeSettings = await before.json();
    await page.request.patch('/api/settings', {
      data: { ...beforeSettings, show_images_with_chinese_text: true },
    });

    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await captureForPR(page, 'train-card-images-unconfigured');

    // images_configured is false server-side, so the image element must stay hidden.
    await expect(page.locator('#card-image')).toBeHidden();

    // Reset to default for other specs.
    await page.request.patch('/api/settings', {
      data: { ...beforeSettings, show_images_with_chinese_text: false },
    });
  });
});
