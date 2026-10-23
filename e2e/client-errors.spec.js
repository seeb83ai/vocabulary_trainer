// @ts-check
import { test, expect } from '@playwright/test';
import { syncNewWordMode } from './helpers/mode.js';
import { seedReviewWord } from './helpers/db.js';
import { captureForPR } from './helpers/screenshot.js';

// Issue #581: when the result screen fails to draw, the app shows the error
// and goes on to the next card instead of leaving a half-drawn screen.

async function setup(page) {
  const email = `e2e-clienterr-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', { data: { email, password: 'ClientErr123!' } });
  expect(regRes.ok()).toBeTruthy();
  for (const [zh, pinyin, en] of [['关于', 'guān yú', 'about'], ['已经', 'yǐ jīng', 'already']]) {
    const res = await page.request.post('/api/words', {
      data: { zh_text: zh, pinyin, translations: { en: [en] }, tags: [], start_training: true },
    });
    expect(res.ok()).toBeTruthy();
    seedReviewWord(email, zh, 0);
  }
  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: false, components: false, tags: [] },
  });
  await syncNewWordMode(page, 'zh_to_transl');
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  await page.goto('/train');
  await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
}

// The answer is saved, but the reply has a shape the result screen can't draw.
async function breakResultScreen(page) {
  await page.route('**/api/quiz/answer', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ correct: false, translations: { en: 'not a list' } }),
  }));
}

test.describe('Result screen errors', () => {
  test('a result screen that fails to draw shows the error and loads the next card', async ({ page }) => {
    await setup(page);
    await breakResultScreen(page);

    const nextCard = page.waitForResponse(r => r.url().includes('/api/quiz/next'));
    await page.locator('#answer-input').fill('wrong');
    await page.locator('#answer-form button[type="submit"]').click();

    await expect(page.getByTestId('toast')).toBeVisible();
    await expect(page.getByTestId('toast')).toHaveClass(/app-toast-error/);
    await nextCard;
    await expect(page.locator('#result-area')).not.toBeVisible();
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#answer-input')).toHaveValue('');

    // The report dialog offers the script error with its stack.
    await page.locator('#nav-report').click();
    await expect(page.locator('#issue-error-text')).toContainText('Stack:');
    await expect(page.locator('#issue-error-text')).not.toContainText('Request:');
  });
});

test.describe('Last error in the bug report', () => {
  test('the report dialog offers the last error with its failed request', async ({ page }) => {
    await setup(page);
    await page.route('**/api/quiz/answer', route => route.fulfill({
      status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'database is locked' }),
    }));
    await page.locator('#answer-input').fill('wrong');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.getByTestId('toast')).toContainText('database is locked');

    await page.locator('#nav-report').click();
    await expect(page.locator('#issue-modal')).toBeVisible({ timeout: 10_000 });
    const includeError = page.locator('#issue-include-error');
    await expect(includeError).toBeChecked();
    const errorText = page.locator('#issue-error-text');
    await expect(errorText).toContainText('database is locked');
    await expect(errorText).toContainText('POST /api/quiz/answer → 500');
    await expect(errorText).not.toHaveAttribute('contenteditable');
    await captureForPR(page, 'report-last-error');

    // Unchecked: the text is hidden and not sent.
    await page.locator('label[for="issue-include-error"]').click();
    await expect(errorText).not.toBeVisible();
    await page.locator('label[for="issue-include-error"]').click();
    await expect(errorText).toBeVisible();

    await page.locator('#issue-include-screenshot').uncheck();
    await page.locator('#issue-title').fill('Answer failed');
    await page.locator('#issue-description').fill('The answer did not save.');
    const sent = page.waitForRequest('**/api/github/issues');
    await page.locator('#issue-submit').click();
    const body = (await sent).postDataJSON();
    expect(body.client_error).toContain('database is locked');
    expect(body.client_error).toContain('POST /api/quiz/answer → 500');
    await expect(page.locator('#issue-success')).toBeVisible({ timeout: 10_000 });
  });

  test('without a recent error the report dialog has no error option', async ({ page }) => {
    await setup(page);
    await page.locator('#nav-report').click();
    await expect(page.locator('#issue-modal')).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#issue-include-error')).not.toBeVisible();
  });
});

// Issue #621: an expired session (401) sends the user to /login. The page that
// made the call must stop there, not run on with an empty reply and record
// "Cannot read properties of undefined" as the last error.
test.describe('Expired session', () => {
  test('a 401 on the next card goes to login without a script error', async ({ page }) => {
    await setup(page);
    // Only once: the test session is still valid, so /login sends the
    // browser back to the app, which must then load normally.
    await page.route('**/api/quiz/next*', route => route.fulfill({
      status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'unauthorized' }),
    }), { times: 1 });
    const toLogin = page.waitForRequest(r => r.isNavigationRequest() && new URL(r.url()).pathname === '/login');
    await page.locator('#answer-input').fill('wrong');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#next-btn')).toBeVisible({ timeout: 8_000 });
    await page.locator('#next-btn').click();

    await toLogin;
    await expect(page).toHaveURL(/\/train$/, { timeout: 10_000 });
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    const lastError = await page.evaluate(() => sessionStorage.getItem('lastClientError'));
    expect(lastError).toBeNull();
  });
});
