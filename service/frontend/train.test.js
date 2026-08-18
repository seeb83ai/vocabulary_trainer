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

// ── "Add as correct answer" language picker ───────────────────────────────────
// Mirrors the pure logic in train.js that decides which languages can be
// picked when attributing a wrong answer as a correct translation, and which
// one is pre-selected (issue #156: let the user choose the language, default
// to their primary language).

function buildAddTranslationLangOptions(selectedLangs, primaryLang) {
  const langs = selectedLangs.length > 0 ? selectedLangs : [primaryLang];
  const defaultLang = langs.includes(primaryLang) ? primaryLang : langs[0];
  return { langs, defaultLang };
}

describe('buildAddTranslationLangOptions', () => {
  it('offers both languages when two are selected', () => {
    const { langs } = buildAddTranslationLangOptions(['en', 'de'], 'en');
    expect(langs).toEqual(['en', 'de']);
  });

  it('defaults to the primary language when it is among the selected langs', () => {
    const { defaultLang } = buildAddTranslationLangOptions(['en', 'de'], 'en');
    expect(defaultLang).toBe('en');
  });

  it('defaults to the primary language regardless of selection order', () => {
    const { defaultLang } = buildAddTranslationLangOptions(['de', 'en'], 'en');
    expect(defaultLang).toBe('en');
  });

  it('falls back to the first selected lang when primary lang is not selected', () => {
    const { defaultLang } = buildAddTranslationLangOptions(['de'], 'en');
    expect(defaultLang).toBe('de');
  });

  it('falls back to the primary lang when no langs are selected', () => {
    const { langs, defaultLang } = buildAddTranslationLangOptions([], 'en');
    expect(langs).toEqual(['en']);
    expect(defaultLang).toBe('en');
  });
});

// ── New word input validation ──────────────────────────────────────────────────
// Mirrors the pure helpers added to train.js for the new-word input fields.

function normalizeNewWordInput(s) {
  return s.trim().toLowerCase();
}

function isZhCorrect(inputVal, prompt) {
  return inputVal.trim() === prompt.trim();
}

function isTransCorrect(inputVal, translations) {
  const normalized = normalizeNewWordInput(inputVal);
  if (!normalized) return false;
  const allTrans = Object.values(translations || {}).flat();
  return allTrans.some(t => normalizeNewWordInput(t) === normalized);
}

describe('isZhCorrect', () => {
  it('returns true for exact match', () => {
    expect(isZhCorrect('你好', '你好')).toBe(true);
  });

  it('trims whitespace from input', () => {
    expect(isZhCorrect('  你好  ', '你好')).toBe(true);
  });

  it('returns false for wrong characters', () => {
    expect(isZhCorrect('再见', '你好')).toBe(false);
  });

  it('returns false for empty input', () => {
    expect(isZhCorrect('', '你好')).toBe(false);
  });
});

describe('isTransCorrect', () => {
  it('returns true for exact EN match', () => {
    expect(isTransCorrect('hello', { en: ['hello', 'hi'], de: ['hallo'] })).toBe(true);
  });

  it('returns true for case-insensitive match', () => {
    expect(isTransCorrect('Hello', { en: ['hello'] })).toBe(true);
  });

  it('returns true for match in any language', () => {
    expect(isTransCorrect('hallo', { en: ['hello'], de: ['hallo'] })).toBe(true);
  });

  it('returns false for wrong translation', () => {
    expect(isTransCorrect('goodbye', { en: ['hello', 'hi'] })).toBe(false);
  });

  it('returns false for empty input', () => {
    expect(isTransCorrect('', { en: ['hello'] })).toBe(false);
  });

  it('returns false for whitespace-only input', () => {
    expect(isTransCorrect('   ', { en: ['hello'] })).toBe(false);
  });

  it('matches after trimming whitespace', () => {
    expect(isTransCorrect('  hello  ', { en: ['hello'] })).toBe(true);
  });

  it('handles empty translations object', () => {
    expect(isTransCorrect('hello', {})).toBe(false);
  });

  it('handles null/undefined translations', () => {
    expect(isTransCorrect('hello', null)).toBe(false);
    expect(isTransCorrect('hello', undefined)).toBe(false);
  });
});

// ── shouldAutoPlay ──────────────────────────────────────────────────────────────
// Mirrors the pure eligibility check in train.js that decides whether a newly
// shown card should trigger auto-play audio (when the user has the auto-play
// toggle enabled). Must never fire for transl_to_zh (would reveal the answer)
// or hmm cards (no audio exists).

