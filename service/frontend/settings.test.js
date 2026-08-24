import { describe, it, expect } from 'vitest';

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
