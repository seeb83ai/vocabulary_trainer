import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// ── MODE_LABELS ───────────────────────────────────────────────────────────────

const MODE_LABELS = {
  'en_to_zh': 'English → Chinese',
  'zh_to_en': 'Chinese → English',
  'zh_pinyin_to_en': 'Chinese + Pinyin → English',
};

describe('MODE_LABELS', () => {
  it('has entry for en_to_zh', () => {
    expect(MODE_LABELS['en_to_zh']).toBe('English → Chinese');
  });

  it('has entry for zh_to_en', () => {
    expect(MODE_LABELS['zh_to_en']).toBe('Chinese → English');
  });

  it('has entry for zh_pinyin_to_en', () => {
    expect(MODE_LABELS['zh_pinyin_to_en']).toBe('Chinese + Pinyin → English');
  });

  it('returns undefined for unknown mode', () => {
    expect(MODE_LABELS['unknown']).toBeUndefined();
  });
});

// ── Answer submission state machine helpers ───────────────────────────────────
// These mirror the guard logic in submitAnswer.

function canSubmit(isSubmitted, currentCard) {
  return !isSubmitted && currentCard !== null;
}

describe('submitAnswer guard', () => {
  it('allows submit when not yet submitted and card is loaded', () => {
    expect(canSubmit(false, { word_id: 1 })).toBe(true);
  });

  it('prevents double-submit', () => {
    expect(canSubmit(true, { word_id: 1 })).toBe(false);
  });

  it('prevents submit with no card loaded', () => {
    expect(canSubmit(false, null)).toBe(false);
  });
});

// ── Result display helpers ────────────────────────────────────────────────────

function buildPinyinSpan(pinyin) {
  if (!pinyin) return '';
  // mirrors the escHtml inline usage in train.js
  return `<span class="text-gray-400 text-base ml-2">${pinyin}</span>`;
}

describe('buildPinyinSpan', () => {
  it('returns empty string when pinyin is null', () => {
    expect(buildPinyinSpan(null)).toBe('');
  });

  it('returns empty string when pinyin is undefined', () => {
    expect(buildPinyinSpan(undefined)).toBe('');
  });

  it('wraps pinyin in a span', () => {
    const html = buildPinyinSpan('nǐ hǎo');
    expect(html).toContain('nǐ hǎo');
    expect(html).toContain('<span');
  });
});

// ── "Add as correct answer" button state ──────────────────────────────────────

function addBtnLabel(answer) {
  return `Add "${answer}" as correct answer`;
}

describe('add-translation button label', () => {
  it('includes the user answer in the label', () => {
    expect(addBtnLabel('essen')).toBe('Add "essen" as correct answer');
  });

  it('handles empty string answer', () => {
    expect(addBtnLabel('')).toBe('Add "" as correct answer');
  });

  it('handles Chinese answer text', () => {
    expect(addBtnLabel('你好')).toBe('Add "你好" as correct answer');
  });
});

// ── DOM integration: result area rendering ────────────────────────────────────

import { JSDOM } from 'jsdom';

function setupDOM() {
  const dom = new JSDOM(`<!DOCTYPE html>
    <html><body>
      <div id="result-area" class="hidden"></div>
      <div id="result-icon"></div>
      <div id="correct-answers"></div>
      <div id="word-breakdown" class="hidden"></div>
      <button id="add-translation-btn" class="hidden"></button>
      <span id="next-due-info"></span>
      <span id="attempt-stats"></span>
    </body></html>`);
  return dom.window.document;
}

function applyResult(doc, result, answer) {
  const icon = doc.getElementById('result-icon');
  if (result.correct) {
    icon.textContent = '✓ Correct!';
    icon.className = 'text-3xl font-bold text-green-600 mb-4';
  } else {
    icon.textContent = '✗ Wrong';
    icon.className = 'text-3xl font-bold text-red-600 mb-4';
  }

  doc.getElementById('correct-answers').textContent = result.correct_answers.join(' / ');

  const breakdown = doc.getElementById('word-breakdown');
  const addBtn = doc.getElementById('add-translation-btn');

  if (!result.correct) {
    breakdown.classList.remove('hidden');
    addBtn.textContent = `Add "${answer}" as correct answer`;
    addBtn.classList.remove('hidden');
  } else {
    breakdown.innerHTML = '';
    breakdown.classList.add('hidden');
    addBtn.classList.add('hidden');
  }

  doc.getElementById('next-due-info').textContent = `Next review in ${result.interval_days} day(s)`;
  if (result.learning_new_word || result.graduated) {
    doc.getElementById('attempt-stats').textContent = `Streak: ${result.repetitions} / 3`;
  } else {
    doc.getElementById('attempt-stats').textContent =
      `Correct: ${result.total_correct} / ${result.total_attempts}`;
  }
}

describe('result area DOM rendering', () => {
  let doc;

  beforeEach(() => {
    doc = setupDOM();
  });

  it('shows ✓ icon and hides add-button on correct answer', () => {
    applyResult(doc, {
      correct: true,
      correct_answers: ['hello'],
      interval_days: 6,
      total_correct: 1,
      total_attempts: 1,
    }, 'hello');

    expect(doc.getElementById('result-icon').textContent).toContain('Correct');
    expect(doc.getElementById('add-translation-btn').classList.contains('hidden')).toBe(true);
    expect(doc.getElementById('word-breakdown').classList.contains('hidden')).toBe(true);
  });

  it('shows ✗ icon and add-button on wrong answer', () => {
    applyResult(doc, {
      correct: false,
      correct_answers: ['hello'],
      interval_days: 1,
      total_correct: 0,
      total_attempts: 1,
    }, 'mist');

    expect(doc.getElementById('result-icon').textContent).toContain('Wrong');
    expect(doc.getElementById('add-translation-btn').classList.contains('hidden')).toBe(false);
    expect(doc.getElementById('add-translation-btn').textContent).toContain('mist');
    expect(doc.getElementById('word-breakdown').classList.contains('hidden')).toBe(false);
  });

  it('sets correct-answers text', () => {
    applyResult(doc, {
      correct: true,
      correct_answers: ['hello', 'hi'],
      interval_days: 1,
      total_correct: 1,
      total_attempts: 1,
    }, 'hello');

    expect(doc.getElementById('correct-answers').textContent).toBe('hello / hi');
  });

  it('sets next-due-info text', () => {
    applyResult(doc, {
      correct: true,
      correct_answers: ['hello'],
      interval_days: 15,
      total_correct: 3,
      total_attempts: 4,
    }, 'hello');

    expect(doc.getElementById('next-due-info').textContent).toBe('Next review in 15 day(s)');
  });

  it('sets attempt-stats text', () => {
    applyResult(doc, {
      correct: false,
      correct_answers: ['hello'],
      interval_days: 1,
      total_correct: 2,
      total_attempts: 5,
    }, 'wrong');

    expect(doc.getElementById('attempt-stats').textContent).toBe('Correct: 2 / 5');
  });
});