function shouldAutoPlay(currentCard) {
  if (!currentCard) return false;
  if (currentCard.mode === 'new_word') return true;
  if (currentCard.card_type === 'component') return true;
  if (currentCard.card_type === 'hmm') return false;
  return currentCard.mode === 'zh_to_transl' || currentCard.mode === 'zh_pinyin_to_transl' || currentCard.mode === 'voice_to_transl';
}

describe('shouldAutoPlay', () => {
  it('returns true for zh_to_transl mode', () => {
    expect(shouldAutoPlay({ mode: 'zh_to_transl' })).toBe(true);
  });

  it('returns true for zh_pinyin_to_transl mode', () => {
    expect(shouldAutoPlay({ mode: 'zh_pinyin_to_transl' })).toBe(true);
  });

  it('returns false for transl_to_zh mode (would reveal the answer)', () => {
    expect(shouldAutoPlay({ mode: 'transl_to_zh' })).toBe(false);
  });

  it('returns true for the new-word introduction screen', () => {
    expect(shouldAutoPlay({ mode: 'new_word' })).toBe(true);
  });

  it('returns true for a component card (including new-component introduction)', () => {
    expect(shouldAutoPlay({ card_type: 'component', is_new: true })).toBe(true);
    expect(shouldAutoPlay({ card_type: 'component', is_new: false })).toBe(true);
  });

  it('returns false for hmm cards (no audio exists)', () => {
    expect(shouldAutoPlay({ card_type: 'hmm' })).toBe(false);
  });

  it('returns false when there is no current card', () => {
    expect(shouldAutoPlay(null)).toBe(false);
    expect(shouldAutoPlay(undefined)).toBe(false);
  });

  it('returns false for the Chinese (no sound) mode', () => {
    expect(shouldAutoPlay({ mode: 'zh_to_transl_no_sound' })).toBe(false);
  });

  it('returns true for voice_to_transl mode', () => {
    expect(shouldAutoPlay({ mode: 'voice_to_transl' })).toBe(true);
  });
});

// ── shouldAutoPlayResult ────────────────────────────────────────────────────────
// Mirrors the pure eligibility check in train.js that decides whether the
// result screen should read out the Chinese answer. Fires whenever auto-play
// is on and the answer wasn't already read out on the question screen (either
// because the mode never plays audio there, e.g. transl_to_zh or
// zh_to_transl_no_sound, or because the question-screen play was skipped for
// some other reason, e.g. the blur guard in autoPlayCard) — except for hmm
// cards which have no audio at all (issue #272).

function shouldAutoPlayResult(currentCard, autoPlayEnabled, alreadyPlayed) {
  if (!autoPlayEnabled || !currentCard) return false;
  if (currentCard.card_type === 'hmm') return false;
  return !alreadyPlayed;
}

describe('shouldAutoPlayResult', () => {
  it('returns true for transl_to_zh when auto-play is enabled and not already played', () => {
    expect(shouldAutoPlayResult({ mode: 'transl_to_zh' }, true, false)).toBe(true);
  });

  it('returns false for transl_to_zh when auto-play is disabled', () => {
    expect(shouldAutoPlayResult({ mode: 'transl_to_zh' }, false, false)).toBe(false);
  });

  it('returns true for other modes when not already played on the question screen', () => {
    expect(shouldAutoPlayResult({ mode: 'zh_to_transl' }, true, false)).toBe(true);
    expect(shouldAutoPlayResult({ mode: 'zh_pinyin_to_transl' }, true, false)).toBe(true);
  });

  it('returns false when already played on the question screen', () => {
    expect(shouldAutoPlayResult({ mode: 'zh_to_transl' }, true, true)).toBe(false);
  });

  it('returns true for zh_to_transl_no_sound on result screen (question screen was silent, result reveals the answer)', () => {
    expect(shouldAutoPlayResult({ mode: 'zh_to_transl_no_sound' }, true, false)).toBe(true);
  });

  it('returns false for hmm cards (no audio exists)', () => {
    expect(shouldAutoPlayResult({ card_type: 'hmm' }, true, false)).toBe(false);
  });

  it('returns true for component cards when not already played', () => {
    expect(shouldAutoPlayResult({ card_type: 'component', mode: undefined }, true, false)).toBe(true);
  });

  it('returns false for new_word when already marked as played (no result screen is shown for it in practice)', () => {
    expect(shouldAutoPlayResult({ mode: 'new_word' }, true, true)).toBe(false);
  });

  it('returns false when there is no current card', () => {
    expect(shouldAutoPlayResult(null, true, false)).toBe(false);
    expect(shouldAutoPlayResult(undefined, true, false)).toBe(false);
  });
});

