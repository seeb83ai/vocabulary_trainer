// @ts-check
import { test, expect } from '@playwright/test';

// Uses the auth state created in global-setup (test user + seeded words).
// The E2E server is started with GITHUB_TOKEN / GITHUB_ISSUE_REPO pointed at a
// mock GitHub API (see global-setup.js), so the feature is enabled and issue
// creation succeeds without touching the real GitHub API.
test.use({ storageState: 'e2e/.auth/user.json' });

test.describe('In-app GitHub issue reporting', () => {
  test('the sidebar entry opens the report dialog and submits an issue', async ({ page }) => {
    await page.goto('/train');

    // The sidebar "Report an issue" entry is present on every authenticated
    // page and becomes visible once the feature flag confirms it is enabled.
    const btn = page.locator('#nav-report');
    await expect(btn).toBeVisible({ timeout: 10_000 });

    await btn.click();

    // Modal appears.
    await expect(page.locator('#issue-modal')).toBeVisible({ timeout: 10_000 });

    // Skip the screenshot to keep the headless run deterministic and fast.
    const includeScreenshot = page.locator('#issue-include-screenshot');
    if (await includeScreenshot.isChecked()) {
      await includeScreenshot.uncheck();
    }

    await page.locator('[data-issue-cat="idea"]').click();
    await expect(page.locator('[data-issue-cat="idea"]')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#issue-category')).toHaveValue('idea');
    await page.locator('#issue-title').fill('E2E test report');
    await page.locator('#issue-description').fill('Reported from the E2E suite.');

    await page.locator('#issue-submit').click();

    // The mock GitHub server returns a created issue; the dialog switches to
    // the success view with a link to it.
    await expect(page.locator('#issue-success')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#issue-success-num')).toHaveText(/^#\d+$/);
    await expect(page.locator('#issue-status')).toBeHidden();

    await page.locator('#issue-done').click();
    await expect(page.locator('#issue-modal')).toBeHidden();
  });

  test('empty title and description show inline errors and keep the dialog open', async ({ page }) => {
    await page.goto('/train');
    await page.locator('#nav-report').click();
    await expect(page.locator('#issue-dialog')).toBeVisible({ timeout: 10_000 });

    await page.locator('#issue-submit').click();
    await expect(page.locator('#issue-title-err')).toBeVisible();
    await expect(page.locator('#issue-title')).toHaveClass(/rp-invalid/);

    await page.locator('#issue-title').fill('Only a title');
    await expect(page.locator('#issue-title-count')).toHaveText('12/120');
    await page.locator('#issue-submit').click();
    await expect(page.locator('#issue-description-err')).toBeVisible();
    await expect(page.locator('#issue-modal')).toBeVisible();
  });
});
