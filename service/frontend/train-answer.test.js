import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

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

function isZhCorrect(inputVal, prompt) {
  if (!inputVal || !inputVal.trim()) return false;
  return expandVariants(prompt).includes(normalizeAnswer(inputVal));
}

function isTransCorrect(inputVal, translations) {
  if (!inputVal || !inputVal.trim()) return false;
  const norm = normalizeAnswer(inputVal);
  const allTrans = Object.values(translations || {}).flat();
  return allTrans.some(t => expandVariants(t).includes(norm));
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

  it('accepts input without a bracketed annotation present in the prompt', () => {
    expect(isZhCorrect('过', '过（动词）')).toBe(true);
  });

  it('still accepts the full form including the bracketed annotation', () => {
    expect(isZhCorrect('过（动词）', '过（动词）')).toBe(true);
  });

  // Issue #343: the new-word-confirmation input never round-trips through the
  // backend — it's checked entirely client-side via this function — so any
  // ellipsis form the backend's normalize() would accept must also be
  // accepted here. Regression: this file's normalizeAnswer only stripped a
  // TRAILING punctuation run, so a mid-string ellipsis like "虽然……但是……"
  // was left untouched and never matched an equivalent "..." or "。。。" typed
  // answer.
  it('accepts an ASCII-dots ellipsis in place of the ideographic ellipsis mid-string', () => {
    expect(isZhCorrect('虽然...但是...', '虽然……但是……')).toBe(true);
  });

  it('accepts a fullwidth-period ellipsis in place of the ideographic ellipsis mid-string', () => {
    expect(isZhCorrect('虽然。。。但是。。。', '虽然……但是……')).toBe(true);
  });

  it('accepts the ideographic ellipsis when the prompt uses ASCII dots', () => {
    expect(isZhCorrect('虽然……但是……', '虽然...但是...')).toBe(true);
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

  it('accepts input without a bracketed annotation present in the translation', () => {
    expect(isTransCorrect('pass', { en: ['(to) pass'] })).toBe(true);
  });

  it('handles empty translations object', () => {
    expect(isTransCorrect('hello', {})).toBe(false);
  });

  it('handles null/undefined translations', () => {
    expect(isTransCorrect('hello', null)).toBe(false);
    expect(isTransCorrect('hello', undefined)).toBe(false);
  });
});

// ── Retype-on-wrong gate ─────────────────────────────────────────────────────
// Mirrors the pure helper added to train.js that decides whether the retype
// gate shown after a wrong answer is satisfied (reuses isZhCorrect/isTransCorrect,
// the same helpers the new-word introduction screen uses).

function wrongRetypeSatisfied(zhVal, transVal, correctZh, translations) {
  return isZhCorrect(zhVal, correctZh) && isTransCorrect(transVal, translations);
}

describe('wrongRetypeSatisfied', () => {
  it('returns true when both the Chinese word and translation are typed correctly', () => {
    expect(wrongRetypeSatisfied('你好', 'hello', '你好', { en: ['hello', 'hi'] })).toBe(true);
  });

  it('returns false when only the Chinese word is correct', () => {
    expect(wrongRetypeSatisfied('你好', 'nope', '你好', { en: ['hello'] })).toBe(false);
  });

  it('returns false when only the translation is correct', () => {
    expect(wrongRetypeSatisfied('错', 'hello', '你好', { en: ['hello'] })).toBe(false);
  });

  it('returns false when both are wrong', () => {
    expect(wrongRetypeSatisfied('错', 'nope', '你好', { en: ['hello'] })).toBe(false);
  });

  // Regression test for issue #348: a correct zh word with a parenthesised
  // part-of-speech annotation must satisfy the gate the same way its
  // checkmark indicates correctness — retyping just the bare word/translation
  // is enough, the annotation is optional (mirrors expandVariants/isZhCorrect).
  it('returns true for a word with a parenthesised annotation when the bare word is retyped', () => {
    expect(wrongRetypeSatisfied('花', 'ausgeben', '花（动词）', { en: ['spend'], de: ['ausgeben'] })).toBe(true);
  });
});

// ── wordModeLabel ────────────────────────────────────────────────────────────
// Mirrors the reciprocal of component.modeLabelAlsoWord: a word card whose
// character is also tracked as a component gets a suffix appended to its
// mode label (e.g. "To Chinese · Also a Component").

function wordModeLabel(baseLabel, isAlsoComponent, suffix) {
  return isAlsoComponent ? `${baseLabel} · ${suffix}` : baseLabel;
}

describe('wordModeLabel', () => {
  it('returns the base label unchanged when not also a component', () => {
    expect(wordModeLabel('To Chinese', false, 'Also a Component')).toBe('To Chinese');
  });

  it('appends the suffix when also a component', () => {
    expect(wordModeLabel('To Chinese', true, 'Also a Component')).toBe('To Chinese · Also a Component');
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

const FULLWIDTH_TO_HALFWIDTH = {
  '？': '?', '！': '!', '，': ',', '。': '.', '：': ':', '；': ';',
};

function normalizeAnswer(s) {
  s = s.toLowerCase().trim();
  s = s.replace(/[？！，。：；]/g, ch => FULLWIDTH_TO_HALFWIDTH[ch]);
  // Mirrors service/sm2/sm2.go's reDotsRun (issue #343): collapse any run of
  // halfwidth periods and/or ideographic ellipsis characters (U+2026),
  // anywhere in the string, into a single space.
  s = s.replace(/[.…]+/g, ' ');
  s = s.replace(/[\p{P}\p{S}\s]+$/u, '');
  return s.trim().split(/\s+/).join(' ');
}

function stripParens(s) {
  let prev;
  do {
    prev = s;
    s = s.replace(/\s*[(（][^()（）]*[)）]\s*/g, ' ').trim();
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
  it('removes a fullwidth-parenthesised segment', () => {
    expect(stripParens('过（动词）')).toBe('过');
  });
});

describe('expandVariants', () => {
  it('returns the normalized plain form', () => {
    expect(expandVariants('Hello')).toContain('hello');
  });
  it('includes the paren-stripped form', () => {
    expect(expandVariants('Morgen (5 Uhr bis 9 Uhr)')).toContain('morgen');
  });
  it('includes the fullwidth-paren-stripped form', () => {
    expect(expandVariants('过（动词）')).toContain('过');
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
