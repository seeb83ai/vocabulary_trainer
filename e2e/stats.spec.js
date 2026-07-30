// @ts-check
import { test, expect } from '@playwright/test';

test.use({ storageState: 'e2e/.auth/user.json' });

test.describe('Stats page', () => {
  test('renders the stats tabs and the words panel', async ({ page }) => {
    await page.goto('/stats');
    // Tab navigation is present.
    await expect(page.locator('#tab-words')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#tab-pinyin')).toBeVisible();
    await expect(page.locator('#tab-mnemonics')).toBeVisible();
    // The default words panel is shown.
    await expect(page.locator('#panel-words')).toBeVisible();
  });

  test('shows a components by due date chart on the components tab', async ({ page }) => {
    await page.goto('/stats');
    await page.locator('#tab-components').click();
    await expect(page.locator('#panel-components')).toBeVisible();
    await expect(page.locator('#panel-components h2', { hasText: 'Components by Due Date' })).toBeVisible();
    await expect(page.locator('#comp-due-date-chart')).toBeAttached();
  });
});
