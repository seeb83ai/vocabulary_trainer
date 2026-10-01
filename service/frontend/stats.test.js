import { describe, it, expect } from 'vitest';

// ── formatDateLabel ────────────────────────────────────────────────────────────
// Inline from stats.js to test in isolation.

function formatDateLabel(dateStr) {
  const parts = dateStr.split('-');
  const months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  return months[parseInt(parts[1], 10) - 1] + ' ' + parseInt(parts[2], 10);
}

describe('formatDateLabel', () => {
  it('formats January 1st', () => {
    expect(formatDateLabel('2026-01-01')).toBe('Jan 1');
  });

  it('formats December 31st', () => {
    expect(formatDateLabel('2026-12-31')).toBe('Dec 31');
  });

  it('strips leading zeros from day', () => {
    expect(formatDateLabel('2026-03-04')).toBe('Mar 4');
  });

  it('formats all 12 months', () => {
    const expected = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
    expected.forEach((mon, i) => {
      const month = String(i + 1).padStart(2, '0');
      expect(formatDateLabel(`2026-${month}-15`)).toBe(`${mon} 15`);
    });
  });
});

// ── statsSummary (redesign summary tiles) ────────────────────────────────────
// Inlined from stats.js.
function statsSummary(days, today) {
  const byDate = new Map((days || []).map(d => [d.date, d]));
  const shift = (date, n) => {
    const d = new Date(date + 'T00:00:00Z');
    d.setUTCDate(d.getUTCDate() + n);
    return d.toISOString().slice(0, 10);
  };
  const windowTotals = (fromOffset, toOffset) => {
    let attempts = 0, mistakes = 0, seconds = 0;
    for (let i = fromOffset; i <= toOffset; i++) {
      const d = byDate.get(shift(today, -i));
      if (!d) continue;
      attempts += d.attempts || 0;
      mistakes += d.mistakes || 0;
      seconds += d.training_seconds || 0;
    }
    return { attempts, mistakes, seconds };
  };
  const pct = w => (w.attempts > 0 ? Math.round(((w.attempts - w.mistakes) / w.attempts) * 100) : null);
  const current = windowTotals(0, 13);
  const previous = windowTotals(14, 27);
  const accuracy = pct(current);
  const prevAccuracy = pct(previous);
  // The streak ends today, or yesterday while today has no answers yet.
  let streak = 0;
  let offset = (byDate.get(today)?.attempts || 0) > 0 ? 0 : 1;
  while ((byDate.get(shift(today, -offset))?.attempts || 0) > 0) {
    streak++;
    offset++;
  }
  const todayRow = byDate.get(today);
  return {
    answersToday: todayRow?.attempts || 0,
    mistakesToday: todayRow?.mistakes || 0,
    accuracy,
    accuracyDelta: accuracy !== null && prevAccuracy !== null ? accuracy - prevAccuracy : null,
    streak,
    trainingSeconds: current.seconds,
  };
}

describe('statsSummary', () => {
  const day = (date, attempts, mistakes, seconds = 0) => ({ date, attempts, mistakes, training_seconds: seconds });

  it('reports today, 14-day accuracy with the change vs the previous 14 days, streak and time', () => {
    const days = [
      day('2026-09-10', 10, 5),         // previous window: 50%
      day('2026-09-28', 10, 2, 60),     // current window
      day('2026-09-29', 10, 0, 120),
      day('2026-09-30', 20, 2, 300),
    ];
    const s = statsSummary(days, '2026-09-30');
    expect(s.answersToday).toBe(20);
    expect(s.mistakesToday).toBe(2);
    expect(s.accuracy).toBe(90);            // 36 correct of 40
    expect(s.accuracyDelta).toBe(40);       // 90 - 50
    expect(s.streak).toBe(3);
    expect(s.trainingSeconds).toBe(480);
  });

  it('has no delta without data in the previous window and no accuracy without answers', () => {
    const s = statsSummary([day('2026-09-30', 4, 1)], '2026-09-30');
    expect(s.accuracy).toBe(75);
    expect(s.accuracyDelta).toBeNull();
    expect(statsSummary([], '2026-09-30')).toEqual({
      answersToday: 0, mistakesToday: 0, accuracy: null, accuracyDelta: null, streak: 0, trainingSeconds: 0,
    });
  });

  it('keeps yesterday\'s streak alive until today is trained', () => {
    const days = [day('2026-09-28', 3, 0), day('2026-09-29', 3, 0)];
    expect(statsSummary(days, '2026-09-30').streak).toBe(2);
    expect(statsSummary([day('2026-09-27', 3, 0)], '2026-09-30').streak).toBe(0);
  });
});