// ── isZhPromptWithSound ────────────────────────────────────────────────────────
// Mirrors the pure play-button-visibility check in train.js's showCard(): the
// button is only shown for the two modes where the Chinese prompt has audio.
// zh_to_transl_no_sound deliberately behaves like a Chinese prompt for every
// other purpose but is excluded here on purpose, not by accidental omission.

function isZhPromptWithSound(mode) {
  return mode === 'zh_to_transl' || mode === 'zh_pinyin_to_transl' || mode === 'voice_to_transl';
}

describe('isZhPromptWithSound', () => {
  it('returns true for zh_to_transl', () => {
    expect(isZhPromptWithSound('zh_to_transl')).toBe(true);
  });

  it('returns true for zh_pinyin_to_transl', () => {
    expect(isZhPromptWithSound('zh_pinyin_to_transl')).toBe(true);
  });

  it('returns true for voice_to_transl', () => {
    expect(isZhPromptWithSound('voice_to_transl')).toBe(true);
  });

  it('returns false for zh_to_transl_no_sound', () => {
    expect(isZhPromptWithSound('zh_to_transl_no_sound')).toBe(false);
  });

  it('returns false for transl_to_zh', () => {
    expect(isZhPromptWithSound('transl_to_zh')).toBe(false);
  });

  it('returns false for an unknown mode', () => {
    expect(isZhPromptWithSound('cycle')).toBe(false);
  });
});

// ── isAutoPlayBlockedByBlur ───────────────────────────────────────────────────
// Mirrors the noAutoVoiceOnBlur guard inside autoPlayCard(). Intro screens
// (new_word, new-component) never blur their pinyin display (see train.js's
// setText('new-word-pinyin', ...) / setText('new-component-pinyin', ...),
// which never route through applyPinyinBlur()), so the guard must not apply
// to them — regression for issue #273 ("Sound missing" on the new-word screen).

function isVoiceOnlyMode(mode) {
  return mode === 'voice_to_transl';
}

function isAutoPlayBlockedByBlur(currentCard, noAutoVoiceOnBlur, blurPinyin) {
  const isIntroScreen = currentCard.mode === 'new_word' ||
    (currentCard.card_type === 'component' && currentCard.is_new);
  if (isIntroScreen || isVoiceOnlyMode(currentCard.mode)) return false;
  return noAutoVoiceOnBlur && (blurPinyin || !currentCard.pinyin);
}

describe('isAutoPlayBlockedByBlur', () => {
  it('does not block a new_word intro with blur and no-auto-voice-on-blur both on', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'new_word', pinyin: 'bīngxiāng' }, true, true)).toBe(false);
  });

  it('does not block a new_word intro with no pinyin at all', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'new_word', pinyin: undefined }, true, false)).toBe(false);
  });

  it('does not block a new-component intro with blur and no-auto-voice-on-blur both on', () => {
    expect(isAutoPlayBlockedByBlur({ card_type: 'component', is_new: true, pinyin: 'bīng' }, true, true)).toBe(false);
  });

  it('still blocks a regular (already-reviewed) component with blur and no-auto-voice-on-blur both on', () => {
    expect(isAutoPlayBlockedByBlur({ card_type: 'component', is_new: false, pinyin: 'bīng' }, true, true)).toBe(true);
  });

  it('still blocks a regular zh_to_transl card with blur and no-auto-voice-on-blur both on', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'zh_to_transl', pinyin: 'nǐhǎo' }, true, true)).toBe(true);
  });

  it('does not block a regular zh_to_transl card when no-auto-voice-on-blur is off', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'zh_to_transl', pinyin: 'nǐhǎo' }, false, true)).toBe(false);
  });

  it('does not block voice_to_transl (already exempt as voice-only mode)', () => {
    expect(isAutoPlayBlockedByBlur({ mode: 'voice_to_transl', pinyin: 'nǐhǎo' }, true, true)).toBe(false);
  });
});

