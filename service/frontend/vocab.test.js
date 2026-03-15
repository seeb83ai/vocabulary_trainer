import { describe, it, expect, beforeEach } from 'vitest';

// ── renderProgress ─────────────────────────────────────────────────────────────
// Inline the function from vocab.js to test it in isolation.

function renderProgress(word) {
  if (word.total_attempts === 0) {
    return '<span class="text-gray-400">New</span>';
  }
  const pct = word.total_attempts > 0
    ? Math.round((word.total_correct / word.total_attempts) * 100)
    : 0;
  const due = new Date(word.due_date);
  const now = new Date();
  const diffDays = Math.round((due - now) / 86400000);
  const dueStr = diffDays <= 0 ? '<span class="text-orange-500">Due</span>'
    : `in ${diffDays}d`;

  let barColor = 'bg-red-400';
  if (pct >= 80) barColor = 'bg-green-400';
  else if (pct >= 50) barColor = 'bg-yellow-400';

  return `
    <div class="flex flex-col gap-0.5 min-w-[90px]">
      <div class="flex items-center gap-1">
        <div class="w-16 h-1.5 bg-gray-200 rounded-full overflow-hidden">
          <div class="${barColor} h-full rounded-full" style="width:${pct}%"></div>
        </div>
        <span class="text-gray-500">${pct}%</span>
      </div>
      <div class="text-gray-400">${word.repetitions} reps · ${dueStr}</div>
    </div>`;
}

describe('renderProgress', () => {
  it('returns "New" when no attempts', () => {
    const result = renderProgress({ total_attempts: 0 });
    expect(result).toContain('New');
  });

  it('shows correct percentage for perfect score', () => {
    const word = {
      total_attempts: 10,
      total_correct: 10,
      due_date: new Date(Date.now() + 86400000 * 5).toISOString(),
      repetitions: 3,
    };
    expect(renderProgress(word)).toContain('100%');
  });

  it('shows correct percentage for partial score', () => {
    const word = {
      total_attempts: 4,
      total_correct: 2,
      due_date: new Date(Date.now() + 86400000 * 5).toISOString(),
      repetitions: 2,
    };
    expect(renderProgress(word)).toContain('50%');
  });

  it('uses green bar when >= 80%', () => {
    const word = {
      total_attempts: 10,
      total_correct: 9,
      due_date: new Date(Date.now() + 86400000).toISOString(),
      repetitions: 5,
    };
    expect(renderProgress(word)).toContain('bg-green-400');
  });

  it('uses yellow bar when 50–79%', () => {
    const word = {
      total_attempts: 10,
      total_correct: 6,
      due_date: new Date(Date.now() + 86400000).toISOString(),
      repetitions: 3,
    };
    expect(renderProgress(word)).toContain('bg-yellow-400');
  });

  it('uses red bar when < 50%', () => {
    const word = {
      total_attempts: 10,
      total_correct: 3,
      due_date: new Date(Date.now() + 86400000).toISOString(),
      repetitions: 2,
    };
    expect(renderProgress(word)).toContain('bg-red-400');
  });

  it('shows "Due" when due_date is in the past', () => {
    const word = {
      total_attempts: 5,
      total_correct: 4,
      due_date: new Date(Date.now() - 86400000).toISOString(),
      repetitions: 3,
    };
    expect(renderProgress(word)).toContain('Due');
  });

  it('shows future days when not yet due', () => {
    const word = {
      total_attempts: 5,
      total_correct: 4,
      due_date: new Date(Date.now() + 86400000 * 7).toISOString(),
      repetitions: 3,
    };
    expect(renderProgress(word)).toContain('in 7d');
  });

  it('shows repetition count', () => {
    const word = {
      total_attempts: 5,
      total_correct: 4,
      due_date: new Date(Date.now() + 86400000).toISOString(),
      repetitions: 7,
    };
    expect(renderProgress(word)).toContain('7 reps');
  });
});

// ── renderPagination (logic only) ─────────────────────────────────────────────

function totalPages(total, perPage) {
  return Math.max(1, Math.ceil(total / perPage));
}

