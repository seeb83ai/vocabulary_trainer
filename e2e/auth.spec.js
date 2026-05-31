// @ts-check
import { test, expect } from '@playwright/test';

const TEST_EMAIL = 'e2e@test.local';
const TEST_PASSWORD = 'E2eTestPassword123!';

test.describe('Authentication', () => {
  test('login page renders with Sign In and Create Account tabs', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('#tab-signin')).toBeVisible();
    await expect(page.locator('#tab-register')).toBeVisible();
    await expect(page.locator('#signin-form')).toBeVisible();
    await expect(page.locator('#panel-register')).toBeHidden();
  });

  test('registration via browser auto-logs in and redirects to /train', async ({ page }) => {
    // Mock the HIBP (Have I Been Pwned) API so it doesn't block registration
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' }); // empty = not pwned
    });

    const uniqueEmail = `e2e-reg-${Date.now()}@test.local`;

    await page.goto('/');
    await page.locator('#tab-register').click();
    await expect(page.locator('#panel-register')).toBeVisible();

    await page.locator('#reg-email').fill(uniqueEmail);
    await page.locator('#reg-password').fill(TEST_PASSWORD);
    await page.locator('#reg-confirm').fill(TEST_PASSWORD);
    await page.locator('#register-btn').click();

    // No SMTP configured → auto-verified → redirected to /train
    await expect(page).toHaveURL('/train', { timeout: 10_000 });
  });

  test('wrong password shows error message', async ({ page }) => {
    await page.goto('/');
    await page.locator('#signin-email').fill(TEST_EMAIL);
    await page.locator('#signin-password').fill('WrongPassword999!');
    await page.locator('#signin-btn').click();

    await expect(page.locator('#signin-error')).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('#signin-error')).not.toBeEmpty();
  });

  test('unauthenticated access to /train redirects to /', async ({ browser }) => {
    // Use a fresh context with no cookies
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await page.goto('/train');
    await expect(page).toHaveURL('/', { timeout: 5_000 });
    await ctx.close();
  });
});
