import { describe, it, expect } from 'vitest';

// ── wordCell ──────────────────────────────────────────────────────────────────

function escHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function wordCell(text, pinyin, translations, kind, wordId, character, other) {
  const pinyinHtml = pinyin ? `<span class="mm-py">${escHtml(pinyin)}</span>` : '';
  const allTexts = Object.values(translations || {}).flat();
  const transHtml = allTexts.length ? `<span class="mm-en">${allTexts.map(escHtml).join(', ')}</span>` : '';
  let audioBtn = '';
  if (kind === 'component' && character) {
    audioBtn = `<button type="button" class="btn-word-play mm-play" data-kind="component" data-character="${escHtml(character)}" title="Read aloud" aria-label="Read aloud">🔊</button>`;
  } else if (wordId) {
    audioBtn = `<button type="button" class="btn-word-play mm-play" data-kind="word" data-word-id="${wordId}" data-zh-text="${escHtml(text)}" title="Read aloud" aria-label="Read aloud">🔊</button>`;
  }
  return `<div class="mm-side${other ? ' is-other' : ''}"><span class="mm-word"><span class="hanzi mm-zh">${escHtml(text)}</span>${pinyinHtml}${audioBtn}</span>${transHtml}</div>`;
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
    const html = wordCell('手', 'shǒu', {}, 'word', 42);
    expect(html).toContain('btn-word-play');
    expect(html).toContain('data-word-id="42"');
  });

  it('renders audio button with correct data-zh-text when wordId provided', () => {
    const html = wordCell('手', 'shǒu', {}, 'word', 42);
    expect(html).toContain('data-zh-text="手"');
  });

  it('escapes special chars in data-zh-text attribute', () => {
    const html = wordCell('<test>', null, {}, 'word', 1);
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

// ── sortMismatches (redesign: client-side sort) ──────────────────────────────
// Inlined from mismatches.js.
function sortMismatches(items, by) {
  const list = [...(items || [])];
  const time = x => new Date(x.last_seen).getTime() || 0;
  if (by === 'recent') {
    list.sort((a, b) => time(b) - time(a) || b.count - a.count);
  } else {
    list.sort((a, b) => b.count - a.count || time(b) - time(a));
  }
  return list;
}

describe('sortMismatches', () => {
  const a = { id: 'a', count: 2, last_seen: '2026-09-30T10:00:00Z' };
  const b = { id: 'b', count: 6, last_seen: '2026-09-27T10:00:00Z' };
  const c = { id: 'c', count: 2, last_seen: '2026-09-29T10:00:00Z' };

  it('sorts by count, most often first, ties by recency', () => {
    expect(sortMismatches([a, b, c], 'often').map(x => x.id)).toEqual(['b', 'a', 'c']);
  });

  it('sorts by last seen, most recent first', () => {
    expect(sortMismatches([b, c, a], 'recent').map(x => x.id)).toEqual(['a', 'c', 'b']);
  });

  it('does not change the input and handles empty input', () => {
    const input = [a, b];
    sortMismatches(input, 'often');
    expect(input.map(x => x.id)).toEqual(['a', 'b']);
    expect(sortMismatches(null, 'often')).toEqual([]);
  });
});
