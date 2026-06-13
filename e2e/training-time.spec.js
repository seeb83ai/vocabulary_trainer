// @ts-check
import { test, expect } from '@playwright/test';

test.describe('Training time tracking', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('accumulates training_seconds after visiting /train', async ({ page }) => {
    await page.goto('/train');
    // The main test user always has seeded acknowledged words, so #card-area appears
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });

    // Wait 1.5 s so at least 1 second of active time is recorded
    await page.waitForTimeout(1_500);

    // Navigate away — triggers beforeunload + sendBeacon flush
    await page.goto('/stats');

    // Give the beacon a moment to be processed by the server
    await page.waitForTimeout(500);

    const res = await page.request.get('/api/quiz/daily-stats');
    expect(res.ok()).toBe(true);
    const data = await res.json();

    const today = data.days?.find(d => d.training_seconds > 0);
    expect(today, 'expected a day entry with training_seconds > 0').toBeTruthy();
    expect(today.training_seconds).toBeGreaterThanOrEqual(1);
  });
});
