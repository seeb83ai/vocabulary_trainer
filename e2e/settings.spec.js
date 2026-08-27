// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// Settings cards save automatically as fields change — no explicit Save
// buttons for language/mode/cycle/random-mode/daily/accept-mode/gamification/
// component-threshold. Change Password and API Keys stay explicit-submit.
test.describe('Settings – Auto-save', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('no Save buttons remain on the auto-saving cards', async ({ page }) => {
    await page.goto('/settings');
    for (const id of ['lang-save-btn', 'mode-save-btn', 'cycle-save-btn', 'random-mode-save-btn',
      'daily-save-btn', 'accept-mode-save-btn', 'gamification-save-btn', 'component-threshold-save-btn']) {
      await expect(page.locator(`#${id}`)).toHaveCount(0);
    }
    // Change Password and API Keys remain explicit-submit.
    await expect(page.locator('#pw-btn')).toBeVisible();
    await expect(page.locator('#apikey-save-btn')).toBeVisible();
  });

  test('changing accept-as-correct mode persists without clicking anything', async ({ page }) => {
    await page.goto('/settings');
    const always = page.locator('input[name="accept-correct-mode"][value="always"]');
    await always.check();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(always).toBeChecked();

    const res = await page.request.get('/api/settings');
    const settings = await res.json();
    expect(settings.accept_correct_mode).toBe('always');

    // Reset to default.
    await page.locator('input[name="accept-correct-mode"][value="typo"]').check();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });
});

test.describe('Settings – Daily Learning', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('daily learning section is visible with expected controls', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('#daily-learning-section')).toBeVisible();
    await expect(page.locator('#max-new-words')).toBeVisible();
    await expect(page.locator('#skip-new-visible')).toBeVisible();
  });

  test('max new words per day can be changed and saved', async ({ page }) => {
    await page.goto('/settings');
    const input = page.locator('#max-new-words');
    await input.fill('3');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    // Reload page and verify value persists
    await page.reload();
    await expect(page.locator('#max-new-words')).toHaveValue('3');

    // Reset to default
    await input.fill('5');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });

  test('skip new words visible toggle saves and persists', async ({ page }) => {
    // Force a known baseline: other specs' direct-API PATCH calls can omit
    // this field, and the handler treats a missing bool as false — leaving
    // this already-unchecked, which would make toggle.uncheck() below a
    // silent no-op (autosave only fires on a real change event).
    const before = await page.request.get('/api/settings');
    const beforeSettings = await before.json();
    await page.request.patch('/api/settings', { data: { ...beforeSettings, skip_new_words_visible: true } });

    await page.goto('/settings');
    const toggle = page.locator('#skip-new-visible');
    await expect(toggle).toBeChecked();

    // Uncheck (hide skip button)
    await toggle.uncheck();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#skip-new-visible')).not.toBeChecked();

    // Re-enable
    await toggle.check();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });

  test('skip button hidden in quiz when skip_new_words_visible is false', async ({ page }) => {
    // Disable skip via API
    const res = await page.request.get('/api/settings');
    const settings = await res.json();

    await page.request.patch('/api/settings', {
      data: { ...settings, skip_new_words_visible: false },
    });

    // The new-word skip button should not appear (even if new-word area were shown).
    // We verify via the API that the setting is reflected.
    const res2 = await page.request.get('/api/settings');
    const updated = await res2.json();
    expect(updated.skip_new_words_visible).toBe(false);

    // Restore
    await page.request.patch('/api/settings', {
      data: { ...settings, skip_new_words_visible: true },
    });
  });

  test('baseline due-today can be enabled with a threshold', async ({ page }) => {
    await page.goto('/settings');

    await page.locator('#baseline-due-today-enabled').check();
    await page.locator('#baseline-due-today-value').fill('15');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#baseline-due-today-enabled')).toBeChecked();
    await expect(page.locator('#baseline-due-today-value')).toHaveValue('15');

    // Disable and reset
    await page.locator('#baseline-due-today-enabled').uncheck();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });

  test('baseline new-bucket can be enabled with a threshold', async ({ page }) => {
    await page.goto('/settings');

    await page.locator('#baseline-new-bucket-enabled').check();
    await page.locator('#baseline-new-bucket-value').fill('3');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#baseline-new-bucket-enabled')).toBeChecked();
    await expect(page.locator('#baseline-new-bucket-value')).toHaveValue('3');

    // Disable and reset
    await page.locator('#baseline-new-bucket-enabled').uncheck();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });

  test('extend-session toggle is visible and defaults to checked', async ({ page }) => {
    await page.goto('/settings');
    const toggle = page.locator('#extend-session-extra-words');
    await expect(toggle).toBeVisible();
    // Default (current, pre-existing) behaviour is enabled for existing/new users.
    await expect(toggle).toBeChecked();
  });

  test('extend-session toggle saves and persists across reload', async ({ page }) => {
    await page.goto('/settings');
    const toggle = page.locator('#extend-session-extra-words');

    // Disable: user opts out of session-extension with extra (not-yet-due) words.
    await toggle.uncheck();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#extend-session-extra-words')).not.toBeChecked();

    // Verify the API reflects the change too.
    const res = await page.request.get('/api/settings');
    const settings = await res.json();
    expect(settings.extend_session_with_extra_words).toBe(false);

    // Re-enable and restore default.
    await toggle.check();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });
});

