// @ts-check
import { test, expect } from '@playwright/test';
import { BASE_URL, ADMIN_EMAIL, ADMIN_PASSWORD } from './global-setup.js';
import { captureForPR } from './helpers/screenshot.js';
import { openAuthModal } from './helpers/auth.js';

// PATCH /api/settings replaces the whole settings row, so swapping just the
// language pair means round-tripping the current settings with those two
// fields overridden (mirrors the pattern in gamification.spec.js).
async function setLangs(page, primaryLang, secondaryLang) {
  const current = await (await page.request.get(`${BASE_URL}/api/settings`)).json();
  const res = await page.request.patch(`${BASE_URL}/api/settings`, {
    data: { ...current, primary_lang: primaryLang, secondary_lang: secondaryLang },
  });
  return res;
}

// Regression test for issue #342: "German and English text are in wrong
// place after auto translate". The bug reproduced when the signed-in user's
// primary language was German (secondary English) — Auto-translate always
// requested target_lang=EN into the primary-language field and target_lang=DE
// into the secondary-language field, regardless of which language was
// actually configured as primary/secondary, so the EN and DE results landed
// under the wrong label.
//
// Uses the admin account (role=admin, so Auto-translate is available) and a
// mock DeepL server (wired up by global-setup.js via DEEPL_API_BASE_URL) so
// the test doesn't depend on a real DeepL key.

test.describe('Vocabulary — Auto-translate EN/DE fields', () => {
  test.afterEach(async ({ page }) => {
    // Restore the default language pair so later specs sharing this admin
    // account aren't affected by the swapped primary/secondary config below.
    await setLangs(page, 'en', 'de');
  });

  test('places EN and DE translations in the correct fields when primary language is German (issue #342)', async ({ page }) => {
    // Log in as admin (role=admin → Auto-translate is available).
    await page.goto('/');
    await openAuthModal(page);
    await page.locator('#signin-email').fill(ADMIN_EMAIL);
    await page.locator('#signin-password').fill(ADMIN_PASSWORD);
    await page.locator('#signin-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    // Reproduce the reported setup: primary language German, secondary English
    // (matches the issue screenshot, which shows "German Translation(s)" above
    // "English Translation(s)").
    const patchRes = await setLangs(page, 'de', 'en');
    expect(patchRes.ok()).toBeTruthy();

    await page.goto('/vocab');

    // Wait for the language-aware labels/inputs to load.
    await expect(page.locator('#primary-lang-label')).toHaveText('German Translation(s)', { timeout: 8_000 });
    await expect(page.locator('#secondary-lang-label')).toHaveText('English Translation(s)', { timeout: 8_000 });

    await page.locator('#form-zh').fill('工具');
    await expect(page.locator('#translate-btn')).toBeVisible({ timeout: 8_000 });

    await captureForPR(page, 'vocab-auto-translate-before');

    await page.locator('#translate-btn').click();

    // en-inputs-container sits under the "German Translation(s)" label here —
    // it must receive the German result, not the English one.
    const germanField = page.locator('#en-inputs-container .en-input').first();
    await expect(germanField).toHaveValue('Werkzeuge', { timeout: 8_000 });

    // de-inputs-container sits under the "English Translation(s)" label —
    // it must receive the English result(s), not the German one.
    const englishField = page.locator('#de-inputs-container .de-input').first();
    await expect(englishField).toHaveValue('Tools', { timeout: 8_000 });

    await captureForPR(page, 'vocab-auto-translate-fixed');
  });
});
