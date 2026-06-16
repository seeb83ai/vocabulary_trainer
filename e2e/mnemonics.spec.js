// @ts-check
import { test, expect } from '@playwright/test';

test.use({ storageState: 'e2e/.auth/user.json' });

test.describe('Mnemonics (Hanzi Movie Method) page', () => {
  test('renders the actor/location/room library structure', async ({ page }) => {
    await page.goto('/mnemonics');
    // The library section containers are rendered (they may be empty until the
    // user configures their library, so assert presence rather than visibility).
    await expect(page.locator('#actors-container')).toBeAttached({ timeout: 10_000 });
    await expect(page.locator('#locations-container')).toBeAttached();
    await expect(page.locator('#tonerooms-container')).toBeAttached();
    await expect(page).toHaveURL(/\/mnemonics/);
  });
});
