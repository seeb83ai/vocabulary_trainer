import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ── Match-game outcome (issue #215) ────────────────────────────────────────────
// Mirrors the decision logic in the right-box click handler in showMatchGame.

function matchGameOutcome(rightIdx, lIdx, rightText, leftTransls, matchedLeftIdxs) {
  if (rightIdx === lIdx) return 'correct';
  if (leftTransls.includes(rightText)) {
    return matchedLeftIdxs.has(rightIdx) ? 'correct' : 'blocked';
  }
  return 'wrong';
}

// ── Match-game translation shortening (issue #428) ─────────────────────────
// Mirrors isNoise from train-answer.js (Bsp.:/CL:/ZEW: annotation segments
// aren't real meanings) and shortenMatchGameTranslation in train-matchgame.js.

function isNoise(text) {
  return /^(CL:|Bsp\.:|ZEW:)/.test(text);
}

const MATCH_GAME_SHORT_MEANING_MAX_CHARS = 20;

function shortenMatchGameTranslation(text) {
  const parts = text.split(';').map(s => s.trim()).filter(s => s && !isNoise(s));
  if (parts.length === 0) return text.trim();
  if (parts.length === 1) return parts[0];
  if (parts[0].length <= MATCH_GAME_SHORT_MEANING_MAX_CHARS &&
      parts[1].length <= MATCH_GAME_SHORT_MEANING_MAX_CHARS) {
    return `${parts[0]}; ${parts[1]}`;
  }
  return parts[0];
}

describe('shortenMatchGameTranslation', () => {
  it('leaves a single short meaning untouched', () => {
    expect(shortenMatchGameTranslation('cat')).toBe('cat');
  });

  it('joins two meanings when both are short', () => {
    expect(shortenMatchGameTranslation('near; close')).toBe('near; close');
  });

  it('keeps only the first meaning when the second is long', () => {
    expect(shortenMatchGameTranslation('near; in the immediate vicinity of something')).toBe('near');
  });

  it('drops Bsp./CL/ZEW annotation segments and keeps real meanings', () => {
    const raw = 'nah (Adj); in der Nähe (S); Bsp.: 附近 附近 -- in der Nähe befindlich; in der Nachbarschaft; Bsp.: 靠近些。 靠近些。 -- Komm etw. näher!';
    expect(shortenMatchGameTranslation(raw)).toBe('nah (Adj); in der Nähe (S)');
  });

  it('returns the trimmed text unchanged when there is nothing to split', () => {
    expect(shortenMatchGameTranslation('  benachbart  ')).toBe('benachbart');
  });
});

// ── Match-game noise-entry skipping (issue #429) ────────────────────────────
// Mirrors pickMatchGameTranslationText in train-matchgame.js.

function pickMatchGameTranslationText(translations, fallbackText) {
  for (const texts of Object.values(translations || {})) {
    const clean = (texts || []).filter(t => !isNoise(t));
    if (clean.length > 0) return shortenMatchGameTranslation(clean[0]);
  }
  return fallbackText;
}

describe('pickMatchGameTranslationText', () => {
  it('skips a leading Bsp. example-sentence entry and returns the next real meaning', () => {
    expect(pickMatchGameTranslationText({ de: ['Bsp.: 附近 -- in der Nähe befindlich', 'nah (Adj)'] }, '近'))
      .toBe('nah (Adj)');
  });

  it('skips a leading CL: measure-word entry', () => {
    expect(pickMatchGameTranslationText({ en: ['CL:個|个[ge4]', 'book'] }, '书'))
      .toBe('book');
  });

  it('skips a leading ZEW: measure-word entry', () => {
    expect(pickMatchGameTranslationText({ de: ['ZEW: 個|个[ge4]', 'Buch'] }, '书'))
      .toBe('Buch');
  });

  it('falls back to the zh text when every translation in every language is noise', () => {
    expect(pickMatchGameTranslationText({ de: ['Bsp.: nur ein Beispiel'] }, '近')).toBe('近');
  });

  it('moves on to the next language when the first language has only noise', () => {
    expect(pickMatchGameTranslationText({ en: ['CL:個|个[ge4]'], de: ['nah (Adj)'] }, '近')).toBe('nah (Adj)');
  });

  it('still shortens a long multi-meaning translation after skipping noise entries', () => {
    expect(pickMatchGameTranslationText(
      { de: ['Bsp.: skip me', 'nah (Adj); in der Nähe (S); in der Nachbarschaft; kürzlich (Adj)'] }, '近',
    )).toBe('nah (Adj); in der Nähe (S)');
  });
});

describe('matchGameOutcome', () => {
  it('is correct when the right box is the word\'s own translation', () => {
    expect(matchGameOutcome(0, 0, 'können', ['in der Lage sein', 'können'], new Set())).toBe('correct');
  });

  it('is wrong when the right text is not among the word\'s translations', () => {
    expect(matchGameOutcome(1, 0, 'möglicherweise', ['in der Lage sein', 'können'], new Set())).toBe('wrong');
  });

  it('is blocked when a shared translation is claimed but its true owner still needs it (issue #215)', () => {
    // 能 (lIdx=0) picks "können", which is 可能's (rightIdx=1) own box — 可能 not yet matched.
    expect(matchGameOutcome(1, 0, 'können', ['in der Lage sein', 'können'], new Set())).toBe('blocked');
  });

  it('is correct via shared translation once the true owner is already matched', () => {
    // 可能 (rightIdx=1) already matched elsewhere, so 能 (lIdx=0) may safely reuse "können".
    expect(matchGameOutcome(1, 0, 'können', ['in der Lage sein', 'können'], new Set([1]))).toBe('correct');
  });

  it('is correct via shared translation when two words have the exact same single translation', () => {
    expect(matchGameOutcome(1, 0, 'hello', ['hello'], new Set())).toBe('blocked');
    expect(matchGameOutcome(1, 0, 'hello', ['hello'], new Set([1]))).toBe('correct');
  });
});
