import { describe, it, expect } from 'vitest';

// ── summarizeImportResult ────────────────────────────────────────────────────
// Inline the result summary function from vocab.js import flow.

function summarizeImportResult(imported, skipped) {
  const wordLabel = imported === 1 ? 'word' : 'words';
  if (skipped === 0) {
    return `Imported ${imported} ${wordLabel}.`;
  }
  return `Imported ${imported} ${wordLabel}, skipped ${skipped} already in your vocabulary.`;
}

describe('summarizeImportResult', () => {
  it('uses singular "word" when imported is 1', () => {
    expect(summarizeImportResult(1, 0)).toBe('Imported 1 word.');
  });

  it('uses plural "words" when imported is 0', () => {
    expect(summarizeImportResult(0, 5)).toContain('0 words');
  });

  it('uses plural "words" when imported > 1', () => {
    expect(summarizeImportResult(5, 0)).toBe('Imported 5 words.');
  });

  it('includes skipped count when skipped > 0', () => {
    const s = summarizeImportResult(3, 2);
    expect(s).toContain('3 words');
    expect(s).toContain('skipped 2');
  });

  it('omits skipped note when skipped is 0', () => {
    const s = summarizeImportResult(5, 0);
    expect(s).not.toContain('skipped');
  });
});
