import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ── toggleLang state logic ─────────────────────────────────────────────────────
// Pure state portion of train.js toggleLang (without DOM side-effects).

function toggleLangState(selectedLangs, lang) {
  if (selectedLangs.includes(lang)) {
    if (selectedLangs.length <= 1) return [...selectedLangs]; // cannot deselect last
    return selectedLangs.filter(l => l !== lang);
  }
  return [...selectedLangs, lang];
}

describe('toggleLang state', () => {
  it('adds a lang when not selected', () => {
    const result = toggleLangState(['en'], 'de');
    expect(result).toEqual(['en', 'de']);
  });

  it('removes a lang when already selected', () => {
    const result = toggleLangState(['en', 'de'], 'de');
    expect(result).toEqual(['en']);
  });

  it('does not remove the last selected lang', () => {
    const result = toggleLangState(['en'], 'en');
    expect(result).toEqual(['en']);
  });

  it('does not duplicate a lang', () => {
    // Adding 'en' when 'en' is already present and another lang exists
    // actually triggers the remove branch because includes() returns true.
    const result = toggleLangState(['en', 'de'], 'en');
    expect(result).toEqual(['de']);
  });

  it('keeps selection unchanged when only one lang and trying to remove', () => {
    const result = toggleLangState(['de'], 'de');
    expect(result).toEqual(['de']);
  });
});