// ── levenshtein distance ───────────────────────────────────────────────────────
// Standard DP edit-distance; inlined per project convention (tests are self-contained).

function levenshtein(a, b) {
  const m = a.length, n = b.length;
  const dp = Array.from({ length: m + 1 }, (_, i) =>
    Array.from({ length: n + 1 }, (_, j) => i === 0 ? j : j === 0 ? i : 0)
  );
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      if (a[i - 1] === b[j - 1]) {
        dp[i][j] = dp[i - 1][j - 1];
      } else {
        dp[i][j] = 1 + Math.min(dp[i - 1][j - 1], dp[i - 1][j], dp[i][j - 1]);
      }
    }
  }
  return dp[m][n];
}

describe('levenshtein', () => {
  it('returns 1 for a single deletion', () => {
    expect(levenshtein('hello', 'helo')).toBe(1);
  });

  it('returns 4 for unrelated short words', () => {
    expect(levenshtein('hello', 'world')).toBe(4);
  });

  it('returns 1 for empty vs single char', () => {
    expect(levenshtein('', 'a')).toBe(1);
  });

  it('returns 0 for identical strings', () => {
    expect(levenshtein('abc', 'abc')).toBe(0);
  });

  it('returns 1 for a single substitution', () => {
    expect(levenshtein('cat', 'bat')).toBe(1);
  });

  it('returns 1 for a single insertion', () => {
    expect(levenshtein('helo', 'hello')).toBe(1);
  });
});

// ── shouldShowAcceptBtn ────────────────────────────────────────────────────────
// Shared typo-tolerance helper; inlined per project convention.

function levenshteinForAccept(a, b) {
  const m = a.length, n = b.length;
  const dp = Array.from({ length: m + 1 }, (_, i) =>
    Array.from({ length: n + 1 }, (_, j) => i === 0 ? j : j === 0 ? i : 0)
  );
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      if (a[i - 1] === b[j - 1]) {
        dp[i][j] = dp[i - 1][j - 1];
      } else {
        dp[i][j] = 1 + Math.min(dp[i - 1][j - 1], dp[i - 1][j], dp[i][j - 1]);
      }
    }
  }
  return dp[m][n];
}

function shouldShowAcceptBtn(answer, normCorrects, mode) {
  if (!answer || answer.trim() === '') return false;
  if (mode === 'always') return true;
  if (mode === 'typo') return normCorrects.some(c => levenshteinForAccept(answer.toLowerCase().trim(), c.toLowerCase().trim()) === 1);
  return false;
}

describe('shouldShowAcceptBtn', () => {
  it('returns false when mode is never', () => {
    expect(shouldShowAcceptBtn('helo', ['hello'], 'never')).toBe(false);
  });

  it('returns true when mode is always and answer is non-empty', () => {
    expect(shouldShowAcceptBtn('anything', ['hello'], 'always')).toBe(true);
  });

  it('returns false when mode is always but answer is empty', () => {
    expect(shouldShowAcceptBtn('', ['hello'], 'always')).toBe(false);
  });

  it('returns true when mode is typo and distance is 1', () => {
    expect(shouldShowAcceptBtn('helo', ['hello'], 'typo')).toBe(true);
  });

  it('returns false when mode is typo and distance is 2', () => {
    expect(shouldShowAcceptBtn('helo', ['hellox'], 'typo')).toBe(false);
  });

  it('returns true when mode is typo and one of multiple corrects matches within 1', () => {
    expect(shouldShowAcceptBtn('wman', ['man', 'woman'], 'typo')).toBe(true);
  });

  it('returns false when mode is typo but answer is empty', () => {
    expect(shouldShowAcceptBtn('', ['hello'], 'typo')).toBe(false);
  });

  it('compares case-insensitively (pre-normalised input)', () => {
    expect(shouldShowAcceptBtn('helo', ['Hello'], 'typo')).toBe(true);
  });
});

// ── normalizeAnswer / stripParens / expandVariants / shouldShowAcceptTypo ──────
// New helpers and the quiz-mode-aware typo gate; inlined per project convention.

function normalizeAnswer(s) {
  s = s.toLowerCase().trim();
  s = s.replace(/[\p{P}\p{S}\s]+$/u, '');
  return s;
}

function stripParens(s) {
  let prev;
  do {
    prev = s;
    s = s.replace(/\s*\([^()]*\)\s*/g, ' ').trim();
  } while (s !== prev);
  return s;
}

