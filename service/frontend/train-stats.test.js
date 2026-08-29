import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ── due-today display count (#186) ─────────────────────────────────────────
// Mirrors dueDisplayCount in train.js: the backend may serve a not-yet-due
// "session extension" card to avoid immediately repeating a just-answered
// word; that card isn't counted in stats.due_today, so the displayed
// remaining count must add 1 back in when such a card is being shown.

function dueDisplayCount(stats, sessionExtension, newWordIntro = false) {
  return stats.due_today + (stats.hmm_due_today || 0) + (stats.components_due_today || 0)
    + (sessionExtension ? 1 : 0) + (newWordIntro ? 1 : 0);
}

describe('dueDisplayCount', () => {
  it('sums due_today, hmm_due_today, and components_due_today', () => {
    const stats = { due_today: 3, hmm_due_today: 2, components_due_today: 1 };
    expect(dueDisplayCount(stats, false)).toBe(6);
  });

  it('treats missing hmm/component counts as zero', () => {
    expect(dueDisplayCount({ due_today: 5 }, false)).toBe(5);
  });

  it('adds 1 when the current card is a session extension (non-due) card', () => {
    const stats = { due_today: 1, hmm_due_today: 0, components_due_today: 0 };
    expect(dueDisplayCount(stats, true)).toBe(2);
  });

  it('does not inflate the count when session extension is false', () => {
    const stats = { due_today: 0, hmm_due_today: 0, components_due_today: 0 };
    expect(dueDisplayCount(stats, false)).toBe(0);
  });

  it('adds 1 when the current card is a new-word introduction (issue #206)', () => {
    // The word being introduced has first_seen_at IS NULL so it is NOT counted
    // in stats.due_today.  The display must still show 1 so the user sees the
    // card they are actively working on reflected in the counter.
    const stats = { due_today: 0, hmm_due_today: 0, components_due_today: 0 };
    expect(dueDisplayCount(stats, false, true)).toBe(1);
  });

  it('combines session-extension and new-word-intro additions', () => {
    const stats = { due_today: 2, hmm_due_today: 0, components_due_today: 0 };
    expect(dueDisplayCount(stats, true, true)).toBe(4);
  });
});

// ── Success-screen advance/introduce-new-word button state ────────────────
// Mirrors successAdvanceState in train-stats.js. Both success-screen render
// paths in train-card.js (the up-front due_today===0 branch, and the
// "no words available" 404-fallback branch) must derive introduce-new-btn
// visibility from this single function so they can't drift apart — a
// mismatch here previously left users with no clickable button once their
// vocabulary was too small for the 10/20/30 advance buttons to ever enable.

function successAdvanceState(stats) {
  const allAdvanceDisabled = (stats?.available_to_advance || 0) < 10;
  const hasUnseen = (stats?.new_available || 0) > 0;
  return { allAdvanceDisabled, showIntroduceNew: allAdvanceDisabled && hasUnseen };
}

describe('successAdvanceState', () => {
  it('shows introduce-new when advance buttons are all disabled and a new word is available', () => {
    const stats = { available_to_advance: 1, new_available: 5 };
    expect(successAdvanceState(stats)).toEqual({ allAdvanceDisabled: true, showIntroduceNew: true });
  });

  it('hides introduce-new when a 10+ advance button is usable, even with new words available', () => {
    const stats = { available_to_advance: 12, new_available: 5 };
    expect(successAdvanceState(stats)).toEqual({ allAdvanceDisabled: false, showIntroduceNew: false });
  });

  it('hides introduce-new when advance is disabled but no new word is available', () => {
    const stats = { available_to_advance: 0, new_available: 0 };
    expect(successAdvanceState(stats)).toEqual({ allAdvanceDisabled: true, showIntroduceNew: false });
  });

  it('treats missing fields as zero (small brand-new account)', () => {
    expect(successAdvanceState({})).toEqual({ allAdvanceDisabled: true, showIntroduceNew: false });
  });
});

// ── End-of-session comeback info ──────────────────────────────────────────────
// Mirror computeDayStreak and dueTomorrowCount in train.js.

function computeDayStreak(days, today) {
  const trained = new Set((days || []).filter(d => d.attempts > 0).map(d => d.date));
  let streak = 0;
  const cur = new Date(today + 'T00:00:00Z');
  while (trained.has(cur.toISOString().slice(0, 10))) {
    streak++;
    cur.setUTCDate(cur.getUTCDate() - 1);
  }
  return streak;
}

function dueTomorrowCount(dates, tomorrow) {
  const hit = (dates || []).find(d => d.date === tomorrow);
  return hit ? hit.count : 0;
}

describe('computeDayStreak', () => {
  it('counts consecutive days ending today', () => {
    const days = [
      { date: '2026-08-13', attempts: 5 },
      { date: '2026-08-14', attempts: 2 },
      { date: '2026-08-15', attempts: 9 },
    ];
    expect(computeDayStreak(days, '2026-08-15')).toBe(3);
  });

  it('breaks the streak at a gap', () => {
    const days = [
      { date: '2026-08-12', attempts: 5 },
      { date: '2026-08-14', attempts: 2 },
      { date: '2026-08-15', attempts: 1 },
    ];
    expect(computeDayStreak(days, '2026-08-15')).toBe(2);
  });

  it('ignores zero-attempt days', () => {
    const days = [
      { date: '2026-08-14', attempts: 0 },
      { date: '2026-08-15', attempts: 1 },
    ];
    expect(computeDayStreak(days, '2026-08-15')).toBe(1);
  });

  it('returns 0 when today has no training', () => {
    expect(computeDayStreak([{ date: '2026-08-14', attempts: 3 }], '2026-08-15')).toBe(0);
  });

  it('handles unordered input and month boundaries', () => {
    const days = [
      { date: '2026-09-01', attempts: 1 },
      { date: '2026-08-31', attempts: 4 },
    ];
    expect(computeDayStreak(days, '2026-09-01')).toBe(2);
  });

  it('handles empty and missing input', () => {
    expect(computeDayStreak([], '2026-08-15')).toBe(0);
    expect(computeDayStreak(null, '2026-08-15')).toBe(0);
  });
});

describe('dueTomorrowCount', () => {
  const dates = [
    { date: '2026-08-15', count: 4 },
    { date: '2026-08-16', count: 7 },
  ];

  it('finds the count for tomorrow', () => {
    expect(dueTomorrowCount(dates, '2026-08-16')).toBe(7);
  });

  it('returns 0 when tomorrow has no due words', () => {
    expect(dueTomorrowCount(dates, '2026-08-17')).toBe(0);
  });

  it('handles empty and missing input', () => {
    expect(dueTomorrowCount([], '2026-08-16')).toBe(0);
    expect(dueTomorrowCount(null, '2026-08-16')).toBe(0);
  });
});
