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

// ── computeThresholdSummary ─────────────────────────────────────────────────
// Inline from settings.js to test in isolation.

function computeThresholdSummary(components, threshold) {
  const total = components.length;
  const qualifying = components.filter(c => c.coverage_pct >= threshold).length;
  return { qualifying, total };
}

describe('computeThresholdSummary', () => {
  const components = [
    { character: '一', coverage_pct: 60 },
    { character: '口', coverage_pct: 40 },
    { character: '氵', coverage_pct: 10 },
    { character: '亻', coverage_pct: 5 },
  ];

  it('counts every component as qualifying at threshold 0', () => {
    expect(computeThresholdSummary(components, 0)).toEqual({ qualifying: 4, total: 4 });
  });

  it('excludes components strictly below the threshold', () => {
    expect(computeThresholdSummary(components, 20)).toEqual({ qualifying: 2, total: 4 });
  });

  it('includes a component exactly at the threshold', () => {
    expect(computeThresholdSummary(components, 40)).toEqual({ qualifying: 2, total: 4 });
  });

  it('returns zero qualifying when the threshold exceeds every component', () => {
    expect(computeThresholdSummary(components, 100)).toEqual({ qualifying: 0, total: 4 });
  });

  it('handles an empty component list', () => {
    expect(computeThresholdSummary([], 10)).toEqual({ qualifying: 0, total: 0 });
  });
});

// ── localValidationError ────────────────────────────────────────────────────
// Inline from settings.js to test in isolation. Auto-save runs this per-group
// check before submitting so a card only blocks on the concern it owns.

function localValidationError(group, payload) {
  if (group === 'lang' && payload.secondary_lang !== '' && payload.primary_lang === payload.secondary_lang) {
    return 'Primary and secondary languages must differ.';
  }
  if (group === 'daily' && (!payload.max_new_words_per_day || payload.max_new_words_per_day < 1)) {
    return 'New words per day must be at least 1.';
  }
  if (group === 'gamification' && (!payload.gamification_frequency || payload.gamification_frequency < 1 || payload.gamification_frequency > 1440)) {
    return 'Frequency must be between 1 and 1440 minutes.';
  }
  if (group === 'component-threshold' && (isNaN(payload.component_coverage_threshold) || payload.component_coverage_threshold < 0 || payload.component_coverage_threshold > 100)) {
    return 'Threshold must be between 0 and 100.';
  }
  return null;
}

describe('localValidationError', () => {
  it('rejects equal primary/secondary languages for the lang group', () => {
    expect(localValidationError('lang', { primary_lang: 'en', secondary_lang: 'en' }))
      .toBe('Primary and secondary languages must differ.');
  });

  it('allows an empty secondary language for the lang group', () => {
    expect(localValidationError('lang', { primary_lang: 'en', secondary_lang: '' })).toBeNull();
  });

  it('rejects max_new_words_per_day below 1 for the daily group', () => {
    expect(localValidationError('daily', { max_new_words_per_day: 0 }))
      .toBe('New words per day must be at least 1.');
  });

  it('rejects an out-of-range gamification_frequency for the gamification group', () => {
    expect(localValidationError('gamification', { gamification_frequency: 1500 }))
      .toBe('Frequency must be between 1 and 1440 minutes.');
  });

  it('rejects an out-of-range component_coverage_threshold for the component-threshold group', () => {
    expect(localValidationError('component-threshold', { component_coverage_threshold: 150 }))
      .toBe('Threshold must be between 0 and 100.');
  });

  it('does not cross-validate a different group\'s concern', () => {
    // A stale-invalid gamification_frequency must not block a daily-group save.
    expect(localValidationError('daily', { max_new_words_per_day: 5, gamification_frequency: 9999 })).toBeNull();
  });

  it('returns null for an unrecognised group', () => {
    expect(localValidationError('mode', { primary_lang: 'en', secondary_lang: 'en' })).toBeNull();
  });
});
