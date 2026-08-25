import { describe, it, expect, beforeEach } from 'vitest';

// ── buildImportPayload ────────────────────────────────────────────────────────
// Inline the payload-building logic from vocab.js import flow.

function buildImportPayload(tag, importEn, importDe, applyTags) {
  return {
    tag: tag.trim(),
    import_en: importEn,
    import_de: importDe,
    apply_tags: [...applyTags],
  };
}

describe('buildImportPayload', () => {
  it('trims whitespace from tag', () => {
    const p = buildImportPayload('  HSK1  ', true, false, []);
    expect(p.tag).toBe('HSK1');
  });

  it('includes import_en and import_de flags', () => {
    const p = buildImportPayload('HSK1', true, false, []);
    expect(p.import_en).toBe(true);
    expect(p.import_de).toBe(false);
  });

  it('copies apply_tags without mutating the source array', () => {
    const src = ['a', 'b'];
    const p = buildImportPayload('HSK1', true, false, src);
    p.apply_tags.push('c');
    expect(src).toEqual(['a', 'b']);
  });

  it('passes apply_tags through', () => {
    const p = buildImportPayload('HSK1', true, true, ['foo', 'bar']);
    expect(p.apply_tags).toEqual(['foo', 'bar']);
  });
});

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
