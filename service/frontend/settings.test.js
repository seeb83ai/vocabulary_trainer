import { describe, it, expect } from 'vitest';

// Inline the functions from settings.js to test them in isolation.

function parseModeRangeForUI(value, defaultValue) {
  if (value === 'off') return { off: true, from: '', to: '' };
  const [from, to] = (value || defaultValue).split(',');
  return { off: false, from, to };
}

function modeRangeValue(off, from, to) {
  return off ? 'off' : `${from},${to}`;
}

describe('parseModeRangeForUI', () => {
  it('falls back to the default range when value is empty (unset)', () => {
    expect(parseModeRangeForUI('', 'new,50-69')).toEqual({ off: false, from: 'new', to: '50-69' });
  });

  it('parses an explicit "from,to" range', () => {
    expect(parseModeRangeForUI('50-69,85-100', 'new,50-69')).toEqual({ off: false, from: '50-69', to: '85-100' });
  });

  it('reports off=true for the literal "off" value, ignoring the default', () => {
    expect(parseModeRangeForUI('off', 'new,50-69')).toEqual({ off: true, from: '', to: '' });
  });
});

describe('modeRangeValue', () => {
  it('returns "off" when the mode is disabled, ignoring bucket selects', () => {
    expect(modeRangeValue(true, 'new', '85-100')).toBe('off');
  });

  it('joins the from/to buckets with a comma when enabled', () => {
    expect(modeRangeValue(false, 'new', '50-69')).toBe('new,50-69');
  });

  it('round-trips through parseModeRangeForUI for an explicit range', () => {
    const value = modeRangeValue(false, '70-84', '85-100');
    expect(parseModeRangeForUI(value, 'new,50-69')).toEqual({ off: false, from: '70-84', to: '85-100' });
  });
});
