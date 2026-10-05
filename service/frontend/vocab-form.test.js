import { describe, it, expect, beforeEach } from 'vitest';

// ── buildFormPayload (DOM-based) ───────────────────────────────────────────────
// Simulate the DOM structure that vocab.html provides.

function buildFormPayload(zhValue, pinyinValue, enValues, deValues = [], tags = [], startTraining = false) {
  // Mirrors the vocab.js buildFormPayload logic
  return {
    zh_text: zhValue.trim(),
    pinyin: pinyinValue.trim(),
    translations: {
      en: enValues.map(v => v.trim()).filter(Boolean),
      de: deValues.map(v => v.trim()).filter(Boolean),
    },
    tags: [...tags],
    start_training: startTraining,
  };
}

describe('buildFormPayload', () => {
  it('trims whitespace from zh_text', () => {
    const p = buildFormPayload('  你好  ', '', ['hello']);
    expect(p.zh_text).toBe('你好');
  });

  it('trims whitespace from pinyin', () => {
    const p = buildFormPayload('你好', '  nǐ hǎo  ', ['hello']);
    expect(p.pinyin).toBe('nǐ hǎo');
  });

  it('filters empty en translations', () => {
    const p = buildFormPayload('你好', '', ['hello', '  ', '']);
    expect(p.translations.en).toEqual(['hello']);
  });

  it('allows multiple en translations', () => {
    const p = buildFormPayload('你好', '', ['hello', 'hi', 'hey']);
    expect(p.translations.en).toHaveLength(3);
  });

  it('returns empty pinyin when not provided', () => {
    const p = buildFormPayload('你好', '', ['hello']);
    expect(p.pinyin).toBe('');
  });

  it('includes tags array', () => {
    const p = buildFormPayload('你好', '', ['hello'], [], ['HSK1', 'greetings']);
    expect(p.tags).toEqual(['HSK1', 'greetings']);
  });

  it('defaults to empty tags', () => {
    const p = buildFormPayload('你好', '', ['hello']);
    expect(p.tags).toEqual([]);
  });

  it('defaults start_training to false', () => {
    const p = buildFormPayload('你好', '', ['hello']);
    expect(p.start_training).toBe(false);
  });

  it('includes start_training when true', () => {
    const p = buildFormPayload('你好', '', ['hello'], [], [], true);
    expect(p.start_training).toBe(true);
  });
});

// ── libraryDiffLines ─────────────────────────────────────────────────────────

function libraryDiffLines(diff) {
  const lines = [];
  for (const kind of ['remove', 'restore']) {
    for (const lang of Object.keys(diff?.[kind] || {}).sort()) {
      const texts = diff[kind][lang] || [];
      if (texts.length) lines.push({ kind, lang, texts });
    }
  }
  if (diff?.pinyin) lines.push({ kind: 'pinyin', mine: diff.pinyin.mine, library: diff.pinyin.library });
  return lines;
}

describe('libraryDiffLines', () => {
  it('lists removed glosses, then glosses that come back, then the pinyin', () => {
    const diff = {
      remove: { en: ['human'], de: ['Mensch!'] },
      restore: { en: ['people'] },
      pinyin: { mine: 'ren2', library: 'rén' },
    };
    expect(libraryDiffLines(diff)).toEqual([
      { kind: 'remove', lang: 'de', texts: ['Mensch!'] },
      { kind: 'remove', lang: 'en', texts: ['human'] },
      { kind: 'restore', lang: 'en', texts: ['people'] },
      { kind: 'pinyin', mine: 'ren2', library: 'rén' },
    ]);
  });

  it('is empty when a reset would change nothing', () => {
    expect(libraryDiffLines({ remove: {}, restore: { en: [] } })).toEqual([]);
    expect(libraryDiffLines(null)).toEqual([]);
  });
});
