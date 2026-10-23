// @ts-check
import { test, expect } from '@playwright/test';
import { createHash } from 'node:crypto';
import { openAuthModal } from './helpers/auth.js';
import { captureForPR } from './helpers/screenshot.js';
import { SERVER_LEAKED_PASSWORD } from './global-setup.js';

// Leaked passwords (Have I Been Pwned) are blocked when an account is created
// and when a password is changed. The browser checks first, the server checks
// again. A leaked password at login shows a warning with a link to Settings.

function sha1(password) {
  return createHash('sha1').update(password).digest('hex').toUpperCase();
}

// Browser-side HIBP mock: the passwords in `leaked` are reported as leaked,
// all other passwords as clean.
async function mockBrowserHIBP(page, leaked = []) {
  const hashes = leaked.map(sha1);
  await page.route('https://api.pwnedpasswords.com/**', route => {
    const prefix = route.request().url().split('/').pop().toUpperCase();
    const body = hashes.filter(h => h.startsWith(prefix)).map(h => `${h.slice(5)}:42`).join('\r\n');
    route.fulfill({ status: 200, body });
  });
}

function uniqueEmail(tag) {
  return `e2e-pwned-${tag}-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
}

async function submitRegister(page, email, password) {
  await page.goto('/');
  await openAuthModal(page, 'register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(password);
  await page.locator('#reg-confirm').fill(password);
  await page.locator('#register-btn').click();
}

async function loginStatus(page, email, password) {
  const res = await page.request.post('/api/login', { data: { email, password } });
  return res.status();
}

async function submitPasswordChange(page, current, next) {
  await page.goto('/settings');
  await page.locator('#pw-toggle').click();
  await page.locator('#pw-current').fill(current);
  await page.locator('#pw-new').fill(next);
  await page.locator('#pw-confirm').fill(next);
  await page.locator('#pw-btn').click();
}

test.describe('Leaked passwords', () => {
  test('registration with a leaked password shows an error and creates no account', async ({ page }) => {
    const leaked = 'LeakedInBrowser1!';
    await mockBrowserHIBP(page, [leaked]);
    const registerCalls = [];
    page.on('request', req => { if (req.url().endsWith('/api/register')) registerCalls.push(req); });

    const email = uniqueEmail('reg');
    await submitRegister(page, email, leaked);

    await expect(page.locator('#register-error')).toContainText('data breach');
    await captureForPR(page, 'pwned-register');
    expect(registerCalls).toHaveLength(0);
    expect(await loginStatus(page, email, leaked)).toBe(401);
  });

  test('the server blocks a leaked password when the browser check finds nothing', async ({ page }) => {
    await mockBrowserHIBP(page, []);
    const email = uniqueEmail('server');
    await submitRegister(page, email, SERVER_LEAKED_PASSWORD);

    await expect(page.locator('#register-error')).toContainText('data breach');
    await expect(page).not.toHaveURL(/\/train/);
    expect(await loginStatus(page, email, SERVER_LEAKED_PASSWORD)).toBe(401);
  });

  test('login with a leaked password shows a warning that links to the password form', async ({ page }) => {
    const email = uniqueEmail('login');
    const password = 'LoginLeak123!';
    expect((await page.request.post('/api/register', { data: { email, password } })).ok()).toBeTruthy();
    await page.request.post('/api/logout');
    await mockBrowserHIBP(page, [password]);

    await page.goto('/');
    await openAuthModal(page);
    await page.locator('#signin-email').fill(email);
    await page.locator('#signin-password').fill(password);
    await page.locator('#signin-btn').click();

    const warning = page.locator('#hibp-warning');
    await expect(warning).toBeVisible();
    await expect(warning.locator('#hibp-change')).toBeVisible();
    await expect(warning.locator('#hibp-continue')).toBeVisible();
    await captureForPR(page, 'pwned-login-warning');

    await warning.locator('#hibp-change').click();
    await expect(page).toHaveURL('/settings#change-password');
    await expect(page.locator('#pw-form')).toBeVisible();
    await expect(page.locator('#pw-current')).toBeFocused();
    await captureForPR(page, 'pwned-settings-form-open');
  });

  test('Continue on the login warning goes to training', async ({ page }) => {
    const email = uniqueEmail('continue');
    const password = 'LoginLeak123!';
    expect((await page.request.post('/api/register', { data: { email, password } })).ok()).toBeTruthy();
    await page.request.post('/api/logout');
    await mockBrowserHIBP(page, [password]);

    await page.goto('/');
    await openAuthModal(page);
    await page.locator('#signin-email').fill(email);
    await page.locator('#signin-password').fill(password);
    await page.locator('#signin-btn').click();

    await page.locator('#hibp-continue').click();
    await expect(page).toHaveURL('/train');
  });

  test('changing to a leaked password is blocked in the browser and on the server', async ({ page }) => {
    const email = uniqueEmail('change');
    const password = 'ChangeLeak123!';
    expect((await page.request.post('/api/register', { data: { email, password } })).ok()).toBeTruthy();

    const leaked = 'NewLeaked123!';
    await mockBrowserHIBP(page, [leaked]);
    await submitPasswordChange(page, password, leaked);
    await expect(page.getByText(/data breach/)).toBeVisible();
    await captureForPR(page, 'pwned-settings-change');

    await page.unrouteAll();
    await mockBrowserHIBP(page, []);
    await submitPasswordChange(page, password, SERVER_LEAKED_PASSWORD);
    await expect(page.getByText(/data breach/)).toBeVisible();

    expect(await loginStatus(page, email, leaked)).toBe(401);
    expect(await loginStatus(page, email, SERVER_LEAKED_PASSWORD)).toBe(401);
    expect(await loginStatus(page, email, password)).toBe(200);
  });
});
