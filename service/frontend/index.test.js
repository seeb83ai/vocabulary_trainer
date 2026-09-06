import { describe, it, expect } from 'vitest';

// ── activeTabClasses / initialTab ───────────────────────────────────────────
// Inlined from index.js per project convention (see app.test.js / toast.test.js)
// to keep tests self-contained and independent of module bundling.

function activeTabClasses(isActive) {
  return 'flex-1 py-2 rounded-lg text-sm font-semibold transition ' +
    (isActive ? 'bg-white text-gray-900 shadow-sm' : 'text-gray-500 hover:text-gray-800');
}

function initialTab(hash) {
  return hash === '#register' ? 'register' : 'signin';
}

describe('activeTabClasses', () => {
  it('gives the active tab a white background and dark text', () => {
    expect(activeTabClasses(true)).toContain('bg-white');
    expect(activeTabClasses(true)).toContain('text-gray-900');
  });

  it('gives the inactive tab muted, hoverable text and no background', () => {
    const classes = activeTabClasses(false);
    expect(classes).toContain('text-gray-500');
    expect(classes).not.toContain('bg-white');
  });
});

describe('initialTab', () => {
  it('opens the register tab when the URL hash is #register', () => {
    expect(initialTab('#register')).toBe('register');
  });

  it('defaults to the sign-in tab for any other hash', () => {
    expect(initialTab('')).toBe('signin');
    expect(initialTab('#something-else')).toBe('signin');
  });
});
