// @ts-check
import { test, expect } from '@playwright/test';
import { syncNewWordMode } from './helpers/mode.js';
import { seedDueComponent, seedReviewWord } from './helpers/db.js';
import { captureForPR } from './helpers/screenshot.js';

// A character that is a word and a component (issue #651): only the word is
// trained. The word card accepts the component definitions too, and the
// result screen shows the component meanings in "More info".

async function setup(page) {
  const email = `e2e-wordcomp-${Date.now()}-${Math.random().toString(36).slice(2)}@test.local`;
  const regRes = await page.request.post('/api/register', {
    data: { email, password: 'WordComponent123!' },
  });
  expect(regRes.ok()).toBeTruthy();
  const settings = await (await page.request.get('/api/settings')).json();
  await page.request.patch('/api/settings', {
    data: { ...settings, wrong_answer_retry_mode: 'off' },
  });
  await page.request.patch('/api/training-filters', {
    data: { mode: 'zh_pinyin_to_transl', langs: ['en'], bucket: '', mnemonics: false, components: true, tags: [] },
  });
  await syncNewWordMode(page, 'zh_pinyin_to_transl');
  const seedRes = await page.request.post('/api/words', {
    data: { zh_text: '更', pinyin: 'gèng', translations: { en: ['more', 'further'] }, tags: [] },
  });
  expect(seedRes.ok()).toBeTruthy();
  seedReviewWord(email, '更', 0);
  // The component is more overdue than the word, so it would come first if
  // components that are also words were still trained.
  seedDueComponent(email, '更', 'even more; to experience; watch (of the night)', '-2 days');
  await page.addInitScript(() => {
    localStorage.setItem('quizMode', 'zh_pinyin_to_transl');
    localStorage.setItem('quizLangs', JSON.stringify(['en']));
  });
  return email;
}

test.describe('Word that is also a component', () => {
  test('only the word is trained and it accepts the component meanings', async ({ page }) => {
    await setup(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText('更');
    await expect(page.locator('#mode-label')).toContainText('Also a Component');
    await captureForPR(page, 'word-is-component-card');

    await page.locator('#answer-input').fill('to experience');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#result-icon')).toHaveText('Correct');

    await page.locator('#result-more-info-toggle').click();
    const body = page.locator('#result-more-info-body');
    await expect(body).toBeVisible();
    await expect(body).toContainText('Component meanings');
    await expect(body).toContainText('watch (of the night)');
    await captureForPR(page, 'word-is-component-more-info');
  });
});
