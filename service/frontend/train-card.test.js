import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

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

// ── Card image eligibility ─────────────────────────────────────────────────
// Mirrors cardImageEligible in train-card.js.

function cardImageEligible(currentCard, showImagesWithChineseText, imagesConfigured) {
  if (!showImagesWithChineseText || !imagesConfigured) return false;
  if (!currentCard || !currentCard.word_id) return false;
  if (currentCard.card_type) return false;
  if (currentCard.mode === 'new_word') return false;
  return true;
}

describe('cardImageEligible', () => {
  it('is eligible for a plain word card when setting on and feature configured', () => {
    expect(cardImageEligible({ word_id: 1, mode: 'zh_to_transl' }, true, true)).toBe(true);
  });

  it('is ineligible when the setting is off', () => {
    expect(cardImageEligible({ word_id: 1, mode: 'zh_to_transl' }, false, true)).toBe(false);
  });

  it('is ineligible when the server feature is not configured', () => {
    expect(cardImageEligible({ word_id: 1, mode: 'zh_to_transl' }, true, false)).toBe(false);
  });

  it('is ineligible with no current card', () => {
    expect(cardImageEligible(null, true, true)).toBe(false);
  });

  it('is ineligible for component cards', () => {
    expect(cardImageEligible({ word_id: 1, card_type: 'component' }, true, true)).toBe(false);
  });

  it('is ineligible for hmm cards', () => {
    expect(cardImageEligible({ card_type: 'hmm' }, true, true)).toBe(false);
  });

  it('is ineligible for the new_word intro screen', () => {
    expect(cardImageEligible({ word_id: 1, mode: 'new_word' }, true, true)).toBe(false);
  });

  it('is ineligible when the card has no word_id', () => {
    expect(cardImageEligible({ mode: 'zh_to_transl' }, true, true)).toBe(false);
  });
});
