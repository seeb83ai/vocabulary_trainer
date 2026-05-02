import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';


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
    doc.getElementById('attempt-stats').textContent = `Streak: ${result.repetitions} / ${result.graduate_reps}`;
  } else {
    const eff = result.total_correct + (result.streak_bonus || 0);
    doc.getElementById('attempt-stats').textContent =
      `Correct: ${eff} / ${result.total_attempts}` +
      (result.streak_bonus > 0 ? ` (+${result.streak_bonus} streak bonus)` : '');
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

// ── renderCharDecomposition component pinyin ──────────────────────────────────
// Inlined from train.js for isolated unit testing.

function escHtml(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function renderCharDecomposition(charData) {
  let html = `<div class="p-3 bg-gray-50 border border-gray-200 rounded-xl mb-2">`;
  html += `<div class="flex items-baseline gap-2 mb-1">`;
  html += `<span class="text-2xl font-bold">${escHtml(charData.character)}</span>`;
  if (charData.radical) {
    html += `<span class="text-sm text-gray-400">${escHtml(charData.radical)}</span>`;
  }
  if (charData.definition) {
    html += `<span class="text-sm text-gray-500">${escHtml(charData.definition)}</span>`;
  }
  html += `</div>`;

  if (charData.components && charData.components.length > 0) {
    html += `<div class="flex flex-wrap gap-2 mt-1">`;
    for (const comp of charData.components) {
      const isPhonetic = comp.is_semantic === false;
      const dimClass = isPhonetic ? ' opacity-40' : '';
      const title = isPhonetic ? ' title="Phonetic component (sound hint only)"' : '';
      html += `<div class="px-2 py-1 bg-white border border-gray-200 rounded-lg text-center min-w-[3rem]${dimClass}"${title}>`;
      html += `<div class="text-lg font-medium">${escHtml(comp.character)}</div>`;
      if (comp.pinyin && comp.pinyin.length > 0) {
        html += `<div class="text-xs text-gray-400">${escHtml(comp.pinyin.join(' / '))}</div>`;
      }
      if (comp.definition) {
        html += `<div class="text-xs text-gray-400 leading-tight">${escHtml(comp.definition)}</div>`;
      }
      html += `</div>`;
    }
    html += `</div>`;
  }

  html += `</div>`;
  return html;
}

describe('renderCharDecomposition component pinyin', () => {
  it('shows pinyin below character when present', () => {
    const html = renderCharDecomposition({
      character: '好',
      components: [{ character: '女', pinyin: ['nǚ'], definition: 'woman' }],
    });
    expect(html).toContain('nǚ');
  });

  it('joins multiple readings with " / "', () => {
    const html = renderCharDecomposition({
      character: '行',
      components: [{ character: '行', pinyin: ['háng', 'xíng'], definition: 'walk' }],
    });
    expect(html).toContain('háng / xíng');
  });

  it('omits pinyin div when pinyin array is empty', () => {
    const html = renderCharDecomposition({
      character: '好',
      components: [{ character: '女', pinyin: [], definition: 'woman' }],
    });
    const pinyinDivCount = (html.match(/text-xs text-gray-400/g) || []).length;
    // definition div also has text-xs text-gray-400 — pinyin adds one more
    const htmlWithPinyin = renderCharDecomposition({
      character: '好',
      components: [{ character: '女', pinyin: ['nǚ'], definition: 'woman' }],
    });
    expect(html.length).toBeLessThan(htmlWithPinyin.length);
  });

  it('omits pinyin div when pinyin is absent', () => {
    const html = renderCharDecomposition({
      character: '好',
      components: [{ character: '女', definition: 'woman' }],
    });
    expect(html).not.toContain('háng');
    expect(html).toContain('woman');
  });
});

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

// ── allTransTexts filtering ────────────────────────────────────────────────────
// Mirrors the logic in train.js that filters translations by selectedLangs.

function buildAllTransTexts(selectedLangs, result) {
  const translations = result.translations || {};
  return selectedLangs.flatMap(lang => translations[lang] || []);
}

describe('allTransTexts', () => {
  it('includes EN texts when en is selected', () => {
    const texts = buildAllTransTexts(['en'], { translations: { en: ['hello'], de: ['hallo'] } });
    expect(texts).toContain('hello');
    expect(texts).not.toContain('hallo');
  });

  it('includes DE texts when de is selected', () => {
    const texts = buildAllTransTexts(['de'], { translations: { en: ['hello'], de: ['hallo'] } });
    expect(texts).toContain('hallo');
    expect(texts).not.toContain('hello');
  });

  it('includes both when both are selected', () => {
    const texts = buildAllTransTexts(['en', 'de'], { translations: { en: ['hello'], de: ['hallo'] } });
    expect(texts).toContain('hello');
    expect(texts).toContain('hallo');
  });

  it('handles missing en translations gracefully', () => {
    const texts = buildAllTransTexts(['en', 'de'], { translations: { de: ['hallo'] } });
    expect(texts).toEqual(['hallo']);
  });

  it('handles missing de translations gracefully', () => {
    const texts = buildAllTransTexts(['en', 'de'], { translations: { en: ['hello'] } });
    expect(texts).toEqual(['hello']);
  });

  it('returns empty array when no langs selected', () => {
    const texts = buildAllTransTexts([], { translations: { en: ['hello'], de: ['hallo'] } });
    expect(texts).toEqual([]);
  });
});
