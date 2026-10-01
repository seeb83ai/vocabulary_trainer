// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// The app shell (issue: redesign handoff): a sidebar at >= 880px, a bottom
// tab bar plus a "More" sheet below. The UI language lives in Settings only.
test.use({ storageState: 'e2e/.auth/user.json' });

// The e2e user is shared by every spec — always put the UI language back.
test.afterEach(async ({ page }) => {
  await page.request.put('/api/settings/ui-lang', { data: { ui_lang: 'en' } });
});

test.describe('App shell – desktop sidebar', () => {
  test.use({ viewport: { width: 1280, height: 860 } });

  test('shows grouped navigation, report and sign out, and no language select', async ({ page }) => {
    await page.goto('/train');

    const sidebar = page.locator('#app-sidebar');
    await expect(sidebar).toBeVisible();
    const nav = sidebar.locator('nav');
    for (const href of ['/train', '/pinyin', '/vocab', '/mnemonics', '/mismatches', '/stats', '/settings']) {
      await expect(nav.locator(`a[href="${href}"]`)).toBeVisible();
    }
    await expect(nav.locator('a[href="/train"]')).toHaveAttribute('aria-current', 'page');
    await expect(sidebar.locator('#nav-report')).toBeVisible({ timeout: 10_000 });
    await expect(sidebar.locator('#logout-btn')).toBeVisible();

    await expect(page.locator('#lang-select')).toHaveCount(0);
    await expect(page.locator('#app-tabbar')).toBeHidden();
    await captureForPR(page, 'shell-desktop-sidebar');
  });

  test('the sidebar report entry opens the report dialog', async ({ page }) => {
    await page.goto('/stats');
    await page.locator('#nav-report').click();
    await expect(page.locator('#issue-modal')).toBeVisible();
    await expect(page.locator('#issue-dialog')).toBeVisible();
    await captureForPR(page, 'report-issue-dialog');
  });
});

test.describe('App shell – phone tab bar', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('shows five tabs and the More sheet', async ({ page }) => {
    await page.goto('/settings');

    await expect(page.locator('#app-sidebar')).toBeHidden();
    const tabbar = page.locator('#app-tabbar');
    await expect(tabbar).toBeVisible();
    await expect(tabbar.locator('a[href="/train"]')).toBeVisible();
    await expect(tabbar.locator('a[href="/vocab"]')).toBeVisible();
    await expect(tabbar.locator('a[href="/stats"]')).toBeVisible();
    await expect(tabbar.locator('#tab-report')).toBeVisible({ timeout: 10_000 });
    // Settings lives in the More sheet, so the More tab is the active one.
    await expect(tabbar.locator('#tab-more')).toHaveAttribute('aria-current', 'page');
    await expect(page.locator('#app-topbar')).toContainText('Settings');

    await tabbar.locator('#tab-more').click();
    const sheet = page.locator('#more-sheet');
    await expect(sheet).toBeVisible();
    for (const href of ['/pinyin', '/mnemonics', '/mismatches', '/settings']) {
      await expect(sheet.locator(`a[href="${href}"]`)).toBeVisible();
    }
    await expect(sheet.locator('#more-logout-btn')).toBeVisible();
    await captureForPR(page, 'shell-phone-more-sheet');

    await page.locator('#more-sheet-backdrop').click({ position: { x: 10, y: 10 } });
    await expect(sheet).toBeHidden();
  });

  test('the report tab opens the report sheet', async ({ page }) => {
    await page.goto('/train');
    await page.locator('#tab-report').click();
    await expect(page.locator('#issue-dialog')).toBeVisible();
    await captureForPR(page, 'report-issue-sheet-phone');
  });

  test('tab bar stays in viewport after scrolling content', async ({ page }) => {
    await page.goto('/vocab');
    await page.locator('.app-content').evaluate(el => { el.scrollTop = el.scrollHeight; });
    await expect(page.locator('#app-tabbar')).toBeInViewport();
  });
});

test.describe('App shell – report tab when reporting is off', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('shows four tabs and no report entry', async ({ page }) => {
    await page.route('**/api/github/config', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: '{"enabled":false}' }));
    await page.goto('/train');
    await expect(page.locator('#app-tabbar a[href="/vocab"]')).toBeVisible();
    await expect(page.locator('#tab-report')).toBeHidden();
    await expect(page.locator('#nav-report')).toBeHidden();
  });
});

test.describe('App language in Settings', () => {
  test.use({ viewport: { width: 1280, height: 860 } });

  test('switching to Deutsch translates the app and is stored on the server', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('body')).toHaveAttribute('data-settings-loaded', 'true', { timeout: 10_000 });

    await page.locator('#ui-lang-de').click();
    await expect(page.locator('#ui-lang-de')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#app-sidebar a[href="/vocab"]')).toHaveText('Vokabeln');
    await captureForPR(page, 'settings-app-language-de');

    await expect.poll(async () => {
      const res = await page.request.get('/api/settings');
      return (await res.json()).ui_lang;
    }).toBe('de');

    // A new device (empty localStorage) picks up the stored language.
    await page.evaluate(() => localStorage.removeItem('uiLang'));
    await page.goto('/stats');
    await expect(page.locator('#app-sidebar a[href="/stats"]')).toHaveText('Statistik');
  });
});
