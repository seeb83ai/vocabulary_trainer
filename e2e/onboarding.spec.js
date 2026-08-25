// @ts-check
import { test, expect } from '@playwright/test';

const PASSWORD = 'E2eOnboardingPass123!';

/** Register a brand-new user in the browser and land on /train. */
async function registerFreshUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => {
    route.fulfill({ status: 200, body: '' });
  });
  const email = `e2e-onboard-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
}

test.describe('One-button onboarding', () => {
  test('new user sees the quick-start level chooser and reaches the first card in one click', async ({ page }) => {
    await registerFreshUser(page);

    await expect(page.locator('#empty-state')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#ob-quickstart')).toBeVisible();
    // The detailed tag picker stays hidden behind the "choose myself" option.
    await expect(page.locator('#ob-step1')).toBeHidden();

    await page.locator('#ob-qs-hsk1').click();

    // Import runs, then the first card appears — for a fresh user that is
    // the new-word introduction screen.
    await expect(page.locator('#new-word-area')).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('#new-word-zh')).not.toBeEmpty();
    await expect(page.locator('#empty-state')).toBeHidden();
  });

  test('the custom option reveals the existing tag picker', async ({ page }) => {
    await registerFreshUser(page);

    await expect(page.locator('#ob-quickstart')).toBeVisible({ timeout: 10_000 });
    await page.locator('#ob-qs-custom').click();

    await expect(page.locator('#ob-step1')).toBeVisible();
    await expect(page.locator('#ob-quickstart')).toBeHidden();
  });
});
