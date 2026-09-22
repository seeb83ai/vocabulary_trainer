import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ── Answer input placeholder ───────────────────────────────────────────────
// Mirrors the i18n key selection in showCard(): sentence fill-in-the-blank
// cards get a placeholder telling the user to type the missing word;
// every other card type keeps the generic answer placeholder.

function placeholderKeyForCard(cardType) {
  return cardType === 'sentence' ? 'card.placeholderSentence' : 'card.placeholder';
}

describe('placeholderKeyForCard', () => {
  it('uses the sentence-specific placeholder key for sentence cards', () => {
    expect(placeholderKeyForCard('sentence')).toBe('card.placeholderSentence');
  });

  it('uses the generic placeholder key for component cards', () => {
    expect(placeholderKeyForCard('component')).toBe('card.placeholder');
  });

  it('uses the generic placeholder key for hmm cards', () => {
    expect(placeholderKeyForCard('hmm')).toBe('card.placeholder');
  });

  it('uses the generic placeholder key for word cards', () => {
    expect(placeholderKeyForCard('word')).toBe('card.placeholder');
  });
});

// ── Answer submission state machine helpers ───────────────────────────────────
// These mirror the guard logic in submitAnswer.

function canSubmit(isSubmitted, currentCard) {
  return !isSubmitted && currentCard !== null;
}

describe('submitAnswer guard', () => {
  it('allows submit when not yet submitted and card is loaded', () => {
    expect(canSubmit(false, { word_id: 1 })).toBe(true);
  });

  it('prevents double-submit', () => {
    expect(canSubmit(true, { word_id: 1 })).toBe(false);
  });

  it('prevents submit with no card loaded', () => {
    expect(canSubmit(false, null)).toBe(false);
  });
});

// Issue #466: hint that tells look-alike characters apart (囗 vs 口) when the
// pinyin is hidden. Inlined from train-card.js.
function formatLookalikeHint(lookalikes, langs) {
  if (!lookalikes || !lookalikes.length) return '';
  return lookalikes.map(l => {
    const defs = l.definitions || {};
    const lang = langs.find(g => defs[g]);
    const gloss = lang ? defs[lang].split(';')[0].trim() : '';
    return gloss ? `≠ ${l.character} (${gloss})` : `≠ ${l.character}`;
  }).join(' · ');
}

describe('formatLookalikeHint', () => {
  it('returns empty string without look-alikes', () => {
    expect(formatLookalikeHint(undefined, ['en'])).toBe('');
    expect(formatLookalikeHint([], ['en'])).toBe('');
  });

  it('shows the character with its first gloss', () => {
    expect(formatLookalikeHint([{ character: '口', definitions: { en: 'mouth; opening' } }], ['en']))
      .toBe('≠ 口 (mouth)');
  });

  it('uses the first selected lang that has a definition', () => {
    const l = [{ character: '口', definitions: { en: 'mouth', de: 'Mund' } }];
    expect(formatLookalikeHint(l, ['de', 'en'])).toBe('≠ 口 (Mund)');
    expect(formatLookalikeHint([{ character: '口', definitions: { en: 'mouth' } }], ['de', 'en'])).toBe('≠ 口 (mouth)');
  });

  it('shows only the character when no definition exists', () => {
    expect(formatLookalikeHint([{ character: '口' }], ['en'])).toBe('≠ 口');
  });

  it('joins several look-alikes', () => {
    const l = [{ character: '已', definitions: { en: 'already' } }, { character: '巳', definitions: {} }];
    expect(formatLookalikeHint(l, ['en'])).toBe('≠ 已 (already) · ≠ 巳');
  });
});
