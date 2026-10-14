import { describe, it, expect } from 'vitest';

// ── normalizeSentenceTag / sentenceWordPayload ───────────────────────────────
// Inlined from vocab-sentences.js. A sentence tag must end in "_s" so
// fill-in-the-blank finds the sentence.

function normalizeSentenceTag(name) {
  const tag = String(name || '').trim().replace(/\s+/g, '-');
  if (!tag) return '';
  return tag.endsWith('_s') ? tag : tag + '_s';
}

function sentenceWordPayload(sentence, tag, startTraining) {
  const translations = {};
  for (const [lang, list] of Object.entries(sentence.translations || {})) {
    const clean = list.map(s => s.trim()).filter(Boolean);
    if (clean.length) translations[lang] = clean;
  }
  return {
    zh_text: sentence.zh_text.trim(),
    pinyin: (sentence.pinyin || '').trim(),
    translations,
    tags: [tag],
    start_training: !!startTraining,
  };
}

describe('normalizeSentenceTag', () => {
  it('adds _s to a tag without it', () => {
    expect(normalizeSentenceTag('daily')).toBe('daily_s');
    expect(normalizeSentenceTag('hsk3-1')).toBe('hsk3-1_s');
  });

  it('keeps a tag that already ends in _s', () => {
    expect(normalizeSentenceTag('food_s')).toBe('food_s');
  });

  it('trims and joins words with "-"', () => {
    expect(normalizeSentenceTag('  small talk ')).toBe('small-talk_s');
  });

  it('returns "" for an empty name', () => {
    expect(normalizeSentenceTag('   ')).toBe('');
    expect(normalizeSentenceTag(undefined)).toBe('');
  });
});

describe('sentenceWordPayload', () => {
  it('builds a word request with the sentence tag', () => {
    const p = sentenceWordPayload(
      { zh_text: ' 我吃米饭。 ', pinyin: 'wǒ chī mǐ fàn.', translations: { en: ['I eat rice.'], de: ['Ich esse Reis.'] } },
      'daily_s',
    );
    expect(p).toEqual({
      zh_text: '我吃米饭。',
      pinyin: 'wǒ chī mǐ fàn.',
      translations: { en: ['I eat rice.'], de: ['Ich esse Reis.'] },
      tags: ['daily_s'],
      start_training: false,
    });
  });

  it('asks to start training when the box is checked', () => {
    const p = sentenceWordPayload({ zh_text: '我吃米饭。', pinyin: '', translations: { en: ['I eat rice.'] } }, 'x_s', true);
    expect(p.start_training).toBe(true);
  });

  it('drops empty translations the user cleared', () => {
    const p = sentenceWordPayload({ zh_text: '我吃米饭。', pinyin: '', translations: { en: ['I eat rice.'], de: ['  '] } }, 'x_s');
    expect(p.translations).toEqual({ en: ['I eat rice.'] });
  });
});