function expandVariants(a) {
  const seen = new Set();
  const add = s => { const n = normalizeAnswer(s); if (n) seen.add(n); };
  add(a);
  const noParens = stripParens(a);
  add(noParens);
  for (const base of [a, noParens]) {
    for (const part of base.split(/[/,]/)) {
      add(part);
      add(stripParens(part));
    }
  }
  return [...seen];
}

function shouldShowAcceptTypo(answer, result, mode, cardMode) {
  if (!answer || answer.trim() === '') return false;
  if (mode === 'always') return true;
  if (mode !== 'typo') return false;
  if (cardMode === 'transl_to_zh') {
    if (result.user_answer_pinyin && result.pinyin) {
      return levenshtein(result.user_answer_pinyin.toLowerCase().trim(),
                         result.pinyin.toLowerCase().trim()) <= 1;
    }
    // Pinyin unavailable (typed word not in vocab): fall back to character comparison.
    const norm = normalizeAnswer(answer);
    const variants = (result.correct_answers || []).flatMap(expandVariants);
    return variants.some(c => levenshtein(norm, c) === 1);
  }
  const norm = normalizeAnswer(answer);
  const variants = (result.correct_answers || []).flatMap(expandVariants);
  return variants.some(c => levenshtein(norm, c) === 1);
}

describe('normalizeAnswer', () => {
  it('lowercases and trims whitespace', () => {
    expect(normalizeAnswer('  Hello  ')).toBe('hello');
  });
  it('strips trailing period', () => {
    expect(normalizeAnswer('hello.')).toBe('hello');
  });
  it('strips trailing closing paren', () => {
    // Only the trailing ) is stripped; inner content stays
    expect(normalizeAnswer('morning (5am to 9 am)')).toBe('morning (5am to 9 am');
  });
  it('leaves internal content intact', () => {
    expect(normalizeAnswer('good morning')).toBe('good morning');
  });
});

describe('stripParens', () => {
  it('removes a parenthesised segment', () => {
    expect(stripParens('Morgen (5 Uhr bis 9 Uhr)')).toBe('Morgen');
  });
  it('removes multiple parenthesised segments', () => {
    expect(stripParens('a (b) and (c)')).toBe('a and');
  });
  it('is a no-op when no parens present', () => {
    expect(stripParens('hello')).toBe('hello');
  });
  it('handles nested parens iteratively', () => {
    expect(stripParens('a (b (c))')).toBe('a');
  });
});

describe('expandVariants', () => {
  it('returns the normalized plain form', () => {
    expect(expandVariants('Hello')).toContain('hello');
  });
  it('includes the paren-stripped form', () => {
    expect(expandVariants('Morgen (5 Uhr bis 9 Uhr)')).toContain('morgen');
  });
  it('splits on slash', () => {
    const vs = expandVariants('hi/hello');
    expect(vs).toContain('hi');
    expect(vs).toContain('hello');
  });
  it('combines paren stripping and slash splitting', () => {
    const vs = expandVariants('good morning (greeting)/morning');
    expect(vs).toContain('good morning');
    expect(vs).toContain('morning');
  });
  it('splits on comma', () => {
    // Regression for #189: "topic, item" stored as one translation must
    // accept "item" on its own, mirroring the backend expandVariants fix.
    const vs = expandVariants('topic, item');
    expect(vs).toContain('topic');
    expect(vs).toContain('item');
  });
});

