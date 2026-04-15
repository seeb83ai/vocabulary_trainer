import { describe, it, expect } from 'vitest';

// ── MISMATCH_MODE_LABELS ───────────────────────────────────────────────────────

const MISMATCH_MODE_LABELS = {
  en_to_zh: 'EN → ZH',
  zh_to_en: 'ZH → EN',
  zh_pinyin_to_en: 'ZH + Pinyin → EN',
};

describe('MISMATCH_MODE_LABELS', () => {
  it('has a label for en_to_zh', () => {
    expect(MISMATCH_MODE_LABELS['en_to_zh']).toBeTruthy();
  });

  it('has a label for zh_to_en', () => {
    expect(MISMATCH_MODE_LABELS['zh_to_en']).toBeTruthy();
  });

  it('has a label for zh_pinyin_to_en', () => {
    expect(MISMATCH_MODE_LABELS['zh_pinyin_to_en']).toBeTruthy();
  });

  it('returns undefined for unknown mode', () => {
    expect(MISMATCH_MODE_LABELS['unknown_mode']).toBeUndefined();
  });
});

// ── formatDate ────────────────────────────────────────────────────────────────

function formatDate(iso) {
  const d = new Date(iso);
  const diffMs = Date.now() - d.getTime();
  const diffDays = Math.floor(diffMs / 86400000);
  if (diffDays === 0) return 'Today';
  if (diffDays === 1) return 'Yesterday';
  if (diffDays < 7) return `${diffDays}d ago`;
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}

describe('formatDate', () => {
  it('returns "Today" for a very recent timestamp', () => {
    const now = new Date().toISOString();
    expect(formatDate(now)).toBe('Today');
  });

  it('returns "Yesterday" for ~24h ago', () => {
    const yesterday = new Date(Date.now() - 86400000 * 1.5).toISOString();
    expect(formatDate(yesterday)).toBe('Yesterday');
  });

  it('returns "Nd ago" for recent days', () => {
    const threeDaysAgo = new Date(Date.now() - 86400000 * 3).toISOString();
    expect(formatDate(threeDaysAgo)).toBe('3d ago');
  });

  it('returns a formatted date for older entries', () => {
    const old = '2020-01-15T00:00:00Z';
    const result = formatDate(old);
    expect(result).not.toMatch(/\d+d ago/);
    expect(result.length).toBeGreaterThan(3);
  });
});
