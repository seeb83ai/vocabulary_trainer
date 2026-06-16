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
});
