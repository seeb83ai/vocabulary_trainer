// @ts-check
import { test, expect } from '@playwright/test';
import { ADMIN_EMAIL, ADMIN_PASSWORD, TEST_EMAIL, TEST_PASSWORD } from './global-setup.js';
import { openAuthModal } from './helpers/auth.js';

test.describe('Admin dashboard', () => {
  test('admin user sees usage insights', async ({ page }) => {
    await page.goto('/');
    await openAuthModal(page);
    await page.locator('#signin-email').fill(ADMIN_EMAIL);
    await page.locator('#signin-password').fill(ADMIN_PASSWORD);
    await page.locator('#signin-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    await page.goto('/admin-dashboard');
    await expect(page).toHaveURL('/admin-dashboard');
    await expect(page.locator('#stat-tiles')).toContainText('Total users');
    await expect(page.locator('#load-error')).toBeHidden();
  });

  test('non-admin user is redirected away from the dashboard', async ({ page }) => {
    await page.goto('/');
    await openAuthModal(page);
    await page.locator('#signin-email').fill(TEST_EMAIL);
    await page.locator('#signin-password').fill(TEST_PASSWORD);
    await page.locator('#signin-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    // RequireAdmin redirects to "/"; the login page then auto-forwards an
    // already-authenticated visitor on to /train.
    await page.goto('/admin-dashboard');
    await expect(page).toHaveURL('/train', { timeout: 5_000 });
  });

  test('unauthenticated visitor is redirected to login', async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await page.goto('/admin-dashboard');
    await expect(page).toHaveURL('/', { timeout: 5_000 });
    await ctx.close();
  });
});