describe('shouldShowAcceptTypo', () => {
  // ── mode gate ──────────────────────────────────────────────────────────────
  it('returns false for empty answer regardless of mode', () => {
    expect(shouldShowAcceptTypo('', { correct_answers: ['hello'] }, 'typo', 'zh_to_transl')).toBe(false);
    expect(shouldShowAcceptTypo('  ', { pinyin: 'nǐ hǎo', user_answer_pinyin: 'nǐ hǎo' }, 'typo', 'transl_to_zh')).toBe(false);
  });
  it('returns true for any non-empty answer when mode is always', () => {
    expect(shouldShowAcceptTypo('anything', { correct_answers: ['hello'] }, 'always', 'zh_to_transl')).toBe(true);
  });
  it('returns false when mode is never', () => {
    expect(shouldShowAcceptTypo('helo', { correct_answers: ['hello'] }, 'never', 'zh_to_transl')).toBe(false);
  });

  // ── zh_to_transl: string comparison with variant expansion ─────────────────
  it('zh_to_transl: true when answer is 1 char off from a correct variant', () => {
    expect(shouldShowAcceptTypo('helo', { correct_answers: ['hello'] }, 'typo', 'zh_to_transl')).toBe(true);
  });
  it('zh_to_transl: true when correct answer has parenthetical suffix — Mirgen / Morgen case', () => {
    expect(shouldShowAcceptTypo('Mirgen', {
      correct_answers: ['morning', 'Morgen (5 Uhr bis 9 Uhr)', 'morning (5am to 9 am)'],
    }, 'typo', 'zh_to_transl')).toBe(true);
  });
  it('zh_to_transl: true when correct answer has slash alternatives', () => {
    expect(shouldShowAcceptTypo('helo', { correct_answers: ['hi/hello'] }, 'typo', 'zh_to_transl')).toBe(true);
  });
  it('zh_to_transl: false when stripped variants are all more than 1 away', () => {
    expect(shouldShowAcceptTypo('xyz', {
      correct_answers: ['morning (5am to 9 am)', 'Morgen (5 Uhr bis 9 Uhr)'],
    }, 'typo', 'zh_to_transl')).toBe(false);
  });

  // ── transl_to_zh: pinyin comparison ────────────────────────────────────────
  it('transl_to_zh: true when pinyin differs by 1 char — tone slip (书 shū vs 数 shù)', () => {
    // 看书 kàn shū vs 看数 kàn shù — only the tone mark on the last vowel differs
    expect(shouldShowAcceptTypo('看数', {
      pinyin: 'kàn shū',
      user_answer_pinyin: 'kàn shù',
    }, 'typo', 'transl_to_zh')).toBe(true);
  });
  it('transl_to_zh: true when pinyins are identical (homophones)', () => {
    expect(shouldShowAcceptTypo('你好', {
      pinyin: 'nǐ hǎo',
      user_answer_pinyin: 'nǐ hǎo',
    }, 'typo', 'transl_to_zh')).toBe(true);
  });
  it('transl_to_zh: false when pinyins differ by more than 1 char', () => {
    expect(shouldShowAcceptTypo('大', {
      pinyin: 'rén',
      user_answer_pinyin: 'dà',
    }, 'typo', 'transl_to_zh')).toBe(false);
  });
  it('transl_to_zh: true via character fallback when pinyin absent — 看数 vs 看书 (1 char diff)', () => {
    // 看数 is not in vocab so user_answer_pinyin is null; fall back to raw character levenshtein
    expect(shouldShowAcceptTypo('看数', {
      correct_answers: ['看书'],
      pinyin: 'kàn shū',
      user_answer_pinyin: null,
    }, 'typo', 'transl_to_zh')).toBe(true);
  });
  it('transl_to_zh: false via character fallback when characters are far apart and pinyin absent', () => {
    expect(shouldShowAcceptTypo('完全不同', {
      correct_answers: ['看书'],
      pinyin: 'kàn shū',
      user_answer_pinyin: null,
    }, 'typo', 'transl_to_zh')).toBe(false);
  });
  it('transl_to_zh: false when correct pinyin is absent', () => {
    expect(shouldShowAcceptTypo('看数', {
      pinyin: null,
      user_answer_pinyin: 'kàn shù',
    }, 'typo', 'transl_to_zh')).toBe(false);
  });
});

// ── splitComponentDefs ─────────────────────────────────────────────────────────
// Mirrors the `,`/`;` splitting that CheckComponentAnswer does on the backend.

function splitComponentDefs(correctAnswersObj) {
  return Object.values(correctAnswersObj || {})
    .flatMap(def => def.split(/[,;]/))
    .map(s => s.toLowerCase().trim())
    .filter(s => s.length > 0);
}

describe('splitComponentDefs', () => {
  it('splits a single-lang definition on commas', () => {
    expect(splitComponentDefs({ en: 'son, child' })).toEqual(expect.arrayContaining(['son', 'child']));
  });

  it('splits on semicolons', () => {
    expect(splitComponentDefs({ en: 'son; child' })).toEqual(expect.arrayContaining(['son', 'child']));
  });

  it('combines alternatives from multiple languages', () => {
    const result = splitComponentDefs({ en: 'son, child', de: 'Sohn, Kind' });
    expect(result).toEqual(expect.arrayContaining(['son', 'child', 'sohn', 'kind']));
  });

  it('trims whitespace and lowercases', () => {
    const result = splitComponentDefs({ en: ' Kind ' });
    expect(result).toContain('kind');
  });

  it('filters out empty strings', () => {
    const result = splitComponentDefs({ en: ',,' });
    expect(result.every(s => s.length > 0)).toBe(true);
  });

  it('returns empty array for empty input', () => {
    expect(splitComponentDefs({})).toEqual([]);
  });
});

