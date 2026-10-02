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
