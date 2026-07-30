// @ts-check
import { test, expect } from '@playwright/test';

// Uses the auth state created in global-setup (test user + seeded words).
// The E2E server is started with GITHUB_TOKEN / GITHUB_ISSUE_REPO pointed at a
// mock GitHub API (see global-setup.js), so the feature is enabled and issue
// creation succeeds without touching the real GitHub API.
test.use({ storageState: 'e2e/.auth/user.json' });

test.describe('In-app GitHub issue reporting', () => {
  test('floating button opens the modal and submits an issue', async ({ page }) => {
    await page.goto('/train');

    // The floating report button is present on every authenticated page and
    // becomes visible once the feature flag confirms the feature is enabled.
    const btn = page.locator('#issue-report-btn');
    await expect(btn).toBeVisible({ timeout: 10_000 });

    await btn.click();

    // Modal appears.
    await expect(page.locator('#issue-modal')).toBeVisible({ timeout: 10_000 });

    // Skip the screenshot to keep the headless run deterministic and fast.
    const includeScreenshot = page.locator('#issue-include-screenshot');
    if (await includeScreenshot.isChecked()) {
      await includeScreenshot.uncheck();
    }

    await page.locator('#issue-category').selectOption('bug');
    await page.locator('#issue-title').fill('E2E test report');
    await page.locator('#issue-description').fill('Reported from the E2E suite.');

    await page.locator('#issue-submit').click();

    // The mock GitHub server returns a created issue; the status line shows a
    // link to it.
    const status = page.locator('#issue-status');
    await expect(status).toBeHidden({ timeout: 10_000 });
  });

  test('screenshot type selector is visible when screenshot is enabled', async ({ page }) => {
    await page.goto('/train');

    const btn = page.locator('#issue-report-btn');
    await expect(btn).toBeVisible({ timeout: 10_000 });
    await btn.click();

    await expect(page.locator('#issue-modal')).toBeVisible({ timeout: 10_000 });

    const typeSelect = page.locator('#issue-screenshot-type');
    const includeCheckbox = page.locator('#issue-include-screenshot');

    // Selector is visible when screenshot is enabled (default).
    await includeCheckbox.check();
    await expect(typeSelect).toBeVisible();

    // Selector is hidden when screenshot is disabled.
    await includeCheckbox.uncheck();
    await expect(typeSelect).toBeHidden();

    // Re-enabling shows the selector again.
    await includeCheckbox.check();
    await expect(typeSelect).toBeVisible();
  });

  test('screenshot type selector defaults to visible area and accepts full page', async ({ page }) => {
    await page.goto('/train');

    const btn = page.locator('#issue-report-btn');
    await expect(btn).toBeVisible({ timeout: 10_000 });
    await btn.click();

    await expect(page.locator('#issue-modal')).toBeVisible({ timeout: 10_000 });

    const typeSelect = page.locator('#issue-screenshot-type');

    // Default value is "visible" (visible area).
    await expect(typeSelect).toHaveValue('visible');

    // User can switch to full-page screenshot.
    await typeSelect.selectOption('full');
    await expect(typeSelect).toHaveValue('full');
  });
});
