// @ts-check
// Rich demo data shared by the README (capture.spec.js) and landing-page
// (landing.spec.js) screenshot specs. Words go in through the REST API; history,
// SM-2 spread and HMM/pinyin rows go in through the test-only Go seed tools
// (see service/cmd/e2e-seed-*), because no endpoint can back-date them.
import { expect } from '@playwright/test';
import { execSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { TEST_EMAIL } from '../e2e/global-setup.js';

/** [zh, pinyin, en[], de[], tag] */
export const WORDS = [
  ['超市', 'chāoshì', ['supermarket'], ['Supermarkt'], 'hsk1'],
  ['城市', 'chéngshì', ['city'], ['Stadt'], 'hsk1'],
  ['号', 'hào', ['date', 'number'], ['Datum', 'Nummer'], 'hsk1'],
  ['行', 'xíng', ['OK', 'all right'], ['in Ordnung'], 'hsk1'],
  ['介绍', 'jièshào', ['introduce'], ['vorstellen'], 'hsk1'],
  ['不客气', 'búkèqi', ['you are welcome'], ['gern geschehen'], 'hsk1'],
  ['想', 'xiǎng', ['want', 'would like'], ['wollen', 'möchten'], 'hsk1'],
  ['准备', 'zhǔnbèi', ['prepare'], ['vorbereiten'], 'hsk1'],
  ['不好意思', 'bùhǎoyìsi', ['sorry'], ['Entschuldigung'], 'hsk1'],
  ['知道', 'zhīdào', ['know'], ['wissen'], 'hsk1'],
  ['时候', 'shíhou', ['time'], ['Zeit'], 'hsk2'],
  ['发烧', 'fāshāo', ['have a fever'], ['Fieber haben'], 'hsk2'],
  ['放心', 'fàngxīn', ['do not worry'], ['Mach dir keine Sorgen'], 'hsk2'],
  ['附近', 'fùjìn', ['nearby'], ['in der Nähe'], 'hsk2'],
  ['刚才', 'gāngcái', ['just now'], ['gerade eben'], 'hsk2'],
  ['打算', 'dǎsuàn', ['plan'], ['planen'], 'hsk2'],
  ['学生', 'xuésheng', ['student'], ['Student'], 'hsk2'],
  ['老师', 'lǎoshī', ['teacher'], ['Lehrer'], 'hsk2'],
  ['朋友', 'péngyou', ['friend'], ['Freund'], 'hsk2'],
  ['水', 'shuǐ', ['water'], ['Wasser'], 'hsk2'],
  ['茶', 'chá', ['tea'], ['Tee'], 'hsk2'],
  ['米饭', 'mǐfàn', ['rice'], ['Reis'], 'hsk2'],
  ['苹果', 'píngguǒ', ['apple'], ['Apfel'], 'hsk2'],
  ['医院', 'yīyuàn', ['hospital'], ['Krankenhaus'], 'hsk2'],
  ['火车', 'huǒchē', ['train'], ['Zug'], 'hsk2'],
  ['银行', 'yínháng', ['bank'], ['Bank'], 'hsk2'],
  ['电脑', 'diànnǎo', ['computer'], ['Computer'], 'hsk2'],
  ['天气', 'tiānqì', ['weather'], ['Wetter'], 'hsk2'],
  ['漂亮', 'piàoliang', ['beautiful'], ['schön'], 'hsk2'],
  ['忙', 'máng', ['busy'], ['beschäftigt'], 'hsk2'],
];

/** Add the demo words on top of the 3 words e2e/global-setup.js seeds. */
export async function seedWords(page) {
  for (const [zh, pinyin, en, de, tag] of WORDS) {
    const res = await page.request.post('/api/words', {
      data: { zh_text: zh, pinyin, translations: { en, de }, tags: [tag], start_training: true },
    });
    if (!res.ok()) throw new Error(`seed ${zh}: ${res.status()} ${await res.text()}`);
  }
}

/**
 * Run one of the test-only e2e-seed-* Go CLIs against the running E2E server's
 * temp DB.
 * @param {string} tool
 * @param {string} [extraArgs]
 */
export function runGoSeedTool(tool, extraArgs = '') {
  const { dbPath } = JSON.parse(readFileSync(join('e2e', '.state', 'server.json'), 'utf8'));
  execSync(`go run ./cmd/${tool} -db "${dbPath}" -email ${TEST_EMAIL} ${extraArgs}`, {
    cwd: 'service',
    stdio: 'inherit',
  });
}

/**
 * Record wrong answers that are another word's meaning, so /mismatches has
 * confusion pairs. Call after seedWords.
 */
export async function seedConfusions(page) {
  const { words } = await (await page.request.get('/api/words?per_page=200')).json();
  const byZh = Object.fromEntries(words.map(w => [w.zh_text, w]));
  const pairs = [['超市', '城市'], ['城市', '超市'], ['你好', '谢谢'], ['谢谢', '你好'], ['再见', '你好'],
    ['想', '打算'], ['刚才', '附近'], ['老师', '学生'], ['朋友', '老师'], ['水', '茶']];
  for (const [a, b] of pairs) {
    const res = await page.request.post('/api/quiz/answer', {
      data: { word_id: byZh[a].id, mode: 'zh_to_transl', answer: byZh[b].translations.en[0], langs: ['en', 'de'] },
    });
    expect(res.ok()).toBe(true);
  }
}

export async function settings(page) {
  return (await page.request.get('/api/settings')).json();
}
export async function patchSettings(page, patch) {
  const res = await page.request.patch('/api/settings', { data: { ...(await settings(page)), ...patch } });
  expect(res.ok()).toBe(true);
}
/** Select the quiz direction for both the filter and the new-word ladder. */
export async function useMode(page, mode) {
  await page.request.patch('/api/training-filters', {
    data: { mode, langs: ['en', 'de'], bucket: '', mnemonics: true, components: false, tags: [] },
  });
  await patchSettings(page, { new_word_mode_0: mode, new_word_mode_1: mode, new_word_mode_2: mode });
}
/** Re-seed so only `zh` is due, then open the Train page on its card. */
export async function openCard(page, zh, mode) {
  runGoSeedTool('e2e-seed-history', `-focus ${zh}`);
  await useMode(page, mode);
  await page.goto('/train');
  await expect(page.locator('#card-area')).toBeVisible({ timeout: 12_000 });
  await expect(page.locator('#prompt-word, #new-word-zh').first()).toHaveText(zh);
}
/** Type `text` on the open Train card and submit; resolves once the result shows. */
export async function answer(page, text) {
  await page.locator('#answer-input').fill(text);
  await page.locator('#answer-form button[type="submit"]').click();
  await expect(page.locator('#result-area')).toBeVisible({ timeout: 8_000 });
}
