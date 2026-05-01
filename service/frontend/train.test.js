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

// ── numberedToToneMark ────────────────────────────────────────────────────────

function numberedToToneMark(pinyin) {
  if (!pinyin) return pinyin;
  const s = pinyin.toLowerCase();
  const last = s[s.length - 1];
  if (last < '1' || last > '5') return s;
  const tone = parseInt(last, 10);
  let syllable = s.slice(0, -1).replace(/u:/g, 'ü').replace(/v/g, 'ü');
  if (tone === 5) return syllable;
  const toneMarks = {
    'a': ['ā','á','ǎ','à'], 'e': ['ē','é','ě','è'], 'i': ['ī','í','ǐ','ì'],
    'o': ['ō','ó','ǒ','ò'], 'u': ['ū','ú','ǔ','ù'], 'ü': ['ǖ','ǘ','ǚ','ǜ'],
  };
  const runes = [...syllable];
  let idx = runes.findIndex(r => r === 'a' || r === 'e');
  if (idx < 0) idx = runes.findIndex((r, i) => r === 'o' && runes[i + 1] === 'u');
  if (idx < 0) {
    const vowels = new Set(['a','e','i','o','u','ü']);
    for (let i = runes.length - 1; i >= 0; i--) {
      if (vowels.has(runes[i])) { idx = i; break; }
    }
  }
  if (idx < 0 || !toneMarks[runes[idx]]) return syllable;
  runes[idx] = toneMarks[runes[idx]][tone - 1];
  return runes.join('');
}

describe('numberedToToneMark', () => {
  it('applies tone 3 to a (ba3 → bǎ)', () => {
    expect(numberedToToneMark('ba3')).toBe('bǎ');
  });

  it('applies tone 1 to a in multi-vowel syllable (hao1 → hāo)', () => {
    expect(numberedToToneMark('hao1')).toBe('hāo');
  });

  it('applies tone 2 to e (he2 → hé)', () => {
    expect(numberedToToneMark('he2')).toBe('hé');
  });

  it('applies tone 4 to o in ou (dou4 → dòu)', () => {
    expect(numberedToToneMark('dou4')).toBe('dòu');
  });

  it('applies tone 3 to last vowel when no a/e/ou (ni3 → nǐ)', () => {
    expect(numberedToToneMark('ni3')).toBe('nǐ');
  });

  it('applies tone 4 to last vowel u (lu4 → lù)', () => {
    expect(numberedToToneMark('lu4')).toBe('lù');
  });

  it('treats v as ü and applies tone mark (lv3 → lǚ)', () => {
    expect(numberedToToneMark('lv3')).toBe('lǚ');
  });

  it('treats v as ü and applies tone mark (nv3 → nǚ)', () => {
    expect(numberedToToneMark('nv3')).toBe('nǚ');
  });

  it('treats colon notation as ü (nu:3 → nǚ)', () => {
    expect(numberedToToneMark('nu:3')).toBe('nǚ');
  });

  it('returns bare syllable with no mark for tone 5 (ma5 → ma)', () => {
    expect(numberedToToneMark('ma5')).toBe('ma');
  });

  it('normalizes v to ü for tone 5 (lv5 → lü)', () => {
    expect(numberedToToneMark('lv5')).toBe('lü');
  });

  it('applies tone 1 (zhong1 → zhōng)', () => {
    expect(numberedToToneMark('zhong1')).toBe('zhōng');
  });

  it('applies tone 2 (ren2 → rén)', () => {
    expect(numberedToToneMark('ren2')).toBe('rén');
  });

  it('applies tone 4 (shi4 → shì)', () => {
    expect(numberedToToneMark('shi4')).toBe('shì');
  });

  it('returns null for null input', () => {
    expect(numberedToToneMark(null)).toBe(null);
  });

  it('returns undefined for undefined input', () => {
    expect(numberedToToneMark(undefined)).toBe(undefined);
  });

  it('returns string unchanged when no tone digit at end', () => {
    expect(numberedToToneMark('ba')).toBe('ba');
  });

  it('lowercases uppercase input (BA3 → bǎ)', () => {
    expect(numberedToToneMark('BA3')).toBe('bǎ');
  });
});

// ── formatComponentPinyin ─────────────────────────────────────────────────────

function formatComponentPinyin(pinyinArr) {
  if (!pinyinArr || pinyinArr.length === 0) return '';
  return pinyinArr.map(numberedToToneMark).join(' / ');
}

describe('formatComponentPinyin', () => {
  it('returns empty string for null', () => {
    expect(formatComponentPinyin(null)).toBe('');
  });

  it('returns empty string for undefined', () => {
    expect(formatComponentPinyin(undefined)).toBe('');
  });

  it('returns empty string for empty array', () => {
    expect(formatComponentPinyin([])).toBe('');
  });

  it('converts a single-element array', () => {
    expect(formatComponentPinyin(['ba3'])).toBe('bǎ');
  });

  it('joins multiple readings with " / "', () => {
    expect(formatComponentPinyin(['ba3', 'ba2'])).toBe('bǎ / bá');
  });

  it('handles lv3 (ü via v) in an array', () => {
    expect(formatComponentPinyin(['lv3'])).toBe('lǚ');
  });

  it('handles nu:3 (ü via colon) in an array', () => {
    expect(formatComponentPinyin(['nu:3'])).toBe('nǚ');
  });

  it('handles tone 5 neutral in an array', () => {
    expect(formatComponentPinyin(['ma5'])).toBe('ma');
  });
});
