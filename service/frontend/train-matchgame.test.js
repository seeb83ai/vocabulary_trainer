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
