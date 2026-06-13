import { describe, it, expect } from 'vitest';

// ── wordCell ──────────────────────────────────────────────────────────────────

function escHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function wordCell(text, pinyin, translations, wordId) {
  const pinyinHtml = pinyin ? `<span class="text-gray-400 text-xs ml-1">${escHtml(pinyin)}</span>` : '';
  const allTexts = Object.values(translations || {}).flat();
  const transHtml = allTexts.length ? `<div class="text-gray-500 text-xs mt-0.5">${allTexts.map(escHtml).join(', ')}</div>` : '';
  const audioBtn = wordId
    ? `<button class="btn-word-play ml-1 text-gray-400 hover:text-blue-500 transition" data-word-id="${wordId}" data-zh-text="${escHtml(text)}" title="Read aloud">🔊</button>`
    : '';
  return `<div class="flex items-center gap-1 text-base font-medium text-gray-800">${escHtml(text)}${pinyinHtml}${audioBtn}</div>${transHtml}`;
}

describe('wordCell', () => {
  it('renders text and pinyin without audio button when no wordId given', () => {
    const html = wordCell('苹果', 'píngguǒ', { en: ['apple'] });
    expect(html).toContain('苹果');
    expect(html).toContain('píngguǒ');
    expect(html).not.toContain('btn-word-play');
    expect(html).not.toContain('🔊');
  });

  it('renders audio button with correct data-word-id when wordId provided', () => {
    const html = wordCell('手', 'shǒu', {}, 42);
    expect(html).toContain('btn-word-play');
    expect(html).toContain('data-word-id="42"');
  });

  it('renders audio button with correct data-zh-text when wordId provided', () => {
    const html = wordCell('手', 'shǒu', {}, 42);
    expect(html).toContain('data-zh-text="手"');
  });

  it('escapes special chars in data-zh-text attribute', () => {
    const html = wordCell('<test>', null, {}, 1);
    expect(html).toContain('data-zh-text="&lt;test&gt;"');
  });
});

// ── MISMATCH_MODE_LABELS ───────────────────────────────────────────────────────

const MISMATCH_MODE_LABELS = {
  transl_to_zh: 'To Chinese',
  zh_to_transl: 'Chinese',
  zh_pinyin_to_transl: 'Chinese + Pinyin',
};

describe('MISMATCH_MODE_LABELS', () => {
  it('has a label for transl_to_zh', () => {
    expect(MISMATCH_MODE_LABELS['transl_to_zh']).toBeTruthy();
  });

  it('has a label for zh_to_transl', () => {
    expect(MISMATCH_MODE_LABELS['zh_to_transl']).toBeTruthy();
  });

  it('has a label for zh_pinyin_to_transl', () => {
    expect(MISMATCH_MODE_LABELS['zh_pinyin_to_transl']).toBeTruthy();
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
