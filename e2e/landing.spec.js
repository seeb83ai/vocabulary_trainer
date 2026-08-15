// @ts-check
import { test, expect } from '@playwright/test';

test.describe('Landing page', () => {
  test('signed-out visit shows the value proposition', async ({ page }) => {
    await page.goto('/');

    await expect(page).toHaveTitle(/Chinese/i);
    await expect(page.locator('#hero-title')).toBeVisible();
    await expect(page.locator('#hero-title')).toContainText(/Chinese/i);
    await expect(page.locator('#hero-features li')).toHaveCount(3);

    // Sign-in card still present next to the hero
    await expect(page.locator('#signin-form')).toBeVisible();
  });

  test('page has SEO and Open Graph meta tags', async ({ page }) => {
    await page.goto('/');

    await expect(page.locator('meta[name="description"]')).toHaveAttribute('content', /spaced repetition/i);
    await expect(page.locator('meta[property="og:title"]')).toHaveAttribute('content', /.+/);
    await expect(page.locator('meta[property="og:description"]')).toHaveAttribute('content', /.+/);
    await expect(page.locator('meta[property="og:type"]')).toHaveAttribute('content', 'website');
  });

  test('hero CTA opens the Create Account tab', async ({ page }) => {
    await page.goto('/');

    await page.locator('#hero-cta').click();
    await expect(page.locator('#panel-register')).toBeVisible();
    await expect(page.locator('#panel-signin')).toBeHidden();
  });
});
