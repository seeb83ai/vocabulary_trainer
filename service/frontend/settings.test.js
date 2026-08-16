import { describe, it, expect } from 'vitest';

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
