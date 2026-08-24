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
    return { selectedCount: 0, totalComponents };
  }
  const target = (targetPct / 100) * totalWords;
  const remaining = components
    .map(c => ({ character: c.character, wordIds: c.word_ids || [] }))
    .sort((a, b) => (a.character < b.character ? -1 : a.character > b.character ? 1 : 0));
  const covered = new Set();
  let selectedCount = 0;
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
    remaining.splice(bestIdx, 1);
    selectedCount++;
  }
  return { selectedCount, totalComponents };
}

describe('computeCoverageSelection', () => {
  // 日 covers words {1,2,3}; 月 covers {3,4}; 女 covers {5}. Total 5 words.
  const components = [
    { character: '日', word_ids: [1, 2, 3] },
    { character: '月', word_ids: [3, 4] },
    { character: '女', word_ids: [5] },
  ];

  it('selects nothing at target 0', () => {
    expect(computeCoverageSelection(components, 5, 0)).toEqual({ selectedCount: 0, totalComponents: 3 });
  });

  it('selects just the highest-gain component when it alone reaches the target', () => {
    // 60% of 5 = 3 -> 日 alone (covers 3) is enough.
    expect(computeCoverageSelection(components, 5, 60)).toEqual({ selectedCount: 1, totalComponents: 3 });
  });

  it('breaks gain ties by ascending character', () => {
    // 80% of 5 = 4 -> 日 (3) then a tie between 月 and 女 (each +1); 女 sorts first.
    expect(computeCoverageSelection(components, 5, 80)).toEqual({ selectedCount: 2, totalComponents: 3 });
  });

  it('selects every component to reach 100% coverage', () => {
    expect(computeCoverageSelection(components, 5, 100)).toEqual({ selectedCount: 3, totalComponents: 3 });
  });

  it('handles an empty component list', () => {
    expect(computeCoverageSelection([], 0, 50)).toEqual({ selectedCount: 0, totalComponents: 0 });
  });
});
