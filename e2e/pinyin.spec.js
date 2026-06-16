// @ts-check
import { test, expect } from '@playwright/test';

test.use({ storageState: 'e2e/.auth/user.json' });

test.describe('Pinyin listening page', () => {
  test('renders the page with the stats bar', async ({ page }) => {
    await page.goto('/pinyin');
    // The stats bar (due / total) is part of the page chrome and always present.
    await expect(page.locator('#stats-due')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#stats-total')).toBeVisible();
    // With no pinyin sounds seeded, the empty state is shown rather than a card.
    await expect(page.locator('#empty-state')).toBeVisible({ timeout: 10_000 });
  });
});
