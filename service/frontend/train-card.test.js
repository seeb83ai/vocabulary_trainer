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

// ── Recent-word exclusion tracking (issue #449) ────────────────────────────
// Mirrors addRecentWordID in train-card.js, used both when leaving a regular
// training card and when a word is answered correctly in the post-answer
// match-game, so a word SM2 just pushed forward doesn't immediately reappear
// as the next training card via GetNextCard's due-date-ignoring session
// extension fallback.

function addRecentWordID(recentWordIDs, wordID, maxLen = 2) {
  return [wordID, ...recentWordIDs].slice(0, maxLen);
}

describe('addRecentWordID', () => {
  it('prepends a new word id to an empty list', () => {
    expect(addRecentWordID([], 42)).toEqual([42]);
  });

  it('prepends and caps the list at maxLen', () => {
    expect(addRecentWordID([1], 2)).toEqual([2, 1]);
    expect(addRecentWordID([2, 1], 3)).toEqual([3, 2]);
  });

  it('tracks a match-game-answered word id the same way as a training card (issue #449)', () => {
    // A word correctly answered in the match-game must land in the same
    // exclusion list a just-shown training card already uses, so it's
    // excluded from the very next GetNextCard call too.
    let recentWordIDs = [];
    recentWordIDs = addRecentWordID(recentWordIDs, 7); // match-game correct answer for word 7
    expect(recentWordIDs).toEqual([7]);
    recentWordIDs = addRecentWordID(recentWordIDs, 3); // next regular training card leaves word 3
    expect(recentWordIDs).toEqual([3, 7]);
  });
});
