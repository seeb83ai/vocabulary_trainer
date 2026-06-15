import { describe, it, expect } from 'vitest';

// Inlined pure helper from pinyin.js (inline-copy convention per CLAUDE.md).
function formatDuration(ms) {
  const mins = Math.round(ms / 60000);
  if (mins < 60) return `${mins} min`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours} hour${hours > 1 ? 's' : ''}`;
  const days = Math.round(hours / 24);
  return `${days} day${days > 1 ? 's' : ''}`;
}

describe('formatDuration', () => {
  it('formats sub-hour durations in minutes', () => {
    expect(formatDuration(60000)).toBe('1 min');
    expect(formatDuration(25 * 60000)).toBe('25 min');
    expect(formatDuration(0)).toBe('0 min');
  });

  it('formats hours with pluralisation', () => {
    expect(formatDuration(60 * 60000)).toBe('1 hour');
    expect(formatDuration(3 * 60 * 60000)).toBe('3 hours');
  });

  it('formats days with pluralisation', () => {
    expect(formatDuration(24 * 60 * 60000)).toBe('1 day');
    expect(formatDuration(3 * 24 * 60 * 60000)).toBe('3 days');
  });
});