// ── due-today display count (#186) ─────────────────────────────────────────
// Mirrors dueDisplayCount in train.js: the backend may serve a not-yet-due
// "session extension" card to avoid immediately repeating a just-answered
// word; that card isn't counted in stats.due_today, so the displayed
// remaining count must add 1 back in when such a card is being shown.

function dueDisplayCount(stats, sessionExtension, newWordIntro = false) {
  return stats.due_today + (stats.hmm_due_today || 0) + (stats.components_due_today || 0)
    + (sessionExtension ? 1 : 0) + (newWordIntro ? 1 : 0);
}

describe('dueDisplayCount', () => {
  it('sums due_today, hmm_due_today, and components_due_today', () => {
    const stats = { due_today: 3, hmm_due_today: 2, components_due_today: 1 };
    expect(dueDisplayCount(stats, false)).toBe(6);
  });

  it('treats missing hmm/component counts as zero', () => {
    expect(dueDisplayCount({ due_today: 5 }, false)).toBe(5);
  });

  it('adds 1 when the current card is a session extension (non-due) card', () => {
    const stats = { due_today: 1, hmm_due_today: 0, components_due_today: 0 };
    expect(dueDisplayCount(stats, true)).toBe(2);
  });

  it('does not inflate the count when session extension is false', () => {
    const stats = { due_today: 0, hmm_due_today: 0, components_due_today: 0 };
    expect(dueDisplayCount(stats, false)).toBe(0);
  });

  it('adds 1 when the current card is a new-word introduction (issue #206)', () => {
    // The word being introduced has first_seen_at IS NULL so it is NOT counted
    // in stats.due_today.  The display must still show 1 so the user sees the
    // card they are actively working on reflected in the counter.
    const stats = { due_today: 0, hmm_due_today: 0, components_due_today: 0 };
    expect(dueDisplayCount(stats, false, true)).toBe(1);
  });

  it('combines session-extension and new-word-intro additions', () => {
    const stats = { due_today: 2, hmm_due_today: 0, components_due_today: 0 };
    expect(dueDisplayCount(stats, true, true)).toBe(4);
  });
});

// ── Match-game outcome (issue #215) ────────────────────────────────────────────
// Mirrors the decision logic in the right-box click handler in showMatchGame.

function matchGameOutcome(rightIdx, lIdx, rightText, leftTransls, matchedLeftIdxs) {
  if (rightIdx === lIdx) return 'correct';
  if (leftTransls.includes(rightText)) {
    return matchedLeftIdxs.has(rightIdx) ? 'correct' : 'blocked';
  }
  return 'wrong';
}

describe('matchGameOutcome', () => {
  it('is correct when the right box is the word\'s own translation', () => {
    expect(matchGameOutcome(0, 0, 'können', ['in der Lage sein', 'können'], new Set())).toBe('correct');
  });

  it('is wrong when the right text is not among the word\'s translations', () => {
    expect(matchGameOutcome(1, 0, 'möglicherweise', ['in der Lage sein', 'können'], new Set())).toBe('wrong');
  });

  it('is blocked when a shared translation is claimed but its true owner still needs it (issue #215)', () => {
    // 能 (lIdx=0) picks "können", which is 可能's (rightIdx=1) own box — 可能 not yet matched.
    expect(matchGameOutcome(1, 0, 'können', ['in der Lage sein', 'können'], new Set())).toBe('blocked');
  });

  it('is correct via shared translation once the true owner is already matched', () => {
    // 可能 (rightIdx=1) already matched elsewhere, so 能 (lIdx=0) may safely reuse "können".
    expect(matchGameOutcome(1, 0, 'können', ['in der Lage sein', 'können'], new Set([1]))).toBe('correct');
  });

  it('is correct via shared translation when two words have the exact same single translation', () => {
    expect(matchGameOutcome(1, 0, 'hello', ['hello'], new Set())).toBe('blocked');
    expect(matchGameOutcome(1, 0, 'hello', ['hello'], new Set([1]))).toBe('correct');
  });
});