describe('pagination logic', () => {
  it('returns 1 for empty list', () => {
    expect(totalPages(0, 20)).toBe(1);
  });

  it('returns 1 when items fit on one page', () => {
    expect(totalPages(10, 20)).toBe(1);
  });

  it('returns 2 when items spill to second page', () => {
    expect(totalPages(21, 20)).toBe(2);
  });

  it('returns correct count for exact multiple', () => {
    expect(totalPages(40, 20)).toBe(2);
  });

  it('rounds up for partial last page', () => {
    expect(totalPages(41, 20)).toBe(3);
  });
});

// ── buildFormPayload (DOM-based) ───────────────────────────────────────────────
// Simulate the DOM structure that vocab.html provides.

function buildFormPayload(zhValue, pinyinValue, enValues, tags = [], startTraining = false) {
  // Mirrors the vocab.js buildFormPayload logic
  return {
    zh_text: zhValue.trim(),
    pinyin: pinyinValue.trim(),
    en_texts: enValues.map(v => v.trim()).filter(Boolean),
    tags: [...tags],
    start_training: startTraining,
  };
}

describe('buildFormPayload', () => {
  it('trims whitespace from zh_text', () => {
    const p = buildFormPayload('  你好  ', '', ['hello']);
    expect(p.zh_text).toBe('你好');
  });

  it('trims whitespace from pinyin', () => {
    const p = buildFormPayload('你好', '  nǐ hǎo  ', ['hello']);
    expect(p.pinyin).toBe('nǐ hǎo');
  });

  it('filters empty en_texts', () => {
    const p = buildFormPayload('你好', '', ['hello', '  ', '']);
    expect(p.en_texts).toEqual(['hello']);
  });

  it('allows multiple en_texts', () => {
    const p = buildFormPayload('你好', '', ['hello', 'hi', 'hey']);
    expect(p.en_texts).toHaveLength(3);
  });

  it('returns empty pinyin when not provided', () => {
    const p = buildFormPayload('你好', '', ['hello']);
    expect(p.pinyin).toBe('');
  });

  it('includes tags array', () => {
    const p = buildFormPayload('你好', '', ['hello'], ['HSK1', 'greetings']);
    expect(p.tags).toEqual(['HSK1', 'greetings']);
  });

  it('defaults to empty tags', () => {
    const p = buildFormPayload('你好', '', ['hello']);
    expect(p.tags).toEqual([]);
  });

  it('defaults start_training to false', () => {
    const p = buildFormPayload('你好', '', ['hello']);
    expect(p.start_training).toBe(false);
  });

  it('includes start_training when true', () => {
    const p = buildFormPayload('你好', '', ['hello'], [], true);
    expect(p.start_training).toBe(true);
  });
});

// ── renderDue ─────────────────────────────────────────────────────────────────
// Inline the fixed function from vocab.js.

function renderDue(word) {
  if (word.total_attempts === 0) {
    return '<span class="text-gray-400">—</span>';
  }
  if (!word.due_date) {
    return '<span class="text-gray-400">—</span>';
  }
  const due = new Date(word.due_date);
  if (isNaN(due.getTime())) {
    return '<span class="text-gray-400">—</span>';
  }
  const diffDays = Math.round((due - new Date()) / 86400000);
  if (diffDays <= 0) return '<span class="text-orange-500">Due</span>';
  return `<span class="text-gray-500">in ${diffDays}d</span>`;
}

describe('renderDue', () => {
  it('returns em-dash for unseen words (total_attempts=0)', () => {
    expect(renderDue({ total_attempts: 0, due_date: null })).toContain('—');
  });

  it('returns em-dash for null due_date', () => {
    expect(renderDue({ total_attempts: 1, due_date: null })).toContain('—');
  });

  it('returns em-dash for invalid date string', () => {
    expect(renderDue({ total_attempts: 1, due_date: 'not-a-date' })).toContain('—');
  });

  it('returns "Due" for past due date', () => {
    const past = new Date(Date.now() - 86400000 * 2).toISOString();
    expect(renderDue({ total_attempts: 1, due_date: past })).toContain('Due');
  });

  it('returns "Due" for due date exactly now', () => {
    const now = new Date().toISOString();
    expect(renderDue({ total_attempts: 1, due_date: now })).toContain('Due');
  });

  it('returns "in Nd" for future due date', () => {
    const future = new Date(Date.now() + 86400000 * 5).toISOString();
    expect(renderDue({ total_attempts: 1, due_date: future })).toMatch(/in \d+d/);
  });
});
