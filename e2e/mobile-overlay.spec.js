// @ts-check
import { test, expect } from '@playwright/test';

test.use({ storageState: 'e2e/.auth/user.json' });

// The training page exposes its filters through a slide-in overlay on small
// (mobile) viewports, opened via the filter button.
test.describe('Training filter overlay (mobile)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('opens and closes the filter overlay', async ({ page }) => {
    await page.goto('/train');

    const openBtn = page.locator('#open-filter-overlay');
    await expect(openBtn).toBeVisible({ timeout: 10_000 });

    await openBtn.click();
    await expect(page.locator('#filter-overlay')).toBeVisible();
    // The overlay close control is visible while the panel is open.
    await expect(page.locator('#filter-overlay-close')).toBeVisible();

    await page.locator('#filter-overlay-close').click();
    await expect(page.locator('#filter-overlay')).toBeHidden();
  });

  test('tag filter bar is hidden on mobile but reachable via the overlay', async ({ page }) => {
    // Seed a tagged word so loadTrainTags() has at least one tag to render.
    await page.goto('/train');
    await page.request.post('/api/words', {
      data: {
        zh_text: '标签词', pinyin: 'biāo qiān cí', translations: { en: ['tagged word'] },
        tags: ['mobiletagtest'], start_training: true,
      },
    });

    await page.goto('/train');
    await expect(page.locator('#open-filter-overlay')).toBeVisible({ timeout: 10_000 });

    // Wait for loadTrainTags() to finish populating the overlay before asserting
    // on the desktop bar, so the check isn't racing the async tag fetch.
    await page.locator('#open-filter-overlay').click();
    await expect(page.locator('#filter-overlay')).toBeVisible();
    await expect(page.locator('#overlay-tag-chips')).toContainText('mobiletagtest');
    await page.locator('#filter-overlay-close').click();

    // The desktop tag filter bar must never be visible on a mobile viewport,
    // even once it has tags to show — it's reachable through the overlay instead.
    await expect(page.locator('#tag-filter-bar')).toBeHidden();
  });
});