// ── One-button onboarding quick start ─────────────────────────────────────────
// Mirrors quickStartPlan in train.js.

function quickStartPlan(tagNames) {
  const has = n => tagNames.includes(n);
  return { hsk1: has('hsk-1'), hsk23: ['hsk-2', 'hsk-3'].filter(has) };
}

describe('quickStartPlan', () => {
  it('offers both buttons when all HSK lists exist', () => {
    expect(quickStartPlan(['hsk-1', 'hsk-2', 'hsk-3', 'food'])).toEqual({
      hsk1: true,
      hsk23: ['hsk-2', 'hsk-3'],
    });
  });

  it('offers only HSK 1 when higher lists are missing', () => {
    expect(quickStartPlan(['hsk-1', 'travel'])).toEqual({ hsk1: true, hsk23: [] });
  });

  it('offers a partial basics import when only HSK 2 exists', () => {
    expect(quickStartPlan(['hsk-2'])).toEqual({ hsk1: false, hsk23: ['hsk-2'] });
  });

  it('offers nothing without HSK library tags', () => {
    expect(quickStartPlan(['food', 'travel'])).toEqual({ hsk1: false, hsk23: [] });
  });

  it('handles an empty tag list', () => {
    expect(quickStartPlan([])).toEqual({ hsk1: false, hsk23: [] });
  });
});

// ── End-of-session comeback info ──────────────────────────────────────────────
// Mirror computeDayStreak and dueTomorrowCount in train.js.

function computeDayStreak(days, today) {
  const trained = new Set((days || []).filter(d => d.attempts > 0).map(d => d.date));
  let streak = 0;
  const cur = new Date(today + 'T00:00:00Z');
  while (trained.has(cur.toISOString().slice(0, 10))) {
    streak++;
    cur.setUTCDate(cur.getUTCDate() - 1);
  }
  return streak;
}

function dueTomorrowCount(dates, tomorrow) {
  const hit = (dates || []).find(d => d.date === tomorrow);
  return hit ? hit.count : 0;
}

describe('computeDayStreak', () => {
  it('counts consecutive days ending today', () => {
    const days = [
      { date: '2026-08-13', attempts: 5 },
      { date: '2026-08-14', attempts: 2 },
      { date: '2026-08-15', attempts: 9 },
    ];
    expect(computeDayStreak(days, '2026-08-15')).toBe(3);
  });

  it('breaks the streak at a gap', () => {
    const days = [
      { date: '2026-08-12', attempts: 5 },
      { date: '2026-08-14', attempts: 2 },
      { date: '2026-08-15', attempts: 1 },
    ];
    expect(computeDayStreak(days, '2026-08-15')).toBe(2);
  });

  it('ignores zero-attempt days', () => {
    const days = [
      { date: '2026-08-14', attempts: 0 },
      { date: '2026-08-15', attempts: 1 },
    ];
    expect(computeDayStreak(days, '2026-08-15')).toBe(1);
  });

  it('returns 0 when today has no training', () => {
    expect(computeDayStreak([{ date: '2026-08-14', attempts: 3 }], '2026-08-15')).toBe(0);
  });

  it('handles unordered input and month boundaries', () => {
    const days = [
      { date: '2026-09-01', attempts: 1 },
      { date: '2026-08-31', attempts: 4 },
    ];
    expect(computeDayStreak(days, '2026-09-01')).toBe(2);
  });

  it('handles empty and missing input', () => {
    expect(computeDayStreak([], '2026-08-15')).toBe(0);
    expect(computeDayStreak(null, '2026-08-15')).toBe(0);
  });
});

describe('dueTomorrowCount', () => {
  const dates = [
    { date: '2026-08-15', count: 4 },
    { date: '2026-08-16', count: 7 },
  ];

  it('finds the count for tomorrow', () => {
    expect(dueTomorrowCount(dates, '2026-08-16')).toBe(7);
  });

  it('returns 0 when tomorrow has no due words', () => {
    expect(dueTomorrowCount(dates, '2026-08-17')).toBe(0);
  });

  it('handles empty and missing input', () => {
    expect(dueTomorrowCount([], '2026-08-16')).toBe(0);
    expect(dueTomorrowCount(null, '2026-08-16')).toBe(0);
  });
});
