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

// ── computeCoverageSelection ─────────────────────────────────────────────────
// Inline from settings.js to test in isolation. Mirrors
// selectComponentsForCoverage in service/db/components.go (same greedy
// set-cover approximation and character-ascending tie-break).

function computeCoverageSelection(components, totalWords, targetPct) {
  const totalComponents = components.length;
  if (totalWords <= 0 || targetPct <= 0 || totalComponents === 0) {
    return { selectedCount: 0, totalComponents, selectedCharacters: new Set() };
  }
  const target = (targetPct / 100) * totalWords;
  const remaining = components
    .map(c => ({ character: c.character, wordIds: c.word_ids || [] }))
    .sort((a, b) => (a.character < b.character ? -1 : a.character > b.character ? 1 : 0));
  const covered = new Set();
  const selectedCharacters = new Set();
  while (covered.size < target && remaining.length > 0) {
    let bestIdx = -1;
    let bestGain = 0;
    for (let i = 0; i < remaining.length; i++) {
      let gain = 0;
      for (const wid of remaining[i].wordIds) {
        if (!covered.has(wid)) gain++;
      }
      if (gain > bestGain) {
        bestGain = gain;
        bestIdx = i;
      }
    }
    if (bestIdx === -1) break;
    for (const wid of remaining[bestIdx].wordIds) covered.add(wid);
    selectedCharacters.add(remaining[bestIdx].character);
    remaining.splice(bestIdx, 1);
  }
  return { selectedCount: selectedCharacters.size, totalComponents, selectedCharacters };
}

// outOfScopeCount is pure: of the currently-trained components, how many are
// not in the selected set for the previewed target — i.e. already-trained
// components that exceed what's needed to reach a (possibly lower) target
// (#352). Informational only; the setting never removes trained components.
function outOfScopeCount(trainedCharacters, selectedCharacters) {
  return trainedCharacters.filter(c => !selectedCharacters.has(c)).length;
}

describe('computeCoverageSelection', () => {
  // 日 covers words {1,2,3}; 月 covers {3,4}; 女 covers {5}. Total 5 words.
  const components = [
    { character: '日', word_ids: [1, 2, 3] },
    { character: '月', word_ids: [3, 4] },
    { character: '女', word_ids: [5] },
  ];

  it('selects nothing at target 0', () => {
    const { selectedCount, totalComponents } = computeCoverageSelection(components, 5, 0);
    expect({ selectedCount, totalComponents }).toEqual({ selectedCount: 0, totalComponents: 3 });
  });

  it('selects just the highest-gain component when it alone reaches the target', () => {
    // 60% of 5 = 3 -> 日 alone (covers 3) is enough.
    const { selectedCount, totalComponents } = computeCoverageSelection(components, 5, 60);
    expect({ selectedCount, totalComponents }).toEqual({ selectedCount: 1, totalComponents: 3 });
  });

  it('breaks gain ties by ascending character', () => {
    // 80% of 5 = 4 -> 日 (3) then a tie between 月 and 女 (each +1); 女 sorts first.
    const { selectedCount, totalComponents } = computeCoverageSelection(components, 5, 80);
    expect({ selectedCount, totalComponents }).toEqual({ selectedCount: 2, totalComponents: 3 });
  });

  it('selects every component to reach 100% coverage', () => {
    const { selectedCount, totalComponents } = computeCoverageSelection(components, 5, 100);
    expect({ selectedCount, totalComponents }).toEqual({ selectedCount: 3, totalComponents: 3 });
  });

  it('handles an empty component list', () => {
    const { selectedCount, totalComponents } = computeCoverageSelection([], 0, 50);
    expect({ selectedCount, totalComponents }).toEqual({ selectedCount: 0, totalComponents: 0 });
  });
});

describe('outOfScopeCount (issue #352)', () => {
  // Same fixture as above: 80% target selects 日 and 女 (not 月).
  const components = [
    { character: '日', word_ids: [1, 2, 3] },
    { character: '月', word_ids: [3, 4] },
    { character: '女', word_ids: [5] },
  ];

  it('counts trained components that are not in the selected set', () => {
    const { selectedCharacters } = computeCoverageSelection(components, 5, 80);
    expect(outOfScopeCount(['日', '月', '女'], selectedCharacters)).toBe(1); // 月 is out of scope
  });

  it('is zero when every trained component is still selected', () => {
    const { selectedCharacters } = computeCoverageSelection(components, 5, 100);
    expect(outOfScopeCount(['日', '月'], selectedCharacters)).toBe(0);
  });

  it('is zero when no components are trained yet', () => {
    const { selectedCharacters } = computeCoverageSelection(components, 5, 80);
    expect(outOfScopeCount([], selectedCharacters)).toBe(0);
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