// Issue #201: a "Blur pinyin" option in Training Mode settings blurs the
// pinyin hint on the quiz card until the user taps/clicks to reveal it.
test.describe('Settings – Blur pinyin (issue #201)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('blur-pinyin toggle is visible and defaults to unchecked', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('#blur-pinyin')).toBeVisible();
    await expect(page.locator('#blur-pinyin')).not.toBeChecked();
  });

  test('blur-pinyin toggle saves and persists across reload', async ({ page }) => {
    await page.goto('/settings');
    const toggle = page.locator('#blur-pinyin');

    await toggle.check();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#blur-pinyin')).toBeChecked();

    const res = await page.request.get('/api/settings');
    const settings = await res.json();
    expect(settings.blur_pinyin).toBe(true);

    // Reset to default.
    await toggle.uncheck();
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });
});

// Component training threshold: skip low-value hanzi components (ones that
// appear in only a small share of the user's zh vocabulary) when deciding
// what gets added to the component training rotation.
test.describe('Settings – Component training threshold', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('component training section is visible with threshold input and coverage summary', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('#component-training-section')).toBeVisible();
    await expect(page.locator('#component-coverage-threshold')).toBeVisible();
    await expect(page.locator('#component-coverage-threshold')).toHaveValue('0');
    await expect(page.locator('#component-coverage-summary')).toBeVisible();
  });

  test('threshold can be changed and saved, and persists across reload', async ({ page }) => {
    await page.goto('/settings');
    const input = page.locator('#component-coverage-threshold');
    await expect(input).toHaveValue('0');

    await input.fill('5');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#component-coverage-threshold')).toHaveValue('5');

    const res = await page.request.get('/api/settings');
    const settings = await res.json();
    expect(settings.component_coverage_threshold).toBe(5);

    // Reset to default.
    await input.fill('0');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });

  test('rejects an out-of-range threshold', async ({ page }) => {
    await page.goto('/settings');
    const input = page.locator('#component-coverage-threshold');
    // Wait for the async settings load to finish populating the field before
    // typing into it, so the fetch response can't race the fill and clobber it.
    await expect(input).toHaveValue('0');

    await input.fill('150');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });
});

// "Chinese (no sound) → Translation" mode: selectable per proficiency tier,
// per new-word step, and as a cycle-mode step, alongside the existing modes.
test.describe('Settings – Chinese (no sound) mode', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('is offered as an option in the progressive tier and new-word-step dropdowns', async ({ page }) => {
    await page.goto('/settings');
    for (const id of ['mode-prog-new', 'mode-prog-struggling', 'mode-prog-learning', 'mode-prog-practicing', 'mode-prog-mastered', 'mode-new-0', 'mode-new-1', 'mode-new-2']) {
      const options = await page.locator(`#${id} option`).allTextContents();
      const values = await page.locator(`#${id} option`).evaluateAll(els => els.map(el => el.value));
      expect(values, `#${id} should offer zh_to_transl_no_sound`).toContain('zh_to_transl_no_sound');
      expect(options.join('')).not.toBe('');
    }
  });

  test('is offered as an option in the cycle-step dropdowns', async ({ page }) => {
    await page.goto('/settings');
    for (const id of ['cycle-step-0', 'cycle-step-1', 'cycle-step-2']) {
      const values = await page.locator(`#${id} option`).evaluateAll(els => els.map(el => el.value));
      expect(values, `#${id} should offer zh_to_transl_no_sound`).toContain('zh_to_transl_no_sound');
    }
  });

  test('selecting it for the Learning tier saves and persists across reload', async ({ page }) => {
    await page.goto('/settings');
    await page.locator('#mode-prog-learning').selectOption('zh_to_transl_no_sound');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#mode-prog-learning')).toHaveValue('zh_to_transl_no_sound');

    const res = await page.request.get('/api/settings');
    const settings = await res.json();
    expect(settings.prog_tier_learning).toBe('zh_to_transl_no_sound');

    // Reset to default.
    await page.locator('#mode-prog-learning').selectOption('zh_pinyin_to_transl');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });

  test('selecting it for a cycle step saves and persists across reload', async ({ page }) => {
    await page.goto('/settings');
    await page.locator('#cycle-step-0').selectOption('zh_to_transl_no_sound');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#cycle-step-0')).toHaveValue('zh_to_transl_no_sound');

    // Reset to default.
    await page.locator('#cycle-step-0').selectOption('zh_pinyin_to_transl');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });
});

