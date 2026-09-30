import { describe, it, expect } from 'vitest';

// ── filledCount (redesign: tab counts) ───────────────────────────────────────
// Inlined from mnemonics.js.
function filledCount(values) {
  return (values || []).filter(v => (v || '').trim() !== '').length;
}

describe('filledCount', () => {
  it('counts non-blank values', () => {
    expect(filledCount(['Anna', '', '  ', 'Bob', null, undefined])).toBe(2);
  });

  it('returns 0 for no values', () => {
    expect(filledCount([])).toBe(0);
    expect(filledCount(null)).toBe(0);
  });
});
