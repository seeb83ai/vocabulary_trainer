// train-answer.js — pure answer-checking / normalization helpers

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

// fullwidthToHalfwidth mirrors service/sm2/sm2.go's fullwidthToHalfwidth
// Replacer so full-width punctuation is interchangeable with its ASCII form.
const FULLWIDTH_TO_HALFWIDTH = {
  '？': '?', '！': '!', '，': ',', '。': '.', '：': ':', '；': ';',
};

function normalizeAnswer(s) {
  s = s.toLowerCase().trim();
  s = s.replace(/[？！，。：；]/g, ch => FULLWIDTH_TO_HALFWIDTH[ch]);
  // Mirrors service/sm2/sm2.go's reDotsRun: collapse any run of halfwidth
  // periods and/or ideographic ellipsis characters (U+2026), anywhere in the
  // string, into a single space — so "……", "。。。" (after fullwidth
  // conversion above), and "..." are all treated as equivalent regardless of
  // position (issue #343).
  s = s.replace(/[.…]+/g, ' ');
  s = s.replace(/[\p{P}\p{S}\s]+$/u, '');
  s = s.trim().split(/\s+/).join(' ');
  return s;
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

// isZhCorrect/isTransCorrect back the "must type it correctly to continue"
// gates — shared by the new-word introduction screen and the
// retype-on-wrong-answer gate (see wrongRetypeSatisfied below). They reuse
// expandVariants/normalizeAnswer (same helpers backing shouldShowAcceptTypo
// above and the backend's CheckAnswer) so that both the punctuation/ellipsis
// handling (issue #343) and optional parenthesised annotations (e.g. "过（动词）")
// are stripped the same way here as in the checkmark UI — issue #348: the
// Next button used to stay disabled even after both fields showed a correct ✓
// checkmark because this file's gate used a different, stricter normalisation.
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

// wrongRetypeSatisfied decides whether the retype-on-wrong gate (shown after
// a wrong answer when the retype_on_wrong setting is enabled) is satisfied —
// the user must retype both the correct Chinese word and a correct translation.
function wrongRetypeSatisfied(zhVal, transVal, correctZh, translations) {
  return isZhCorrect(zhVal, correctZh) && isTransCorrect(transVal, translations);
}

// Returns true if a wrong answer should be offered as "accept as typo".
// For transl_to_zh mode compares pinyin strings (levenshtein ≤ 1) so that
// tone-slip / same-sound characters are caught instead of raw character diffs.
// For other modes expands correct-answer variants (strip parens, split on /
// or ,) before comparing — mirrors the backend expandVariants / CheckAnswer logic.
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

// Reciprocal of the component card's "Component & Word" label: a word card
// whose character is also tracked as a component gets a suffix appended.
function wordModeLabel(baseLabel, isAlsoComponent, suffix) {
  return isAlsoComponent ? `${baseLabel} · ${suffix}` : baseLabel;
}

function shouldShowAcceptBtn(answer, normCorrects, mode) {
  if (!answer || answer.trim() === '') return false;
  if (mode === 'always') return true;
  if (mode === 'typo') return normCorrects.some(c => levenshtein(answer.toLowerCase().trim(), c.toLowerCase().trim()) === 1);
  return false;
}

function buildAddTranslationLangOptions(selectedLangs, primaryLang) {
  const langs = selectedLangs.length > 0 ? selectedLangs : [primaryLang];
  const defaultLang = langs.includes(primaryLang) ? primaryLang : langs[0];
  return { langs, defaultLang };
}

// Splits a component correct_answers object ({lang: "def1, def2"}) into
// individual normalised alternatives, mirroring CheckComponentAnswer's `,`/`;` split.
function splitComponentDefs(correctAnswersObj) {
  return Object.values(correctAnswersObj || {})
    .flatMap(def => def.split(/[,;]/))
    .map(s => s.toLowerCase().trim())
    .filter(s => s.length > 0);
}
