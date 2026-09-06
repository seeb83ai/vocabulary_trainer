import { describe, it, expect } from 'vitest';

// ── nextIndex / slideOpacityClass / dotMarkClass ────────────────────────────
// Inlined from carousel.js per project convention (see app.test.js).

function nextIndex(current, total) {
  return (current + 1) % total;
}

function slideOpacityClass(isActive) {
  return isActive ? 'opacity-100' : 'opacity-0';
}

function dotMarkClass(isActive) {
  return isActive ? 'bg-blue-600' : 'bg-gray-900/25';
}

describe('nextIndex', () => {
  it('advances to the next slide', () => {
    expect(nextIndex(0, 3)).toBe(1);
    expect(nextIndex(1, 3)).toBe(2);
  });

  it('wraps back to 0 after the last slide', () => {
    expect(nextIndex(2, 3)).toBe(0);
  });

  it('stays at 0 for a single-slide carousel', () => {
    expect(nextIndex(0, 1)).toBe(0);
  });
});

describe('slideOpacityClass', () => {
  it('is fully opaque when active, fully transparent otherwise', () => {
    expect(slideOpacityClass(true)).toBe('opacity-100');
    expect(slideOpacityClass(false)).toBe('opacity-0');
  });
});

describe('dotMarkClass', () => {
  it('is blue when active, muted gray otherwise', () => {
    expect(dotMarkClass(true)).toBe('bg-blue-600');
    expect(dotMarkClass(false)).toBe('bg-gray-900/25');
  });
});
