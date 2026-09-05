// @ts-check
import { test, expect } from '@playwright/test';
import { openAuthModal } from './helpers/auth.js';

test.describe('Landing page', () => {
  test('signed-out visit shows the value proposition', async ({ page }) => {
    await page.goto('/');

    await expect(page).toHaveTitle(/Chinese/i);
    await expect(page.locator('#hero-title')).toBeVisible();
    await expect(page.locator('#hero-title')).toContainText(/Chinese/i);
    await expect(page.locator('#hero-features li')).toHaveCount(3);

    // Sign-in/create-account lives behind a modal now, opened from CTA
    // buttons — it should be present but not shown until a visitor asks for it.
    await expect(page.locator('#signin-form')).toBeHidden();
    await expect(page.locator('#btn-signup')).toBeVisible();
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

  test('small screens stack hero copy above the demo card, with no horizontal overflow', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/');

    const heroBox = await page.locator('#hero-title').boundingBox();
    const demoBox = await page.locator('#demo-section-wrap').boundingBox();
    expect(heroBox.y).toBeLessThan(demoBox.y);

    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
    expect(scrollWidth).toBeLessThanOrEqual(375);
  });

  test('auth modal is centered, not pinned to the bottom, on small screens', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/');
    await openAuthModal(page);

    const panel = await page.locator('#auth-modal-panel').boundingBox();
    const gapBelow = 812 - (panel.y + panel.height);
    // A bottom sheet would have gapBelow near 0; a centered modal leaves a
    // real margin on both sides.
    expect(gapBelow).toBeGreaterThan(10);
  });
});
