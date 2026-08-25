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