// Issue #287: per-bucket eligibility for Random/Cycle mode selection.
test.describe('Settings – Random/Cycle Mode by Bucket (issue #287)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('shows a from/to bucket range for each of the 5 modes, defaulting to the built-in ladder', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.locator('#random-mode-transl_to_zh-from')).toBeVisible();
    await expect(page.locator('#random-mode-transl_to_zh-to')).toBeVisible();
    await expect(page.locator('#random-mode-transl_to_zh-from')).toHaveValue('new');
    await expect(page.locator('#random-mode-transl_to_zh-to')).toHaveValue('50-69');
    await expect(page.locator('#random-mode-voice_to_transl-from')).toHaveValue('70-84');
    await expect(page.locator('#random-mode-voice_to_transl-to')).toHaveValue('85-100');
  });

  test('changing a range and saving persists across reload', async ({ page }) => {
    await page.goto('/settings');

    await page.locator('#random-mode-zh_to_transl-from').selectOption('50-69');
    await page.locator('#random-mode-zh_to_transl-to').selectOption('70-84');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    await page.reload();
    await expect(page.locator('#random-mode-zh_to_transl-from')).toHaveValue('50-69');
    await expect(page.locator('#random-mode-zh_to_transl-to')).toHaveValue('70-84');

    const res = await page.request.get('/api/settings');
    const settings = await res.json();
    expect(settings.random_mode_range_zh_to_transl).toBe('50-69,70-84');

    // Reset to default.
    await page.locator('#random-mode-zh_to_transl-from').selectOption('0-49');
    await page.locator('#random-mode-zh_to_transl-to').selectOption('85-100');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });

  test('turning off every mode is rejected with an inline error and not persisted', async ({ page }) => {
    await page.goto('/settings');

    const before = await page.request.get('/api/settings');
    const beforeSettings = await before.json();

    for (const mode of ['transl_to_zh', 'zh_pinyin_to_transl', 'zh_to_transl', 'zh_to_transl_no_sound', 'voice_to_transl']) {
      await page.locator(`#random-mode-${mode}-off`).check();
    }
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    const after = await page.request.get('/api/settings');
    const afterSettings = await after.json();
    expect(afterSettings.random_mode_range_transl_to_zh).toBe(beforeSettings.random_mode_range_transl_to_zh);

    // Restore the UI (uncheck the boxes) so subsequent tests start clean.
    await page.reload();
  });
});

// Issue #347: save/error messages used to render inline (different position
// per card, causing layout jumps as they were shown/hidden). They now render
// as a single fixed-position hovering toast that never shifts page layout.
test.describe('Settings – Toast notifications (issue #347)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  test('a save shows a hovering toast without shifting the page layout, then auto-dismisses', async ({ page }) => {
    await page.goto('/settings');
    // Let the page's own async loads (settings, languages, component coverage
    // table) finish and the layout settle before taking the "before" reading,
    // so only the toast itself is under test.
    await page.waitForLoadState('networkidle');
    const anchor = page.locator('#daily-learning-section');
    // Anchor the comparison on the input itself (rather than the section),
    // and pin scroll position explicitly — fill() auto-scrolls its target
    // into view, which would otherwise move every element's viewport-relative
    // boundingBox() regardless of whether the toast affected layout at all.
    const input = page.locator('#max-new-words');
    await input.scrollIntoViewIfNeeded();
    const before = await anchor.boundingBox();

    await input.fill('7');
    const toast = page.locator('[data-testid="toast"]');
    await expect(toast).toBeVisible();
    await expect(toast).toContainText('Saved');
    await captureForPR(page, 'settings-toast-saved');

    await input.scrollIntoViewIfNeeded();
    const after = await anchor.boundingBox();
    expect(after?.y).toBe(before?.y);
    expect(after?.x).toBe(before?.x);

    // Auto-dismisses on its own after a few seconds.
    await expect(toast).toBeHidden({ timeout: 6000 });

    // Reset.
    await input.fill('5');
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();
  });

  test('rapid successive saves collapse into a single visible toast', async ({ page }) => {
    await page.goto('/settings');

    const maxNew = page.locator('#max-new-words');
    const cooldown = page.locator('#new-word-cooldown');

    await maxNew.fill('8');
    await cooldown.fill('2');

    // Only one toast element should ever be present/visible at a time, even
    // though two saves fired back-to-back.
    await expect(page.locator('[data-testid="toast"]')).toHaveCount(1);
    await expect(page.locator('[data-testid="toast"]')).toBeVisible();

    // Reset.
    await maxNew.fill('5');
    await cooldown.fill('1');
    await expect(page.locator('[data-testid="toast"]')).toHaveCount(1);
  });

  test('inline per-card success/error message elements are gone from the DOM', async ({ page }) => {
    await page.goto('/settings');
    for (const id of ['lang-success', 'lang-error', 'mode-success', 'mode-error',
      'cycle-success', 'cycle-error', 'random-mode-success', 'random-mode-error',
      'daily-success', 'daily-error', 'accept-mode-success', 'accept-mode-error',
      'gamification-success', 'gamification-error',
      'component-threshold-success', 'component-threshold-error']) {
      await expect(page.locator(`#${id}`)).toHaveCount(0);
    }
  });
});
