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

test.describe('Mnemonics – add prop after a language change', () => {
  test('one click on Add saves the prop once, also after language changes', async ({ page }) => {
    // The e2e DB has no HMM library, so the library reads are mocked and the
    // prop PUTs are counted instead of saved.
    for (const name of ['actors', 'locations', 'tone-rooms']) {
      await page.route(`**/api/hmm/${name}`, r => r.fulfill({ json: [] }));
    }
    const puts = [];
    await page.route('**/api/hmm/props', r => {
      if (r.request().method() === 'PUT') {
        puts.push(r.request().postDataJSON());
        return r.fulfill({ json: {} });
      }
      return r.fulfill({ json: [] });
    });

    await page.goto('/mnemonics');
    await expect(page.locator('#add-prop-btn')).toBeAttached({ timeout: 10_000 });
    await page.locator('#mn-tab-props').click();

    // app.js sends this event when the UI language changes.
    for (let i = 0; i < 2; i++) {
      await page.evaluate(() => document.dispatchEvent(new Event('langchange')));
    }
    // Let the re-renders finish before the click.
    await page.waitForLoadState('networkidle');

    await page.locator('#new-prop-radical').fill('木');
    await page.locator('#new-prop-name').fill('wooden spoon');
    await page.locator('#add-prop-btn').click();

    await expect.poll(() => puts.length).toBeGreaterThan(0);
    await page.waitForLoadState('networkidle');
    expect(puts).toEqual([{ radical: '木', prop_name: 'wooden spoon' }]);
  });
});
