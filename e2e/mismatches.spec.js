// @ts-check
import { test, expect } from '@playwright/test';

// ─────────────────────────────────────────────────────────────────────────────
// Issue #280: component quiz answers must trigger the mismatch UI/tracking,
// the same way vocabulary-word answers already do. A wrong answer for a
// component (e.g. typing "to go" for 扑) can happen to be the translation of
// a different word or component — that should render the yellow "belongs to"
// box on the result screen (mirroring renderWordAnswerResult) and show up on
// the /mismatches page.
// ─────────────────────────────────────────────────────────────────────────────

test.describe('Component mismatch detection (issue #280)', () => {
  test.use({ storageState: 'e2e/.auth/user.json' });

  async function useZhToTranslMode(page) {
    await page.request.patch('/api/training-filters', {
      data: { mode: 'zh_to_transl', langs: ['en'], bucket: '', mnemonics: true, components: true, tags: [] },
    });
    return page.addInitScript(() => {
      localStorage.setItem('quizMode', 'zh_to_transl');
      localStorage.setItem('quizLangs', JSON.stringify(['en']));
    });
  }

  test('wrong component answer that matches a different word shows the "belongs to" mismatch box', async ({ page }) => {
    await page.route('**/api/quiz/next*', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          card_type: 'component',
          prompt: '扑',
          pinyin: 'pū',
          is_new: false,
          is_also_word: false,
          definitions: { en: 'to rap, to tap; script; to let go' },
        }),
      });
    });
    await page.route('**/api/component/answer', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          correct: false,
          correct_answers: { en: 'to rap, to tap; script; to let go' },
          interval_days: 1,
          total_correct: 11,
          total_attempts: 19,
          confused_with: {
            zh_kind: 'component',
            zh_component: '扑',
            zh_text: '扑',
            zh_pinyin: 'pū',
            confused_with_kind: 'word',
            confused_with_id: 42,
            confused_with_text: '去',
            confused_with_pinyin: 'qù',
            confused_with_translations: { en: ['to go'] },
            mode: 'zh_pinyin_to_transl',
            count: 1,
            last_seen: new Date().toISOString(),
          },
        }),
      });
    });

    await useZhToTranslMode(page);
    await page.goto('/train');
    await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
    await expect(page.locator('#prompt-word')).toHaveText('扑');

    await page.locator('#answer-input').fill('To go');
    await page.locator('#answer-form button[type="submit"]').click();
    await expect(page.locator('#result-icon')).toHaveText('✗ Wrong', { timeout: 8_000 });

    // Same yellow "belongs to" box used for word-vs-word confusions, adapted
    // for a component result — must show the confused-with word.
    const yellowBox = page.locator('#word-breakdown .bg-yellow-50');
    await expect(yellowBox).toBeVisible();
    await expect(yellowBox).toContainText('去');
    await expect(yellowBox).toContainText('to go');
  });

  test('mismatches page lists a component-vs-word confusion row', async ({ page }) => {
    await page.route('**/api/mismatches', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          {
            zh_kind: 'component',
            zh_component: '扑',
            zh_text: '扑',
            zh_pinyin: 'pū',
            zh_translations: { en: ['to rap, to tap; script; to let go'] },
            confused_with_kind: 'word',
            confused_with_id: 42,
            confused_with_text: '去',
            confused_with_pinyin: 'qù',
            confused_with_translations: { en: ['to go'] },
            mode: 'zh_pinyin_to_transl',
            count: 3,
            last_seen: new Date().toISOString(),
          },
        ]),
      });
    });

    const audioRequests = [];
    await page.route('**/api/audio/**', (route) => {
      audioRequests.push(route.request().url());
      route.fulfill({ status: 200, contentType: 'audio/mpeg', body: Buffer.alloc(0) });
    });

    await page.goto('/mismatches');
    await expect(page.locator('#table-wrap')).toBeVisible({ timeout: 8_000 });
    const row = page.locator('#mismatches-tbody tr').first();
    await expect(row).toContainText('扑');
    await expect(row).toContainText('去');
    await expect(row).toContainText('to go');

    // The "word tested" cell (扑) is a component — its play button must hit the
    // component audio endpoint, not the word one (which would 404 for a
    // character that isn't a standalone word).
    const cells = row.locator('td');
    await cells.nth(0).locator('button').click();
    await expect.poll(() => audioRequests.at(-1)).toContain('/api/audio/component/');

    // The "confused with" cell (去) is a real word — its play button must hit
    // the word audio endpoint with its word_id.
    await cells.nth(1).locator('button').click();
    await expect.poll(() => audioRequests.at(-1)).toContain('/api/audio/42');
  });
});
