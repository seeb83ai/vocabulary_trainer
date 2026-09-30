// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// Redesigned Stats page (app redesign handoff): segmented tabs, four summary
// tiles, training history, Levels (stacked bar + rows incl. Unseen), due
// dates, level history, hardest / most practiced rows and a folded
// "Last 14 days" table. Each test registers its own user.

const PASSWORD = 'E2eStatsRedesign123!';

async function registerUser(page) {
  await page.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
  const email = `e2e-stats-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
  await page.goto('/#register');
  await page.locator('#reg-email').fill(email);
  await page.locator('#reg-password').fill(PASSWORD);
  await page.locator('#reg-confirm').fill(PASSWORD);
  await page.locator('#register-btn').click();
  await expect(page).toHaveURL('/train', { timeout: 10_000 });
}

async function setGamification(page, on) {
  const st = await (await page.request.get('/api/settings')).json();
  await page.request.patch('/api/settings', { data: { ...st, gamification_enabled: on } });
}

async function seedAndAnswer(page) {
  const res = await page.request.post('/api/words', {
    data: { zh_text: '好', pinyin: 'hǎo', translations: { en: ['good'] }, tags: ['basics'], start_training: true },
  });
  expect(res.ok()).toBe(true);
  const word = await res.json();
  await page.request.post('/api/words', {
    data: { zh_text: '猫', pinyin: 'māo', translations: { en: ['cat'] }, tags: [], start_training: false },
  });
  const ans = await page.request.post('/api/quiz/answer', {
    data: { word_id: word.id, mode: 'zh_to_transl', answer: 'good', langs: ['en'] },
  });
  expect(ans.ok()).toBe(true);
}

test.describe('Stats redesign', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('summary tiles, levels with unseen and the folded table', async ({ page }) => {
    await registerUser(page);
    await setGamification(page, true);
    await seedAndAnswer(page);
    await page.goto('/stats');

    await expect(page.locator('#tile-answers-value')).toHaveText('1', { timeout: 10_000 });
    await expect(page.locator('#tile-accuracy-value')).toHaveText('100%');
    await expect(page.locator('#tile-training-value')).toHaveText('1');
    await expect(page.locator('#tile-streak-label')).toHaveText('Day streak');
    await expect(page.locator('#tile-streak-value')).toHaveText('1');

    await expect(page.locator('#levels-bar > span').first()).toBeVisible();
    const legend = page.locator('#tier-legend');
    await expect(legend).toContainText('Unseen');
    await expect(legend.locator('.sx-level-row')).toHaveCount(6);
    await captureForPR(page, 'stats-words');
    await page.locator('#word-stats-section').scrollIntoViewIfNeeded();
    await captureForPR(page, 'stats-levels');

    await expect(page.locator('#stats-table-wrap')).toBeHidden();
    await page.locator('#stats-table-toggle').click();
    await expect(page.locator('#stats-table-wrap')).toBeVisible();
    await expect(page.locator('#stats-table-body tr')).toHaveCount(1);
  });

  test('without gamification the streak tile shows training time', async ({ page }) => {
    await registerUser(page);
    await setGamification(page, false);
    await seedAndAnswer(page);
    await page.goto('/stats');

    await expect(page.locator('#tile-streak-label')).toHaveText('Training time', { timeout: 10_000 });
  });

  test('tabs are a segmented control', async ({ page }) => {
    await registerUser(page);
    await page.goto('/stats');
    await expect(page.locator('#tab-words')).toHaveAttribute('aria-pressed', 'true');
    await page.locator('#tab-pinyin').click();
    await expect(page.locator('#tab-pinyin')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#panel-pinyin')).toBeVisible();
    await captureForPR(page, 'stats-pinyin');
    await page.locator('#tab-mnemonics').click();
    await expect(page.locator('#panel-mnemonics')).toBeVisible();
  });
});

test.describe('Stats redesign – phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('tiles fit without horizontal scroll', async ({ page }) => {
    await registerUser(page);
    await seedAndAnswer(page);
    await page.goto('/stats');
    await expect(page.locator('#tile-answers-value')).toHaveText('1', { timeout: 10_000 });
    const scrollW = await page.evaluate(() => document.documentElement.scrollWidth);
    expect(scrollW).toBeLessThanOrEqual(390);
    await captureForPR(page, 'stats-phone');
  });
});
