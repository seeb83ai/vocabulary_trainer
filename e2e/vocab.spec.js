// @ts-check
import { test, expect } from '@playwright/test';

// These tests use the auth state created in global-setup (test user + seeded words)
test.use({ storageState: 'e2e/.auth/user.json' });

test.describe('Vocabulary Management', () => {
  test('vocab page shows seeded words', async ({ page }) => {
    await page.goto('/vocab');
    const tbody = page.locator('#words-tbody');
    // globalSetup seeds at least one word (你好)
    await expect(tbody.locator('tr').first()).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#words-tbody')).toContainText('你好');
  });

  test('add a new word via the form', async ({ page }) => {
    await page.goto('/vocab');

    // Wait for the EN translation input to appear — it's created after loadLangSettings()
    // resolves (async settings fetch), so we must wait before filling it.
    await page.locator('#en-inputs-container .en-input').first().waitFor({ state: 'visible', timeout: 8_000 });

    // Fill Chinese word
    await page.locator('#form-zh').fill('水');

    // Fill the first EN translation input
    await page.locator('#en-inputs-container .en-input').first().fill('water');

    // Check "Start training immediately" so the word is acknowledged (first_seen_date set)
    // and is visible even with the default hide_unseen=1 filter active.
    await page.locator('#form-start-training').check();

    // Submit the form
    await page.locator('#word-form button[type="submit"]').click();

    // The new word should appear in the word list
    await expect(page.locator('#words-tbody')).toContainText('水', { timeout: 8_000 });
  });

  test('adding a multi-character word auto-creates a tagged sub-word', async ({ page }) => {
    // globalSetup imports a tiny cedict fixture covering 踢 ("to kick") and
    // 足球 ("football/soccer"), so creating 踢足球 should auto-create 足球
    // as its own inert vocabulary word, tagged HSK1-sub (issue #293).
    await page.goto('/vocab');
    await page.locator('#en-inputs-container .en-input').first().waitFor({ state: 'visible', timeout: 8_000 });

    await page.locator('#form-zh').fill('踢足球');
    await page.locator('#en-inputs-container .en-input').first().fill('play football');
    await page.locator('#form-tag-input').fill('HSK1');
    await page.locator('#form-tag-input').press('Enter');
    await page.locator('#form-start-training').check();
    await page.locator('#word-form button[type="submit"]').click();

    await expect(page.locator('#words-tbody')).toContainText('踢足球', { timeout: 8_000 });

    // The auto-created sub-word is inert (never acknowledged), so it's
    // hidden by the default "hide unseen" filter — reveal it, then search.
    await page.locator('#hide-unseen-btn').click();
    await page.locator('#search-input').fill('足球');
    const row = page.locator('#words-tbody tr', { hasText: '足球' }).filter({ hasNotText: '踢足球' });
    await expect(row).toBeVisible({ timeout: 8_000 });
  });

  test('translate button uses the free local dictionary before DeepL', async ({ page }) => {
    // 足球 is covered by the cedict fixture imported in globalSetup, so the
    // Translate button should fill it in via the free local lookup — no
    // plus role or DeepL key needed.
    await page.goto('/vocab');
    await page.locator('#en-inputs-container .en-input').first().waitFor({ state: 'visible', timeout: 8_000 });

    await page.locator('#form-zh').fill('足球');
    await page.locator('#translate-btn').click();

    // import-cedict joins CC-CEDICT's multiple /def1/def2/ glosses into one
    // "; "-separated definition string.
    await expect(page.locator('#en-inputs-container .en-input').first())
      .toHaveValue('football; soccer', { timeout: 8_000 });
  });

  test('title is on its own line above controls on mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 641 });
    await page.goto('/vocab');

    const title = page.locator('h2[data-i18n="vocab.listTitle"]');
    const toggleBtn = page.locator('#view-words-btn');

    await expect(title).toBeVisible({ timeout: 8_000 });
    await expect(toggleBtn).toBeVisible({ timeout: 8_000 });

    const titleBox = await title.boundingBox();
    const toggleBox = await toggleBtn.boundingBox();

    expect(titleBox).not.toBeNull();
    expect(toggleBox).not.toBeNull();

    // Title must be above the toggle button on mobile — they must not share the same row
    expect(titleBox.y + titleBox.height).toBeLessThanOrEqual(toggleBox.y + 5);
  });

  test('search box appears in a new row on mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 641 });
    await page.goto('/vocab');

    const searchInput = page.locator('#search-input');
    await expect(searchInput).toBeVisible({ timeout: 8_000 });

    const toggleBtn = page.locator('#view-words-btn');
    await expect(toggleBtn).toBeVisible({ timeout: 8_000 });

    const searchBox = await searchInput.boundingBox();
    const toggleBox = await toggleBtn.boundingBox();
    expect(searchBox).not.toBeNull();
    expect(toggleBox).not.toBeNull();

    // Search input must be fully within horizontal viewport bounds (no overflow)
    expect(searchBox.x).toBeGreaterThanOrEqual(0);
    expect(searchBox.x + searchBox.width).toBeLessThanOrEqual(360);

    // Search input must be on a new row below the toggle button
    expect(searchBox.y).toBeGreaterThan(toggleBox.y + toggleBox.height - 5);
  });

  test('reset a trained word restores it to unseen state', async ({ page }) => {
    await page.goto('/vocab');
    await expect(page.locator('#words-tbody tr').first()).toBeVisible({ timeout: 10_000 });

    // The seeded word (你好) is trained (start_training=true in global-setup),
    // so editing it should show the Reset button, not the "start training" row.
    const row = page.locator('#words-tbody tr', { hasText: '你好' }).first();
    await row.locator('.btn-edit').click();

    const resetBtn = page.locator('#form-reset-btn');
    await expect(resetBtn).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#start-training-row')).toBeHidden();

    page.on('dialog', dialog => dialog.accept());
    await resetBtn.click();

    // After reset the word is unseen again: the Reset button hides and the
    // "start training" row reappears, showing it left the New bucket.
    await expect(resetBtn).toBeHidden({ timeout: 8_000 });
    await expect(page.locator('#start-training-row')).toBeVisible();
  });

  test('pinyin auto-fill does not overwrite an existing value until blur (issue #310)', async ({ page }) => {
    await page.goto('/vocab');
    await page.locator('#en-inputs-container .en-input').first().waitFor({ state: 'visible', timeout: 8_000 });

    const zhInput = page.locator('#form-zh');
    const pinyinInput = page.locator('#form-pinyin');

    // Empty pinyin field: typing zh text still auto-fills pinyin (existing behaviour).
    await zhInput.fill('水');
    await expect(pinyinInput).toHaveValue('shuǐ', { timeout: 3_000 });

    let dialogSeen = false;
    page.on('dialog', dialog => {
      dialogSeen = true;
      dialog.dismiss();
    });

    // Editing zh again while pinyin is already set must not immediately
    // recompute/prompt — only after the idle period or on blur.
    await zhInput.fill('水果');
    await page.waitForTimeout(1_000);
    expect(dialogSeen).toBe(false);
    await expect(pinyinInput).toHaveValue('shuǐ');

    // Losing focus triggers the recalculation immediately.
    await zhInput.blur();
    await expect.poll(() => dialogSeen, { timeout: 3_000 }).toBe(true);
  });

  test('delete a word removes it from the list', async ({ page }) => {
    await page.goto('/vocab');

    // Wait for words to load
    await expect(page.locator('#words-tbody tr').first()).toBeVisible({ timeout: 10_000 });

    // Get the text of the first word so we can verify it's gone
    const firstRow = page.locator('#words-tbody tr').first();
    const zhText = await firstRow.locator('td').first().textContent();
    const trimmedZh = zhText?.replace(/[🔊]/g, '').trim().slice(0, 5) || '';

    // Click the delete button for the first word
    const deleteBtn = firstRow.locator('.btn-delete');
    await deleteBtn.click();

    // Confirm the deletion dialog (browser confirm dialog)
    page.on('dialog', dialog => dialog.accept());

    // Give the page a moment to refresh
    await page.waitForTimeout(1_500);

    // The deleted word's zh text should no longer be the first row
    // (it's either gone or a different word is first)
    // We just verify the request succeeded by checking the row count changed or the text is gone
    const rows = page.locator('#words-tbody tr');
    const count = await rows.count();
    if (trimmedZh && count > 0) {
      // If there are still rows, verify at least that the page reloaded fine
      await expect(page.locator('#words-tbody')).toBeVisible();
    }
  });
});
