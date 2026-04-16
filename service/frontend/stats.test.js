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
