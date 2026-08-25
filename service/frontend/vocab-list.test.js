import { describe, it, expect, beforeEach } from 'vitest';

// ── renderProgress ─────────────────────────────────────────────────────────────
// Inline the function from vocab.js to test it in isolation.

function renderProgress(word) {
  if (word.total_attempts === 0) {
    return '<span class="text-gray-400">New</span>';
  }
  const effCorrect = word.total_correct + (word.streak_bonus || 0);
  const pct = word.total_attempts > 0
    ? Math.round((effCorrect / word.total_attempts) * 100)
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

// ── renderDue ─────────────────────────────────────────────────────────────────
// Inlined from vocab.js (without i18n/HTML, using plain strings for logic).

function renderDue(word) {
  if (word.total_attempts === 0) return 'unseen';
  if (!word.due_date) return '—';
  const due = new Date(word.due_date);
  if (isNaN(due.getTime())) return '—';
  const diffDays = Math.round((due - new Date()) / 86400000);
  if (diffDays <= 0) return 'due';
  return `in ${diffDays}d`;
}

describe('renderDue', () => {
  it('returns "unseen" when no attempts', () => {
    expect(renderDue({ total_attempts: 0, due_date: null })).toBe('unseen');
  });

  it('returns em-dash for null due_date', () => {
    expect(renderDue({ total_attempts: 1, due_date: null })).toBe('—');
  });

  it('returns em-dash for invalid date string', () => {
    expect(renderDue({ total_attempts: 1, due_date: 'not-a-date' })).toBe('—');
  });

  it('returns "due" when due_date is in the past', () => {
    const word = {
      total_attempts: 5,
      due_date: new Date(Date.now() - 86400000 * 2).toISOString(),
    };
    expect(renderDue(word)).toBe('due');
  });

  it('returns "due" when due_date is now (diff rounds to 0)', () => {
    const word = {
      total_attempts: 3,
      due_date: new Date().toISOString(),
    };
    expect(renderDue(word)).toBe('due');
  });

  it('returns future days when not yet due', () => {
    const word = {
      total_attempts: 5,
      due_date: new Date(Date.now() + 86400000 * 7).toISOString(),
    };
    expect(renderDue(word)).toBe('in 7d');
  });
});

// ── missingLangFilter state logic ────────────────────────────────────────────
// The filter state is a simple string that controls query param sent to API.

function buildMissingLangParam(missingLangFilter) {
  if (!missingLangFilter) return null;
  if (missingLangFilter === 'en' || missingLangFilter === 'de') return missingLangFilter;
  return null;
}

describe('missingLangFilter state', () => {
  it('returns null for empty string (no filter)', () => {
    expect(buildMissingLangParam('')).toBeNull();
  });

  it('returns "en" for en filter', () => {
    expect(buildMissingLangParam('en')).toBe('en');
  });

  it('returns "de" for de filter', () => {
    expect(buildMissingLangParam('de')).toBe('de');
  });

  it('returns null for unknown filter value', () => {
    expect(buildMissingLangParam('fr')).toBeNull();
  });
});

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}


function crossRefBadge(show, label) {
  return show
    ? `<span class="inline-block bg-purple-100 text-purple-600 text-xs px-1.5 py-0.5 rounded-full ml-1 align-middle">${escHtml(label)}</span>`
    : '';
}

describe('crossRefBadge', () => {
  it('renders the badge with the given label when show is true', () => {
    const html = crossRefBadge(true, 'also a component');
    expect(html).toContain('also a component');
  });

  it('renders nothing when show is false', () => {
    expect(crossRefBadge(false, 'also a component')).toBe('');
  });

  it('escapes the label', () => {
    const html = crossRefBadge(true, '<b>x</b>');
    expect(html).not.toContain('<b>');
    expect(html).toContain('&lt;b&gt;');
  });
});
