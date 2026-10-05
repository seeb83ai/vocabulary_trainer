// @ts-check
import { test, expect } from '@playwright/test';
import { execSync } from 'node:child_process';
import { captureForPR } from './helpers/screenshot.js';

const PASSWORD = 'E2eImportPass123!';

test.describe('Vocabulary → Import', () => {
  test('imports several library lists at once and tags words the user already has', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const email = `e2e-import-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
    await page.goto('/#register');
    await page.locator('#reg-email').fill(email);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    await page.locator('#register-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    // The user already owns 时间 from the HSK 2.0 list.
    const res = await page.request.post('/api/import', { data: { tag: 'hsk2-2', apply_tags: ['hsk2-2'] } });
    expect(res.status()).toBe(202);
    const { id: setupJobId } = await res.json();
    await expect.poll(async () => {
      const job = await (await page.request.get(`/api/import/jobs/${setupJobId}`)).json();
      return job.status;
    }, { timeout: 15_000 }).toBe('done');

    await page.goto('/vocab');
    await page.locator('#open-import-btn').click();
    await page.locator('#import-tag-list button', { hasText: /^hsk3-2$/ }).click();
    await page.locator('#import-tag-list button', { hasText: /^hsk3-3$/ }).click();
    await expect(page.locator('#import-tag-list button[aria-pressed="true"]')).toHaveCount(2);
    // 大, 时间 + 已经
    await expect(page.locator('#import-preview-stats')).toContainText('3', { timeout: 10_000 });
    await captureForPR(page, 'vocab-import-multi-select');

    await page.locator('#import-next-btn').click();
    await page.locator('#import-next2-btn').click();
    await expect(page.locator('#import-apply-tags')).toContainText('hsk3-2');
    await expect(page.locator('#import-apply-tags')).toContainText('hsk3-3');
    await page.locator('#import-submit-btn').click();

    await expect(page.locator('#import-status')).toHaveClass(/text-green-600/, { timeout: 15_000 });
    await captureForPR(page, 'vocab-import-multi-select-done');

    const words = await page.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
    const tagsByWord = Object.fromEntries(words.words.map(w => [w.zh_text, w.tags]));
    expect(tagsByWord).toEqual({
      '大': ['hsk3-2'],
      '时间': ['hsk2-2', 'hsk3-2'],
      '已经': ['hsk2-2', 'hsk3-3'],
    });
  });

  test('shows live progress while the import runs and the result when it is done', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const email = `e2e-import-progress-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
    await page.goto('/#register');
    await page.locator('#reg-email').fill(email);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    await page.locator('#register-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    // The real import finishes in milliseconds on the test database, so the
    // first status polls report a job that is still running.
    let polls = 0;
    await page.route('**/api/import/jobs/*', route => {
      if (polls++ < 3) {
        route.fulfill({ json: { id: 1, tag: 'hsk3-2', status: 'running', total: 3, done: 1, imported: 1, tagged: 0, skipped: 0 } });
      } else {
        route.continue();
      }
    });

    await page.goto('/vocab');
    await page.locator('#open-import-btn').click();
    await page.locator('#import-tag-list button', { hasText: /^hsk3-2$/ }).click();
    await page.locator('#import-next-btn').click();
    await page.locator('#import-next2-btn').click();
    await page.locator('#import-submit-btn').click();

    await expect(page.locator('#import-status')).toContainText('1 of 3');
    await captureForPR(page, 'vocab-import-progress');
    await expect(page.locator('#import-status')).toHaveClass(/text-green-600/, { timeout: 15_000 });
    await expect(page.locator('#import-status')).toContainText('Imported');
  });

  test('imports library words whose translations follow the settings and whose edits stay private', async ({ page, browser }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const register = async (p, prefix) => {
      const email = `${prefix}-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
      await p.goto('/#register');
      await p.locator('#reg-email').fill(email);
      await p.locator('#reg-password').fill(PASSWORD);
      await p.locator('#reg-confirm').fill(PASSWORD);
      await p.locator('#register-btn').click();
      await expect(p).toHaveURL('/train', { timeout: 10_000 });
    };
    await register(page, 'e2e-import-ref');

    await page.goto('/vocab');
    await page.locator('#open-import-btn').click();
    await page.locator('#import-tag-list button', { hasText: /^hsk3-1$/ }).click();
    await page.locator('#import-next-btn').click();
    // No language checkboxes: the learner's primary/secondary language decide.
    await expect(page.locator('#import-langs-label')).toHaveText('Translations: EN + DE');
    await expect(page.locator('#import-langs-hint a')).toHaveAttribute('href', '/settings');
    await captureForPR(page, 'vocab-import-langs-hint');
    await page.locator('#import-next2-btn').click();
    await page.locator('#import-submit-btn').click();
    await expect(page.locator('#import-status')).toHaveClass(/text-green-600/, { timeout: 15_000 });

    const wordsOf = async p => {
      const res = await p.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
      return Object.fromEntries(res.words.map(w => [w.zh_text, w]));
    };
    const person = (await wordsOf(page))['人'];
    expect([...person.translations.en].sort()).toEqual(['people', 'person']);

    // The learner removes "people" from their 人.
    const put = await page.request.put(`/api/words/${person.id}`, {
      data: { zh_text: '人', pinyin: 'rén', translations: { en: ['person'] }, translation_sources: { en: ['cedict'] }, tags: person.tags },
    });
    expect(put.status()).toBe(200);
    expect((await wordsOf(page))['人'].translations.en).toEqual(['person']);

    // Another learner importing the same list still gets both meanings.
    const other = await browser.newContext();
    const otherPage = await other.newPage();
    await otherPage.route('https://api.pwnedpasswords.com/**', route => route.fulfill({ status: 200, body: '' }));
    await register(otherPage, 'e2e-import-ref-other');
    const res = await otherPage.request.post('/api/import', { data: { tag: 'hsk3-1', apply_tags: ['hsk3-1'] } });
    expect(res.status()).toBe(202);
    const { id: jobId } = await res.json();
    await expect.poll(async () => (await (await otherPage.request.get(`/api/import/jobs/${jobId}`)).json()).status,
      { timeout: 15_000 }).toBe('done');
    expect([...(await wordsOf(otherPage))['人'].translations.en].sort()).toEqual(['people', 'person']);
    await other.close();
  });

  test('resets an edited library word after showing what changes', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const email = `e2e-reset-library-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
    await page.goto('/#register');
    await page.locator('#reg-email').fill(email);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    await page.locator('#register-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });
    const res = await page.request.post('/api/import', { data: { tag: 'hsk3-1', apply_tags: ['hsk3-1'] } });
    expect(res.status()).toBe(202);
    const { id: jobId } = await res.json();
    await expect.poll(async () => (await (await page.request.get(`/api/import/jobs/${jobId}`)).json()).status,
      { timeout: 15_000 }).toBe('done');
    const wordsOf = async () => {
      const r = await page.evaluate(() => fetch('/api/words/?per_page=50').then(x => x.json()));
      return Object.fromEntries(r.words.map(w => [w.zh_text, w]));
    };

    // The learner deletes "people", adds "human" and changes the pinyin of 人.
    const person = (await wordsOf())['人'];
    const put = await page.request.put(`/api/words/${person.id}`, {
      data: {
        zh_text: '人', pinyin: 'ren2',
        translations: { ...person.translations, en: ['person', 'human'] },
        translation_sources: { ...person.translation_sources, en: ['cedict', 'user'] },
        tags: person.tags,
      },
    });
    expect(put.status()).toBe(200);

    await page.goto('/vocab');
    // Imported words are unseen, and the list hides those by default.
    await page.locator('#hide-unseen-btn').click();
    await expect(page.locator('#hide-unseen-btn')).toHaveAttribute('aria-pressed', 'false');
    // A library word the learner did not change has no reset.
    const other = Object.keys(await wordsOf()).find(zh => zh !== '人');
    await page.locator('#words-tbody .vb-row', { hasText: other }).click();
    await expect(page.locator('#form-library-reset-btn')).toBeHidden();
    await page.locator('#form-cancel-btn').click();

    await page.locator('#words-tbody .vb-row', { hasText: '人' }).click();
    await page.locator('#form-library-reset-btn').click();
    const box = page.locator('#library-reset-confirm');
    await expect(box).toBeVisible();
    await expect(box).toContainText('Removes (EN): human');
    await expect(box).toContainText('Brings back (EN): people');
    await expect(box).toContainText('Pinyin: ren2 → rén');
    await captureForPR(page, 'vocab-edit-reset-to-library');
    await page.locator('#library-reset-apply').click();

    await expect(box).toBeHidden();
    await expect(page.locator('#form-pinyin')).toHaveValue('rén');
    await expect(page.locator('#form-library-reset-btn')).toBeHidden();
    const after = (await wordsOf())['人'];
    expect([...after.translations.en].sort()).toEqual(['people', 'person']);
    expect(after.pinyin).toBe('rén');
  });

  // The library changes only through the CLI tools (ADR-0005), so this test
  // runs them against the E2E database, on its own list topic-e2esync.
  test('updates an imported list and adds removed words again', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const email = `e2e-import-sync-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
    await page.goto('/#register');
    await page.locator('#reg-email').fill(email);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    await page.locator('#register-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    const res = await page.request.post('/api/import', { data: { tag: 'topic-e2esync', apply_tags: ['topic-e2esync'] } });
    expect(res.status()).toBe(202);
    const { id: jobId } = await res.json();
    await expect.poll(async () => (await (await page.request.get(`/api/import/jobs/${jobId}`)).json()).status,
      { timeout: 15_000 }).toBe('done');
    const wordsOf = async () => {
      const r = await page.evaluate(() => fetch('/api/words/?per_page=50').then(x => x.json()));
      return Object.fromEntries(r.words.map(w => [w.zh_text, w]));
    };
    let words = await wordsOf();
    // The learner adds a meaning to 椅子 and deletes 桌子.
    expect((await page.request.post(`/api/words/${words['椅子'].id}/translations`, { data: { text: 'seat', lang: 'en' } })).status()).toBe(204);
    expect((await page.request.delete(`/api/words/${words['桌子'].id}`)).status()).toBe(204);

    // Later the library adds 门 to the list and a new sense to 椅子.
    await page.waitForTimeout(1100);
    const cli = cmd => execSync(`cd service && go run ${cmd}`, { stdio: 'inherit', cwd: process.cwd() });
    const db = process.env.E2E_DB_PATH;
    cli(`./cmd/import-topics -db ${db} -dir ../e2e/fixtures/library-update/topics`);
    // -append: the fixture is a partial file. import-cedict refreshes the library.
    cli(`./cmd/import-cedict -db ${db} -file ../e2e/fixtures/library-update/cedict-update.u8 -lang en -append`);

    await page.goto('/vocab');
    await page.locator('#open-import-btn').click();
    const row = page.locator('#import-my-lists-rows li', { hasText: 'topic-e2esync' });
    await expect(row).toContainText('1 new · Update');
    await expect(row).toContainText('1 removed earlier · Include again');
    await expect(row).not.toContainText('changed in the library');
    await captureForPR(page, 'vocab-import-my-lists');

    // The library's new sense reaches 椅子 by itself, next to the learner's own.
    expect([...(await wordsOf())['椅子'].translations.en].sort()).toEqual(['chair', 'seat', 'stool']);

    await row.getByText('1 new · Update').click();
    await expect(row).toContainText('Up to date', { timeout: 15_000 });
    await row.getByText('1 removed earlier · Include again').click();
    await expect(row).not.toContainText('removed earlier', { timeout: 15_000 });
    words = await wordsOf();
    expect(Object.keys(words).sort()).toEqual(['椅子', '桌子', '门'].sort());
  });

  test('words added by hand or by CSV that are in the dictionary get later dictionary updates', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const email = `e2e-ondemand-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
    await page.goto('/#register');
    await page.locator('#reg-email').fill(email);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    await page.locator('#register-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    const add = await page.request.post('/api/words/', {
      data: { zh_text: '窗', pinyin: 'chuāng', translations: { en: ['window'] }, tags: ['mine'] },
    });
    expect(add.status()).toBe(201);
    const csv = await page.request.post('/api/words/upload-csv', {
      multipart: {
        tags: 'mine',
        start_training_count: '0',
        file: { name: 'words.csv', mimeType: 'text/csv', buffer: Buffer.from('chinese,pinyin,en\n墙,qiáng,wall\n') },
      },
    });
    expect(csv.status()).toBe(200);

    const db = process.env.E2E_DB_PATH;
    execSync(`cd service && go run ./cmd/import-cedict -db ${db} -file ../e2e/fixtures/library-update/cedict-update-ondemand.u8 -lang en -append`, { stdio: 'inherit' });

    const res = await page.evaluate(() => fetch('/api/words/?per_page=50').then(r => r.json()));
    const en = Object.fromEntries(res.words.map(w => [w.zh_text, [...w.translations.en].sort()]));
    expect(en).toEqual({ '窗': ['casement', 'window'], '墙': ['partition', 'wall'] });
  });

  test('lists imported before import jobs existed show up in Your lists', async ({ page }) => {
    await page.route('https://api.pwnedpasswords.com/**', route => {
      route.fulfill({ status: 200, body: '' });
    });
    const email = `e2e-old-list-${Date.now()}-${Math.floor(Math.random() * 1e6)}@test.local`;
    await page.goto('/#register');
    await page.locator('#reg-email').fill(email);
    await page.locator('#reg-password').fill(PASSWORD);
    await page.locator('#reg-confirm').fill(PASSWORD);
    await page.locator('#register-btn').click();
    await expect(page).toHaveURL('/train', { timeout: 10_000 });

    // A word carrying the list tag, without any import job (as older imports left it).
    const add = await page.request.post('/api/words/', {
      data: { zh_text: '一', pinyin: 'yī', translations: { en: ['one'] }, translation_sources: { en: ['cedict'] }, tags: ['hsk3-1'] },
    });
    expect(add.status()).toBe(201);

    await page.goto('/vocab');
    await page.locator('#open-import-btn').click();
    const row = page.locator('#import-my-lists-rows li', { hasText: 'hsk3-1' });
    await expect(row).toContainText('1 new · Update');
    await row.getByText('1 new · Update').click();
    await expect(row).toContainText('Up to date', { timeout: 15_000 });
  });
});
