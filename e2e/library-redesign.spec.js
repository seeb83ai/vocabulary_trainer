// @ts-check
import { test, expect } from '@playwright/test';
import { captureForPR } from './helpers/screenshot.js';

// Redesigned Mismatches and Mnemonics pages (app redesign handoff).
// Mismatches: card per pair, client-side sort, count pill, empty state.
// Mnemonics: segmented tabs with filled counts, key + input items, actor
// groups kept as sub-headings.

test.use({ storageState: 'e2e/.auth/user.json' });

const DAY = 86400000;
const PAIRS = [
  {
    zh_kind: 'word', zh_word_id: 101, zh_text: '买', zh_pinyin: 'mǎi', zh_translations: { en: ['to buy'] },
    confused_with_kind: 'word', confused_with_id: 102, confused_with_text: '卖', confused_with_pinyin: 'mài',
    confused_with_translations: { en: ['to sell'] }, mode: 'zh_to_transl', count: 2,
    last_seen: new Date(Date.now() - 1000).toISOString(),
  },
  {
    zh_kind: 'word', zh_word_id: 103, zh_text: '已', zh_pinyin: 'yǐ', zh_translations: { en: ['already'] },
    confused_with_kind: 'word', confused_with_id: 104, confused_with_text: '己', confused_with_pinyin: 'jǐ',
    confused_with_translations: { en: ['self'] }, mode: 'transl_to_zh', count: 6,
    last_seen: new Date(Date.now() - 3 * DAY).toISOString(),
  },
];

// The e2e DB has no HMM library, so the four library endpoints are mocked.
// PUT/DELETE calls are answered with 200 so auto-save works.
const HMM = {
  actors: [
    { initial: 'b', category: 'male', actor_name: 'Bruce Lee', hint: 'Someone you know' },
    { initial: 'p', category: 'male', actor_name: '', hint: 'Someone you know' },
    { initial: 'm', category: 'female', actor_name: 'Marie Curie', hint: 'Someone you know' },
    { initial: 'null', category: 'wildcard', actor_name: '', hint: 'Someone you know' },
  ],
  locations: [
    { final_key: 'a', location_name: 'Grandma’s house' },
    { final_key: 'o', location_name: '' },
    { final_key: 'null', location_name: '' },
  ],
  'tone-rooms': [1, 2, 3, 4, 5].map(tone => ({ tone, room_name: tone === 1 ? 'Outside the entrance' : '' })),
  props: [
    { radical: '木', prop_name: 'wooden spoon' },
    { radical: '水', prop_name: '' },
  ],
};

async function mockHmm(page) {
  for (const [name, body] of Object.entries(HMM)) {
    await page.route(`**/api/hmm/${name}**`, r => r.request().method() === 'GET'
      ? r.fulfill({ json: body })
      : r.fulfill({ json: {} }));
  }
}

test.describe('Mismatches redesign', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('pairs render as cards, sorted by count, and can be sorted by recency', async ({ page }) => {
    await page.route('**/api/mismatches', r => r.fulfill({ json: PAIRS }));
    await page.goto('/mismatches');

    const rows = page.locator('#mismatches-list .mm-row');
    await expect(rows).toHaveCount(2, { timeout: 8_000 });
    await expect(page.locator('#mm-sort-often')).toHaveAttribute('aria-pressed', 'true');
    await expect(rows.first()).toContainText('已');
    await expect(rows.first().locator('.mm-count')).toHaveText('6× confused');
    await expect(rows.first().locator('.mm-count')).toHaveClass(/is-high/);
    await expect(rows.nth(1).locator('.mm-count')).not.toHaveClass(/is-high/);
    await expect(rows.first().locator('.mm-mode')).toBeVisible();
    await captureForPR(page, 'mismatches-cards');

    await page.locator('#mm-sort-recent').click();
    await expect(page.locator('#mm-sort-recent')).toHaveAttribute('aria-pressed', 'true');
    await expect(rows.first()).toContainText('买');
  });

  test('no pairs shows the empty state with a link to training', async ({ page }) => {
    await page.route('**/api/mismatches', r => r.fulfill({ json: [] }));
    await page.goto('/mismatches');
    await expect(page.locator('#empty-state')).toBeVisible({ timeout: 8_000 });
    await expect(page.locator('#empty-state a[href="/train"]')).toBeVisible();
    await expect(page.locator('#mm-sort')).toBeHidden();
    await captureForPR(page, 'mismatches-empty');
  });
});

test.describe('Mnemonics redesign', () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test('tabs switch sections and show filled counts', async ({ page }) => {
    await mockHmm(page);
    await page.goto('/mnemonics');
    await expect(page.locator('#actors-container .mn-item').first()).toBeVisible({ timeout: 10_000 });
    await expect(page.locator('#mn-tab-actors')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#mn-tab-actors .mn-tab-count')).toHaveText('2/4');
    await expect(page.locator('#actors-container .mn-group')).toHaveCount(3);
    await expect(page.locator('#actors-container .mn-group-title').first()).toBeVisible();
    await captureForPR(page, 'mnemonics-actors');

    await page.locator('#mn-tab-rooms').click();
    await expect(page.locator('#mn-panel-rooms')).toBeVisible();
    await expect(page.locator('#mn-panel-actors')).toBeHidden();
    await expect(page.locator('#tonerooms-container .mn-item')).toHaveCount(5);

    await page.locator('#mn-tab-props').click();
    await expect(page.locator('#add-prop-btn')).toBeVisible();
    await captureForPR(page, 'mnemonics-props');
  });

  test('typing a location updates the filled count and saves', async ({ page }) => {
    await mockHmm(page);
    const saved = [];
    page.on('request', req => { if (req.method() === 'PUT') saved.push(req.url()); });
    await page.goto('/mnemonics');
    await page.locator('#mn-tab-locations').click();
    const count = page.locator('#mn-tab-locations .mn-tab-count');
    await expect(count).toHaveText('1/3', { timeout: 10_000 });
    const input = page.locator('#locations-container .mn-item input').nth(1);
    await input.fill('School yard');
    await expect(count).toHaveText('2/3');
    await input.blur();
    await expect.poll(() => saved.at(-1)).toContain('/api/hmm/locations/o');
  });
});

test.describe('Library redesign – phone', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('mismatch cards and mnemonics tabs fit a phone', async ({ page }) => {
    await page.route('**/api/mismatches', r => r.fulfill({ json: PAIRS }));
    await page.goto('/mismatches');
    await expect(page.locator('#mismatches-list .mm-row')).toHaveCount(2, { timeout: 8_000 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await captureForPR(page, 'mismatches-phone');

    await mockHmm(page);
    await page.goto('/mnemonics');
    await expect(page.locator('#actors-container .mn-item').first()).toBeVisible({ timeout: 10_000 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await captureForPR(page, 'mnemonics-phone');
  });
});
