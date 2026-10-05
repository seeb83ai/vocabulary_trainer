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

// ── Keyboard reveal (issue #512) ───────────────────────────────────────────
// Mirrors scrollDeltaToReveal in train-card.js.

function scrollDeltaToReveal(top, bottom, visibleHeight, margin = 12) {
  const limit = visibleHeight - margin;
  if (bottom <= limit) return 0;
  return Math.max(0, Math.min(bottom - limit, top - margin));
}

describe('scrollDeltaToReveal', () => {
  it('does not scroll when the element already fits in the visible area', () => {
    expect(scrollDeltaToReveal(100, 250, 330)).toBe(0);
  });

  it('scrolls down just far enough to reveal the bottom edge plus margin', () => {
    expect(scrollDeltaToReveal(300, 413, 330)).toBe(95);
  });

  it('never scrolls the top edge out of view when the element is taller than the area', () => {
    expect(scrollDeltaToReveal(120, 700, 330)).toBe(108);
  });

  it('never scrolls upward', () => {
    expect(scrollDeltaToReveal(-50, 700, 330)).toBe(0);
  });
});

// ── Enter key continues (issue #535) ─────────────────────────────────────────
// Mirrors enterActionButtonId in train-card.js.

function enterActionButtonId(screens, focusedTag) {
  if (['INPUT', 'TEXTAREA', 'SELECT'].includes(focusedTag)) return null;
  if (screens.celebration) return 'celebration-continue-btn';
  if (screens.result) return 'next-btn';
  if (screens.newComponent) return 'new-component-got-it-btn';
  if (screens.newWord) return 'new-word-got-it-btn';
  if (screens.error) return 'error-retry-btn';
  return null;
}

describe('enterActionButtonId', () => {
  it('continues from the result screen', () => {
    expect(enterActionButtonId({ result: true }, 'BODY')).toBe('next-btn');
  });

  it('continues from the result screen when another button has focus', () => {
    expect(enterActionButtonId({ result: true }, 'BUTTON')).toBe('next-btn');
  });

  it('leaves Enter to a focused text field', () => {
    expect(enterActionButtonId({ result: true }, 'INPUT')).toBeNull();
    expect(enterActionButtonId({ result: true }, 'TEXTAREA')).toBeNull();
    expect(enterActionButtonId({ result: true }, 'SELECT')).toBeNull();
  });

  it('prefers the celebration screen over the result screen', () => {
    expect(enterActionButtonId({ celebration: true, result: true }, 'BODY')).toBe('celebration-continue-btn');
  });

  it('acknowledges a new word or a new component', () => {
    expect(enterActionButtonId({ newWord: true }, 'BODY')).toBe('new-word-got-it-btn');
    expect(enterActionButtonId({ newComponent: true }, 'BODY')).toBe('new-component-got-it-btn');
  });

  it('retries from the error card', () => {
    expect(enterActionButtonId({ error: true }, 'BODY')).toBe('error-retry-btn');
  });

  it('does nothing on the question card or the all-done screen', () => {
    expect(enterActionButtonId({}, 'BODY')).toBeNull();
  });
});
