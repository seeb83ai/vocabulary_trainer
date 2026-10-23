// train-card.js — core load/show/submit loop, top-level state, init wiring

// Training page logic

// Language settings loaded from /api/settings on init
let userPrimaryLang = 'en';
let userSecondaryLang = '';
let acceptCorrectMode = 'typo';
let skipNewWordsVisible = true;
let blurPinyin = false;
let noAutoVoiceOnBlur = false;
let celebrateBucketChange = false;
let skipRevealAnswer = false;
let voiceUnavailable = false; // session-only flag, set when user skips a voice card and opts out
let _gamificationEnabled = false;
let _gamificationFrequencyMs = 5 * 60 * 1000;
let _lastGameShownAt = 0;
let _matchGamePinyinReveal = 'always';
let _matchGameSm2Update = 'always';
let _matchGameShowSummary = true;
const _settingsPromise = fetch('/api/settings', { signal: timeoutSignal() }).then(r => r.ok ? r.json() : null).then(st => {
  if (st?.primary_lang) userPrimaryLang = st.primary_lang;
  userSecondaryLang = st?.secondary_lang ?? '';
  acceptCorrectMode = st?.accept_correct_mode ?? 'typo';
  skipNewWordsVisible = st?.skip_new_words_visible !== false;
  blurPinyin = !!st?.blur_pinyin;
  noAutoVoiceOnBlur = !!st?.no_auto_voice_on_blur;
  celebrateBucketChange = !!st?.celebrate_bucket_change;
  skipRevealAnswer = !!st?.skip_reveal_answer;
  _gamificationEnabled = !!st?.gamification_enabled;
  autoPlayEnabled = !!st?.autoplay_always;
  autofocusInput = shouldAutofocus(st);
  _gamificationFrequencyMs = (st?.gamification_frequency ?? 5) * 60 * 1000;
  _matchGamePinyinReveal = st?.match_game_pinyin_reveal || 'always';
  _matchGameSm2Update = st?.match_game_sm2_update || 'always';
  _matchGameShowSummary = st?.match_game_show_summary !== false;
  const btn = document.getElementById('new-word-skip-btn');
  if (btn && !skipNewWordsVisible) btn.classList.add('hidden');
  // Restore server-persisted training filter settings (overrides localStorage).
  // train_mode is always set (default 'random') so this reliably detects server support.
  if (st?.train_mode !== undefined) {
    selectedMode = st.train_mode || 'random';
    selectedBucket = st.train_bucket || '';
    if (Array.isArray(st.train_langs) && st.train_langs.length > 0) {
      selectedLangs = st.train_langs;
    }
    includeMnemonics = st.train_mnemonics !== false;
    includeComponents = st.train_components !== false;
    if (Array.isArray(st.train_tags)) selectedTags = st.train_tags;
    localStorage.setItem('quizMode', selectedMode);
    localStorage.setItem('quizBucket', selectedBucket);
    localStorage.setItem('quizLangs', JSON.stringify(selectedLangs));
    localStorage.setItem('quizMnemonics', includeMnemonics ? 'true' : 'false');
    localStorage.setItem('quizComponents', includeComponents ? 'true' : 'false');
    localStorage.setItem('quizTags', JSON.stringify(selectedTags));
  }
}).catch(() => {});

const HMM_TYPE_COLORS = {
  actor:     'bg-purple-100 text-purple-700',
  location:  'bg-blue-100 text-blue-700',
  tone_room: 'bg-amber-100 text-amber-700',
  prop:      'bg-emerald-100 text-emerald-700',
};

let currentCard = null;
let isSubmitted = false;
// Set while an ambiguous-answer disambiguation panel is shown and unresolved.
// Holds a function that renders the normal wrong-answer screen; consumed (and
// cleared) by the Next button so continuing without resolving the ambiguity
// falls back to the usual wrong-answer result instead of silently advancing.
let ambiguousUnresolved = null;
let selectedMode = localStorage.getItem('quizMode') || 'random';

// IDs of the last two answered vocabulary words, used to avoid immediate re-show.
let recentWordIDs = [];
// Characters of the last two answered components, used to avoid immediate re-show.
let recentComponentChars = [];

// ── Training-time tracking ──────────────────────────────────────────────────
// Counts seconds while this tab is visible, the window has focus, and a card
// is available to train. Timer pauses on success-state / empty-state.
let _trainStartMs = null;
let _pendingSeconds = 0;
let _noCardsPaused = false; // true while no cards are due (success/empty state)

function _isTrainActive() {
  return document.visibilityState === 'visible' && document.hasFocus() && !_noCardsPaused;
}

function _onFocusOrVisibility() {
  if (_isTrainActive()) {
    if (_trainStartMs === null) _trainStartMs = Date.now();
  } else if (_trainStartMs !== null) {
    _pendingSeconds += Math.floor((Date.now() - _trainStartMs) / 1000);
    _trainStartMs = null;
  }
}

async function _flushTime() {
  if (_trainStartMs !== null) {
    _pendingSeconds += Math.floor((Date.now() - _trainStartMs) / 1000);
    _trainStartMs = null;
  }
  // Restart the timer immediately so time keeps accumulating across card loads
  if (_isTrainActive()) _trainStartMs = Date.now();
  if (_pendingSeconds <= 0) return;
  const secs = _pendingSeconds;
  _pendingSeconds = 0;
  try {
    await apiFetch('/api/quiz/record-time', { method: 'POST', body: JSON.stringify({ seconds: secs }) });
  } catch (_) {}
}

document.addEventListener('visibilitychange', _onFocusOrVisibility);
window.addEventListener('focus', _onFocusOrVisibility);
window.addEventListener('blur', _onFocusOrVisibility);
window.addEventListener('beforeunload', () => {
  if (_trainStartMs !== null) {
    _pendingSeconds += Math.floor((Date.now() - _trainStartMs) / 1000);
    _trainStartMs = null;
  }
  if (_pendingSeconds <= 0) return;
  const secs = _pendingSeconds;
  _pendingSeconds = 0;
  navigator.sendBeacon(
    '/api/quiz/record-time',
    new Blob([JSON.stringify({ seconds: secs })], { type: 'application/json' }),
  );
});
document.addEventListener('DOMContentLoaded', () => {
  if (_isTrainActive()) _trainStartMs = Date.now();
});
// ── End training-time tracking ───────────────────────────────────────────────

// Phone: the chip + sound + fullscreen buttons sit in the shell's top bar.
document.addEventListener('DOMContentLoaded', () => {
  const actions = document.querySelector('#session-bar .tr-bar-actions');
  const topbar = document.getElementById('app-topbar');
  if (!actions || !topbar) return;
  const home = actions.parentElement;
  const mq = window.matchMedia('(max-width: 879px)');
  const place = () => (mq.matches ? topbar : home).appendChild(actions);
  mq.addEventListener('change', place);
  place();
});
let selectedTags = JSON.parse(localStorage.getItem('quizTags') || '[]');
let selectedBucket = localStorage.getItem('quizBucket') || '';
let selectedLangs = JSON.parse(localStorage.getItem('quizLangs') || '["en"]');
let includeMnemonics = localStorage.getItem('quizMnemonics') !== 'false';
let includeComponents = localStorage.getItem('quizComponents') !== 'false';
let latestStats = null;

// Auto-play toggle: in-memory only, never persisted — always resets to off
// on page load/reload so it doesn't surprise the user across sessions.
let autoPlayEnabled = false;
let autofocusInput = true;
// Tracks whether audio actually started playing for the current card via
// autoPlayCard, so the result screen knows whether to play it there instead
// (issue #272). Reset whenever loadNextCard replaces currentCard.
let questionAutoPlayed = false;
// True once the user pressed play on the question screen of the current card.
let questionAudioPlayed = false;
// The help the user had before the last word answer (pinyin seen, audio
// played, play pressed by the user) and whether it was a drill answer. Sent with the answer and again
// with accept-correct, so the server counts both in the same answer buckets.
let answerHelp = {};

let skipNewWords = false;
let requireNewWordZh = true;
let requireNewWordTrans = true;
let wrongAnswerRetryMode = 'off';
// Holds { zhText, translations } for the currently rendered wrong-answer
// result while the retype-on-wrong gate is active; read by the top-level
// wrong-retype input listeners set up in DOMContentLoaded.
let wrongRetypeTarget = null;
// Wrong answers in a row per card in this session (issue #554). From the
// SKIP_AFTER_WRONG-th one, the wrong-answer screen offers "Skip for today".
const SKIP_AFTER_WRONG = 3;
const wrongStreaks = new Map();

function cardStreakKey(card) {
  if (card.card_type === 'hmm') return `hmm:${card.entity_type}:${card.entity_key}`;
  if (card.card_type === 'component') return `component:${card.prompt}`;
  return `word:${card.word_id}`;
}

// Updates the count of wrong answers in a row for key and returns it.
function recordWrongStreak(streaks, key, correct) {
  const n = correct ? 0 : (streaks.get(key) || 0) + 1;
  if (n) streaks.set(key, n);
  else streaks.delete(key);
  return n;
}
// Difficult-words drill: when active, /api/quiz/next is queried with difficult=true
// and only flagged (hardest) words are served until each is answered correctly.
let difficultDrill = localStorage.getItem('quizDifficultDrill') === 'true';

let obTagsLoaded = false;
function showEmptyState() {
  show('empty-state');
  if (!obTagsLoaded) {
    obTagsLoaded = true;
    // obLoadTags is defined inside DOMContentLoaded; call lazily via event
    document.dispatchEvent(new CustomEvent('ob:loadtags'));
  }
}

// Scrolls the given card container to the top of the viewport. Called
// whenever a new card/word/component is shown so the prompt is always
// fully visible right away, instead of relying on incidental scrolling
// (e.g. an input's auto-focus), which only happened sometimes (issue #374).
function scrollCardIntoView(id) {
  $(id)?.scrollIntoView({ behavior: 'auto', block: 'start' });
}

// How far to scroll down so that [top, bottom] (viewport-relative px) fits in
// a visible area of visibleHeight px, keeping `margin` px free below. Never
// scrolls up, and never pushes the top edge out of view (issue #512).
function scrollDeltaToReveal(top, bottom, visibleHeight, margin = 12) {
  const limit = visibleHeight - margin;
  if (bottom <= limit) return 0;
  return Math.max(0, Math.min(bottom - limit, top - margin));
}

// On touch devices the on-screen keyboard covers part of the page. Scrolls
// the answer input and its Check button into the area the keyboard leaves
// free (issue #512).
function revealAnswerForm() {
  if (!window.matchMedia('(pointer: coarse)').matches) return;
  const form = $('answer-form');
  if (!form || document.activeElement !== $('answer-input')) return;
  const vv = window.visualViewport;
  const offsetTop = vv ? vv.offsetTop : 0;
  const rect = form.getBoundingClientRect();
  const delta = scrollDeltaToReveal(rect.top - offsetTop, rect.bottom - offsetTop, vv ? vv.height : window.innerHeight);
  if (delta > 0) window.scrollBy({ top: delta, behavior: 'smooth' });
}

// watchImports shows the progress of a background import above the card until
// it is finished.
function watchImports() {
  watchActiveImports(summary => {
    if (!summary) {
      hide('import-progress');
      return;
    }
    setText('import-progress', importProgressText(summary));
    show('import-progress');
  });
}

// A filter change starts a new load before the old one ends. The newest load
// wins, so an older card can't render over a newer one.
const nextCardRequest = latestRequest();

async function loadNextCard(trackCurrent = false) {
  const signal = nextCardRequest();
  _noCardsPaused = true;  // pause until we confirm a card is ready
  // A clip of the card we leave must not play on the next screen (#620).
  stopAudio();
  // Every way off a result screen (Next, accept as correct, add translation)
  // ends here; only renderWordAnswerResult shows the leech box again.
  hide('leech-box');
  // Track the word we're leaving so it isn't immediately re-shown.
  // Only track regular vocabulary cards (not new-word introductions, HMM, or components).
  if (trackCurrent && currentCard?.word_id && !currentCard.card_type && currentCard.mode !== 'new_word') {
    recentWordIDs = [currentCard.word_id, ...recentWordIDs].slice(0, 2);
  }
  // Same tracking for components, so a wrong-answered component isn't
  // immediately re-shown while another component is also due (#391).
  if (trackCurrent && currentCard?.card_type === 'component' && !currentCard.is_new) {
    recentComponentChars = [currentCard.prompt, ...recentComponentChars].slice(0, 2);
  }
  isSubmitted = false;
  ambiguousUnresolved = null;
  restoreResultQuestion();
  hide('card-area');
  hide('result-area');
  hide('celebration-screen');
  hide('empty-state');
  hide('success-state');
  hide('error-state');
  hide('add-translation-row');
  hide('add-translation-lang-select');
  hide('accept-correct-btn');
  hide('result-play-btn');
  hide('new-word-area');
  hide('new-component-area');
  hide('result-question');
  hide('result-sentence');
  hide('result-decompose');
  hide('result-decompose-content');
  hide('bucket-info');
  hide('streak-info');
  hide('streak-dots');
  hide('result-subtitle');
  hide('wrong-retype-area');
  hide('result-skip-today-row');
  wrongRetypeTarget = null;
  $('next-btn').disabled = false;
  $('answer-input').value = '';
  const reviewBtn = $('needs-review-btn');
  renderFlagButton(reviewBtn, false);
  reviewBtn.disabled = false;
  reviewBtn.onclick = null;
  // The screens are hidden before the first await, so the button that
  // started this load is gone before withBusyButton enables it again.
  await _flushTime();
  if (signal.aborted) return;

  // Fetch fresh stats first. The backend's GetNextCard may return non-due
  // (future) cards via its fallback queries even when due_today = 0, so we
  // cannot rely solely on a 404 "no words available" to trigger the success
  // screen — we must check due_today proactively.
  await loadStats();
  if (signal.aborted) return;

  if (latestStats) {
    if (latestStats.total === 0) {
      await exitDifficultDrill(false);
      showEmptyState();
      return;
    }
    // While drilling difficult words we bypass the "all done" screen and serve
    // flagged words regardless of their due date.
    if (!difficultDrill && latestStats.due_today === 0 && (latestStats.hmm_due_today || 0) === 0 && (latestStats.components_due_today || 0) === 0 && (!latestStats.new_available || skipNewWords)) {
      skipNewWords = false;
      setText('success-stats', t('stats.attemptsAndMistakes', { attempts: latestStats.today_attempts, mistakes: latestStats.today_mistakes }));
      document.querySelectorAll('.advance-btn').forEach(btn => {
        btn.disabled = latestStats.available_to_advance === 0;
      });
      updateAdvanceButtonsForDifficult();
      const { showIntroduceNew } = successAdvanceState(latestStats);
      showIntroduceNew ? show('introduce-new-btn') : hide('introduce-new-btn');
      show('success-state');
      loadComebackInfo(latestStats.words_improved_today);
      loadWrongToday();
      return;
    }
  }

  try {
    const params = new URLSearchParams();
    if (selectedMode !== 'random') params.set('mode', selectedMode);
    if (selectedTags.length) params.set('tags', selectedTags.join(','));
    if (selectedBucket) params.set('bucket', selectedBucket);
    if (selectedLangs.length) params.set('langs', selectedLangs.join(','));
    if (skipNewWords) params.set('skip_new', 'true');
    if (!includeMnemonics) params.set('mnemonics', 'false');
    if (includeComponents) params.set('trainComponents', '1');
    if (recentWordIDs.length) params.set('exclude', recentWordIDs.join(','));
    if (recentComponentChars.length) params.set('exclude_components', recentComponentChars.join(','));
    if (difficultDrill) params.set('difficult', 'true');
    const qs = params.toString();
    const url = qs ? `/api/quiz/next?${qs}` : '/api/quiz/next';
    currentCard = await apiFetch(url, { signal });
    if (currentCard?.mode) currentCard.mode = voiceFallbackMode(currentCard.mode, voiceUnavailable);
    questionAutoPlayed = false;
    questionAudioPlayed = false;
    // The served card was pulled in from beyond today's due-date bound solely
    // to avoid repeating a just-answered word — it isn't reflected in
    // latestStats.due_today, so bump the displayed count by 1 to match.
    if (currentCard?.session_extension && latestStats) {
      setText('stats-due', dueDisplayCount(latestStats, true));
    }
  } catch (e) {
    if (isAbortError(e)) return;
    hide('card-area');
    if (e.message === 'no words available' && difficultDrill) {
      // The drill pool is exhausted — leave the drill and fall back to the
      // normal "all done" / next-card flow.
      await exitDifficultDrill(false);
      return loadNextCard();
    }
    if (e.message === 'no words available') {
      // latestStats was fetched above; if stale or fetch failed, re-fetch now.
      const fbParams = new URLSearchParams();
      if (selectedTags.length) fbParams.set('tags', selectedTags.join(','));
      if (selectedBucket) fbParams.set('bucket', selectedBucket);
      const fbQs = fbParams.toString();
      const statsUrl = fbQs ? `/api/quiz/stats?${fbQs}` : '/api/quiz/stats';
      const stats = latestStats || await apiFetch(statsUrl, { signal }).catch(() => null);
      if (signal.aborted) return;
      if (!stats || stats.total === 0) {
        showEmptyState();
      } else {
        setText('success-stats', t('stats.attemptsAndMistakes', { attempts: stats.today_attempts, mistakes: stats.today_mistakes }));
        document.querySelectorAll('.advance-btn').forEach(btn => {
          btn.disabled = stats.available_to_advance === 0;
        });
        updateAdvanceButtonsForDifficult();
        const { showIntroduceNew } = successAdvanceState(stats);
        showIntroduceNew ? show('introduce-new-btn') : hide('introduce-new-btn');
        show('success-state');
        loadComebackInfo(stats.words_improved_today);
        loadWrongToday();
      }
    } else {
      show('error-state');
      setText('error-msg', e.message);
    }
    return;
  }

  // New word introduction (progressive mode)
  if (currentCard.mode === 'new_word') {
    hide('card-area');
    hide('new-component-area');
    _noCardsPaused = false; _onFocusOrVisibility();
    show('new-word-area');
    scrollCardIntoView('new-word-area');
    setText('new-word-zh', currentCard.prompt);
    setText('new-word-pinyin', currentCard.pinyin || '');
    // Numbered meaning rows (row i pairs the i-th meaning of each language,
    // primary first); rows beyond 3, noise annotations and cap overflow fold
    // into "▸ N more meanings · 1 example" (issue #431/#432/#433).
    const { rows, rest } = numberedMeaningRows(currentCard.translations, currentCard.translations_extra,
      orderLangsPrimaryFirst(selectedLangs, userPrimaryLang, userSecondaryLang), 3);
    const tailInfo = splitMoreInfo(rest);
    const tailLabel = moreInfoCountLabel(tailInfo);
    $('new-word-en').innerHTML = (rows.length
      ? rows.map((r, i) => `<div class="tr-meaning-row"><span class="tr-meaning-num">${i + 1}</span><span>${escHtml(r)}</span></div>`).join('')
      : '<div class="tr-meaning-row"><span>—</span></div>')
      + (tailLabel ? `<div style="padding-left:30px"><button type="button" class="tr-disclosure-link" data-disclosure="new-word-tail"><span>▸ ${escHtml(tailLabel)}</span></button><div id="new-word-tail" class="hidden tr-more-body" style="padding:6px 0 0">${moreInfoBlocksHTML(tailInfo)}</div></div>` : '');
    wireDisclosures($('new-word-en'));
    $('new-word-play-btn').onclick = () => playAudio(currentCard.word_id, currentCard.prompt);
    autoPlayCard(currentCard);
    currentCard.pinyin ? show('new-word-pinyin') : hide('new-word-pinyin');
    $('new-word-zh-input').value = '';
    $('new-word-trans-input').value = '';
    setCheckMark('new-word-zh-check', '', false);
    setCheckMark('new-word-trans-check', '', false);
    requireNewWordZh    ? show('new-word-zh-row')    : hide('new-word-zh-row');
    requireNewWordTrans ? show('new-word-trans-row') : hide('new-word-trans-row');
    const needsInput = requireNewWordZh || requireNewWordTrans;
    if (needsInput) {
      $('new-word-inputs').classList.remove('hidden');
      $('new-word-got-it-btn').disabled = true;
      // See the matching guard on the wrong-retype gate in train-result.js —
      // don't steal focus back from a field someone already started typing in.
      setTimeout(() => {
        const el = requireNewWordZh ? $('new-word-zh-input') : $('new-word-trans-input');
        if (autofocusInput && !el.value) el.focus({ preventScroll: true });
      }, 50);
    } else {
      $('new-word-inputs').classList.add('hidden');
      $('new-word-got-it-btn').disabled = false;
    }
    loadNewWordBreakdown(currentCard.prompt);
    await loadStats();
    if (signal.aborted) return;
    // The word being introduced has first_seen_at IS NULL and is not counted in
    // stats.due_today; add 1 so the counter reflects the card the user is working on.
    if (latestStats) {
      setText('stats-due', dueDisplayCount(latestStats, false, true));
    }
    return;
  }

  // New component introduction
  if (currentCard.card_type === 'component' && currentCard.is_new) {
    hide('card-area');
    hide('new-word-area');
    _noCardsPaused = false; _onFocusOrVisibility();
    show('new-component-area');
    scrollCardIntoView('new-component-area');
    setText('new-component-char', currentCard.prompt);
    $('new-component-play-btn').onclick = () => playComponentAudio(currentCard.prompt);
    autoPlayCard(currentCard);
    const compPinyin = currentCard.pinyin || null;
    setText('new-component-pinyin', compPinyin || '');
    compPinyin ? show('new-component-pinyin-row') : hide('new-component-pinyin-row');
    const defs = currentCard.definitions || {};
    $('new-component-defs').innerHTML = Object.entries(defs).map(([lang, def]) =>
      `<div class="tr-def"><span class="tr-def-lang">${escHtml(lang)}</span><span class="tr-def-text">${escHtml(def)}</span></div>`
    ).join('');
    await loadStats();
    return;
  }

  hide('new-component-area');
  _noCardsPaused = false; _onFocusOrVisibility();
  showCard();
  await loadStats();
}

// Languages with a "Type <language>" placeholder (issue #542).
const ANSWER_LANG_PLACEHOLDERS = ['en', 'de'];

function translPlaceholderKey(answerLang) {
  return ANSWER_LANG_PLACEHOLDERS.includes(answerLang) ? `card.placeholderTransl.${answerLang}` : 'card.placeholder';
}

// Sentence cards name the language of the blanked side (issue #621).
function placeholderKeyForCard(cardType, mode, answerLang) {
  if (cardType === 'sentence') {
    if (answerLang === 'zh') return 'card.placeholderZh';
    return ANSWER_LANG_PLACEHOLDERS.includes(answerLang) ? translPlaceholderKey(answerLang) : 'card.placeholderSentence';
  }
  if (!cardType && isZhAnswerMode(mode)) return 'card.placeholderZh';
  if (!cardType || cardType === 'component') return translPlaceholderKey(answerLang);
  return 'card.placeholder';
}

// Name the primary language if it is selected, else the first selected one.
function currentAnswerLang() {
  return orderLangsPrimaryFirst(selectedLangs, userPrimaryLang, userSecondaryLang)[0] || userPrimaryLang;
}

function showCard() {
  show('card-area');
  const answerLang = currentCard.card_type === 'sentence' ? currentCard.answer_lang : currentAnswerLang();
  $('answer-input').placeholder = t(placeholderKeyForCard(currentCard.card_type, currentCard.mode, answerLang));
  // Prompt size: Hanzi prompts are larger, sentence prompts smaller.
  const promptEl = $('prompt-word');
  const zhPrompt = currentCard.card_type === 'component' ||
    (!currentCard.card_type && !isZhAnswerMode(currentCard.mode));
  promptEl.className = 'tr-prompt font-hanzi' + (zhPrompt ? ' is-hanzi' : '') +
    (currentCard.card_type === 'sentence' ? ' is-sentence' : '');

  if (currentCard.card_type === 'component') {
    const compLabel = currentCard.is_also_word ? t('component.modeLabelAlsoWord') : t('component.modeLabel');
    setText('mode-label', compLabel);
    setText('prompt-word', currentCard.prompt);
    show('prompt-word');
    const playBtn = $('play-btn');
    playBtn.onclick = () => playComponentAudio(currentCard.prompt);
    show('play-btn');
    if (currentCard.pinyin) {
      setText('pinyin-hint', currentCard.pinyin);
      show('pinyin-hint');
    } else {
      hide('pinyin-hint');
    }
    hide('translations-hint');
    hide('hmm-type-badge');
    hide('hmm-actor-hint');
    hide('sentence-context');
  } else if (currentCard.card_type === 'hmm') {
    setText('mode-label', t('hmm.modeLabel'));
    setText('prompt-word', currentCard.prompt);
    hide('play-btn');
    hide('pinyin-hint');
    hide('translations-hint');
    hide('sentence-context');

    const badge = $('hmm-type-badge');
    badge.className = 'inline-block px-3 py-1 rounded-full text-xs font-bold uppercase tracking-wider mb-2 ' +
      (HMM_TYPE_COLORS[currentCard.entity_type] || 'bg-gray-100 text-gray-700');
    badge.textContent = t('hmm.type.' + currentCard.entity_type);
    show('hmm-type-badge');

    if (currentCard.entity_type === 'actor' && (currentCard.category || currentCard.hint)) {
      const parts = [];
      if (currentCard.category) parts.push(currentCard.category);
      if (currentCard.hint) parts.push(currentCard.hint);
      setText('hmm-actor-hint', parts.join(' · '));
      show('hmm-actor-hint');
    } else {
      hide('hmm-actor-hint');
    }
  } else if (currentCard.card_type === 'sentence') {
    setText('mode-label', t('sentence.modeLabel'));
    setText('prompt-word', currentCard.sentence_blank);
    show('prompt-word');
    hide('play-btn');
    hide('pinyin-hint');
    hide('translations-hint');
    hide('hmm-type-badge');
    hide('hmm-actor-hint');
    setText('sentence-context', currentCard.sentence_context);
    show('sentence-context');
  } else {
    hide('hmm-type-badge');
    hide('hmm-actor-hint');
    hide('sentence-context');

    setText('mode-label', wordModeLabel(getModeLabel(currentCard.mode), currentCard.is_also_component, t('word.alsoComponent')));

    // voice_to_transl / voice_to_zh: hide the prompt — the audio IS the
    // question. With voice unavailable, loadNextCard already switched the
    // card to its fallback mode (voiceFallbackMode).
    if (isVoiceOnlyMode(currentCard.mode)) {
      hide('prompt-word');
    } else {
      setText('prompt-word', currentCard.prompt);
      show('prompt-word');
    }

    // Show play button only when Chinese is the prompt and has sound — never
    // for transl_to_zh (would reveal the answer) or zh_to_transl_no_sound
    // (the point of that mode is to drill without an audio cue).
    const playBtn = $('play-btn');
    if (isZhPromptWithSound(currentCard.mode)) {
      playBtn.onclick = () => {
        questionAudioPlayed = true;
        playAudio(currentCard.word_id, currentCard.zh_text || currentCard.prompt);
      };
      show('play-btn');
    } else {
      hide('play-btn');
    }

    if (currentCard.pinyin) {
      setText('pinyin-hint', currentCard.pinyin);
      show('pinyin-hint');
    } else {
      hide('pinyin-hint');
    }

    if (currentCard.mode === 'transl_to_zh') {
      // Grouped by language (primary language first, then secondary), with
      // the card's own prompt excluded and cap overflow collapsed
      // (issue #431/#432/#433). Noise annotations (e.g. CEDICT/HanDeDict
      // example sentences) are dropped entirely here, not just collapsed —
      // an example sentence could spoil the zh answer on this question screen.
      const { shown: others, collapsed: moreInfoTexts } =
        groupTranslationsByLang(currentCard.translations, currentCard.translations_extra, orderLangsPrimaryFirst(selectedLangs, userPrimaryLang, userSecondaryLang), currentCard.prompt, true);
      // "Also: …" line, with meanings beyond the display cap folded
      // under "▸ N more".
      const extraHtml = moreInfoTexts.length > 0
        ? `<button type="button" class="tr-disclosure-link" data-disclosure="translations-hint-more"><span>▸ ${escHtml(t('card.nMore', { n: moreInfoTexts.length }))}</span></button><div id="translations-hint-more" class="hidden" style="font-size:13px;color:#9ca3af">${moreInfoTexts.map(escHtml).join(' · ')}</div>`
        : '';
      if (others.length > 0 || moreInfoTexts.length > 0) {
        $('translations-hint').innerHTML = (others.length ? `<div>${escHtml(t('card.also', { list: others.join(' · ') }))}</div>` : '') + extraHtml;
        wireDisclosures($('translations-hint'));
        show('translations-hint');
      } else {
        hide('translations-hint');
      }
    } else {
      hide('translations-hint');
    }
  }

  applyPinyinBlur();
  autoPlayCard(currentCard);
  // preventScroll: focus() otherwise scrolls just enough to reveal the input,
  // which can fight the explicit scrollCardIntoView below (issue #374).
  if (autofocusInput) $('answer-input').focus({ preventScroll: true });
  scrollCardIntoView('card-area');
}

async function submitAnswer(e) {
  e.preventDefault();
  if (isSubmitted || !currentCard) return;
  isSubmitted = true;

  const answer = $('answer-input').value;

  try {
    if (currentCard.card_type === 'component') {
      const result = await apiFetch('/api/component/answer', {
        method: 'POST',
        body: JSON.stringify({ character: currentCard.prompt, answer, langs: selectedLangs }),
      });
      maybeCelebrateThenShow(result, showComponentResult);
      return;
    }

    if (currentCard.card_type === 'hmm') {
      const result = await apiFetch('/api/hmm-quiz/answer', {
        method: 'POST',
        body: JSON.stringify({
          entity_type: currentCard.entity_type,
          entity_key: currentCard.entity_key,
          answer: answer,
        }),
      });
      maybeCelebrateThenShow(result, showHMMResult);
      return;
    }

    // A blurred pinyin hint counts as seen only once the user tapped it
    // (the tap removes blur-sm, see applyPinyinBlur).
    const pinyinEl = $('pinyin-hint');
    answerHelp = {
      drill: difficultDrill,
      pinyin_seen: !pinyinEl.classList.contains('hidden') && !pinyinEl.classList.contains('blur-sm'),
      audio_played: questionAutoPlayed || questionAudioPlayed,
      play_pressed: questionAudioPlayed,
    };
    const result = await apiFetch('/api/quiz/answer', {
      method: 'POST',
      body: JSON.stringify({
        word_id: currentCard.word_id,
        mode: currentCard.mode,
        answer: answer,
        langs: selectedLangs,
        ...answerHelp,
      }),
    });
    maybeCelebrateThenShow(result, (r) => renderWordAnswerResult(r, answer));
  } catch (err) {
    isSubmitted = false;
    showToastError(err.message);
  }
}

// Shows the celebration interstitial first when the answer was both correct
// and advanced the word's tier (gated by the celebrate_bucket_change
// setting) — so it appears BEFORE the correct/wrong result screen, not
// after it. Otherwise renders the result screen immediately.
function maybeCelebrateThenShow(result, render) {
  // The answer is already saved here: if the result screen fails to draw,
  // show the error and go on to the next card instead of leaving a
  // half-drawn screen (issue #581).
  const showFn = (r) => {
    try {
      render(r);
    } catch (err) {
      showToastError(err.message, err);
      loadNextCard(true);
    }
  };
  const wrongInARow = recordWrongStreak(wrongStreaks, cardStreakKey(currentCard), result.correct);
  $('result-skip-today-row').classList.toggle('hidden', wrongInARow < SKIP_AFTER_WRONG);
  const tierAdvanced = result.correct && result.tier && result.prev_tier && result.prev_tier !== result.tier;
  if (celebrateBucketChange && tierAdvanced) {
    const meanings = result.translations
      ? groupTranslationsByLang(result.translations, null, orderLangsPrimaryFirst(selectedLangs, userPrimaryLang, userSecondaryLang)).shown
      : [];
    showCelebrationScreen({
      prevTier: result.prev_tier, tier: result.tier,
      zhText: result.zh_text || (currentCard && currentCard.card_type === 'component' ? currentCard.prompt : ''),
      pinyin: result.pinyin || '', meanings,
    }, () => showFn(result));
  } else {
    showFn(result);
  }
}

// Advances the current card's due date by 1 day.
function skipCurrentCardForToday(reveal) {
  let url, body;
  if (currentCard.card_type === 'hmm') {
    url = '/api/hmm-quiz/skip';
    body = { entity_type: currentCard.entity_type, entity_key: currentCard.entity_key, days: 1 };
  } else if (currentCard.card_type === 'component') {
    url = '/api/component/skip';
    body = { character: currentCard.prompt, days: 1, langs: selectedLangs };
  } else {
    url = '/api/quiz/skip';
    body = { word_id: currentCard.word_id, days: 1 };
  }
  if (reveal) body.reveal = true;
  const key = cardStreakKey(currentCard);
  return apiFetch(url, { method: 'POST', body: JSON.stringify(body) }).then(resp => {
    wrongStreaks.delete(key);
    return resp;
  });
}

// Picks the button Enter presses on the visible train screen (issue #535), or
// null when a focused text field owns Enter or the screen has no such action.
function enterActionButtonId(screens, focusedTag) {
  if (['INPUT', 'TEXTAREA', 'SELECT'].includes(focusedTag)) return null;
  if (screens.celebration) return 'celebration-continue-btn';
  if (screens.result) return 'next-btn';
  if (screens.newComponent) return 'new-component-got-it-btn';
  if (screens.newWord) return 'new-word-got-it-btn';
  if (screens.error) return 'error-retry-btn';
  return null;
}

document.addEventListener('DOMContentLoaded', () => {
  applyModeButtons();
  applyTierPills();
  applyMnemonicPill();
  applyComponentPill();
  loadLangs();
  loadTrainTags();

  function toggleMnemonics() {
    includeMnemonics = !includeMnemonics;
    localStorage.setItem('quizMnemonics', includeMnemonics ? 'true' : 'false');
    scheduleFilterSave();
    applyMnemonicPill();
    loadNextCard();
  }
  const overlayMnemonicsPill = $('overlay-mnemonics-pill');
  if (overlayMnemonicsPill) overlayMnemonicsPill.addEventListener('click', toggleMnemonics);

  function toggleComponents() {
    includeComponents = !includeComponents;
    localStorage.setItem('quizComponents', includeComponents ? 'true' : 'false');
    scheduleFilterSave();
    applyComponentPill();
    loadNextCard();
  }
  const overlayComponentsPill = $('overlay-components-pill');
  if (overlayComponentsPill) overlayComponentsPill.addEventListener('click', toggleComponents);

  function applyAutoPlayButton() {
    const btn = $('autoplay-toggle-btn');
    if (!btn) return;
    btn.setAttribute('aria-pressed', autoPlayEnabled ? 'true' : 'false');
    const d = autoPlayEnabled
      ? 'M11 5 6 9H3v6h3l5 4zM15.5 8.5a5 5 0 0 1 0 7M18.5 5.5a9 9 0 0 1 0 13'
      : 'M11 5 6 9H3v6h3l5 4zM22 9l-6 6M16 9l6 6';
    btn.innerHTML = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="${d}"/></svg>`;
    const label = t(autoPlayEnabled ? 'train.autoPlay.onTitle' : 'train.autoPlay.offTitle');
    btn.title = label;
    btn.setAttribute('aria-label', label);
  }
  const autoPlayBtn = $('autoplay-toggle-btn');
  if (autoPlayBtn) {
    autoPlayBtn.addEventListener('click', () => {
      autoPlayEnabled = !autoPlayEnabled;
      applyAutoPlayButton();
    });
  }

  document.querySelectorAll('.overlay-tier-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      selectedBucket = btn.dataset.bucket;
      localStorage.setItem('quizBucket', selectedBucket);
      scheduleFilterSave();
      applyTierPills();
    });
  });
  $('answer-form').addEventListener('submit', submitAnswer);
  $('answer-input').addEventListener('focus', () => setTimeout(revealAnswerForm, 300));
  window.visualViewport?.addEventListener('resize', revealAnswerForm);
  $('next-btn').addEventListener('click', async () => {
    // Continuing past an unresolved ambiguous result reveals the normal
    // wrong-answer screen first; a second click then actually advances.
    if (ambiguousUnresolved) {
      const showFallback = ambiguousUnresolved;
      ambiguousUnresolved = null;
      showFallback();
      return;
    }
    hide('leech-box');
    // A second tap during a slow match-game load opened a second round
    // (issue #563). loadNextCard enables the button again.
    $('next-btn').disabled = true;
    await _maybeShowMatchGame();
    loadNextCard(true);
  });
  $('leech-keep-btn').addEventListener('click', () => hide('leech-box'));
  // "Back to unseen" erases the word's history (POST /api/words/{id}/reset);
  // it leaves today's session and is introduced again later as a new word.
  $('leech-reset-btn').addEventListener('click', async () => {
    const btn = $('leech-reset-btn');
    btn.disabled = true;
    try {
      await apiFetch(`/api/words/${currentCard.word_id}/reset`, { method: 'POST' });
      loadNextCard(true);
    } catch (err) {
      showToastError(err.message);
    } finally {
      btn.disabled = false;
    }
  });
  $('accept-correct-btn').addEventListener('click', async () => {
    const btn = $('accept-correct-btn');
    btn.disabled = true;
    try {
      if (currentCard.card_type === 'component') {
        await apiFetch('/api/component/accept-correct', {
          method: 'POST',
          body: JSON.stringify({ character: currentCard.prompt }),
        });
      } else {
        await apiFetch('/api/quiz/accept-correct', {
          method: 'POST',
          body: JSON.stringify({
            word_id: currentCard.word_id,
            mode: currentCard.mode,
            langs: selectedLangs,
            ...answerHelp,
          }),
        });
      }
      wrongStreaks.delete(cardStreakKey(currentCard));
      loadNextCard(true);
    } catch (err) {
      btn.disabled = false;
      showToastError(err.message);
    }
  });

  // Session sheet (all screen sizes)
  function openFilterOverlay() {
    $('filter-overlay').classList.remove('hidden');
    document.body.style.overflow = 'hidden';
  }
  function closeFilterOverlay() {
    loadNextCard();
    $('filter-overlay').classList.add('hidden');
    document.body.style.overflow = '';
  }
  $('open-filter-overlay').addEventListener('click', openFilterOverlay);
  $('filter-overlay-close').addEventListener('click', closeFilterOverlay);
  $('filter-overlay-backdrop').addEventListener('click', closeFilterOverlay);
  document.querySelectorAll('.overlay-mode-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      selectedMode = btn.dataset.mode;
      localStorage.setItem('quizMode', selectedMode);
      scheduleFilterSave();
      applyModeButtons();
    });
  });


  // Skip current card for today (advance due_date by 1 day).
  $('skip-today-btn').addEventListener('click', () => withBusyButton($('skip-today-btn'), async () => {
    if (isSubmitted || !currentCard) return;

    // If skipping a voice card, ask whether to disable voice for this
    // session. On yes, the same word is asked again at once in the fallback
    // mode instead of being skipped (issue #571).
    const fallbackMode = voiceFallbackMode(currentCard.mode, true);
    if (fallbackMode !== currentCard.mode && !voiceUnavailable) {
      if (confirm(t(currentCard.mode === 'voice_to_zh' ? 'card.voiceUnavailableToZh' : 'card.voiceUnavailableToTransl'))) {
        voiceUnavailable = true;
        if (selectedMode === currentCard.mode) {
          selectedMode = fallbackMode;
          applyModeButtons();
          localStorage.setItem('quizMode', selectedMode);
        }
        currentCard.mode = fallbackMode;
        showCard();
        return;
      }
    }

    // With skip_reveal_answer on, the skip response holds the answer and a
    // "Skipped" screen shows it before the next card (issue #536).
    let resp;
    try {
      resp = await skipCurrentCardForToday(skipRevealAnswer);
    } catch (err) {
      showToastError(err.message);
      return;
    }
    if (skipRevealAnswer && resp) {
      isSubmitted = true;
      showSkipResult(resp);
      return;
    }
    loadNextCard();
  }));

  // The wrong-answer screen offers the same skip after SKIP_AFTER_WRONG wrong
  // answers in a row (issue #554). The answer is already on screen.
  $('result-skip-today-btn').addEventListener('click', async () => {
    if (!currentCard) return;
    const btn = $('result-skip-today-btn');
    btn.disabled = true;
    try {
      await skipCurrentCardForToday(false);
    } catch (err) {
      alert('Error: ' + err.message);
      return;
    } finally {
      btn.disabled = false;
    }
    loadNextCard(true);
  });

  // Progressive mode: new word buttons
  $('new-word-skip-btn').addEventListener('click', () => withBusyButton($('new-word-skip-btn'), async () => {
    if (!currentCard) return;
    try {
      await apiFetch('/api/quiz/skip', {
        method: 'POST',
        body: JSON.stringify({ word_id: currentCard.word_id }),
      });
    } catch (err) {
      showToastError(err.message);
      return;
    }
    loadNextCard();
  }));
  // Bracketed annotations (e.g. "过（动词）") are optional, mirroring the
  // backend's CheckAnswer / expandVariants rule for regular quiz answers.
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
  function updateGotItState() {
    if (!currentCard) return;
    const zhVal    = $('new-word-zh-input').value;
    const transVal = $('new-word-trans-input').value;
    const zhCorrect    = isZhCorrect(zhVal, currentCard.prompt);
    const transCorrect = isTransCorrect(transVal, mergeTranslationMaps(currentCard.translations, currentCard.translations_extra));
    const zhOk    = !requireNewWordZh    || zhCorrect;
    const transOk = !requireNewWordTrans || transCorrect;
    if (requireNewWordZh) setCheckMark('new-word-zh-check', zhVal, zhCorrect);
    if (requireNewWordTrans) setCheckMark('new-word-trans-check', transVal, transCorrect);
    $('new-word-got-it-btn').disabled = !(zhOk && transOk);
  }
  $('new-word-zh-input').addEventListener('input', updateGotItState);
  $('new-word-trans-input').addEventListener('input', updateGotItState);

  $('new-word-got-it-btn').addEventListener('click', () => withBusyButton($('new-word-got-it-btn'), async () => {
    if (!currentCard) return;
    try {
      await apiFetch('/api/quiz/acknowledge', {
        method: 'POST',
        body: JSON.stringify({ word_id: currentCard.word_id }),
      });
    } catch (err) {
      showToastError(err.message);
      return;
    }
    loadNextCard();
  }));
  $('new-word-no-new-btn').addEventListener('click', () => {
    skipNewWords = true;
    loadNextCard();
  });

  // Retype-on-wrong gate: mirrors updateGotItState above, but drives the
  // shared #next-btn instead of a dedicated "Got it" button.
  function updateWrongRetypeState() {
    if (!wrongRetypeTarget) return;
    const { requireZh, requireTrans } = wrongRetypeTarget;
    const zhVal    = $('wrong-retype-zh-input').value;
    const transVal = $('wrong-retype-trans-input').value;
    const zhCorrect    = isZhCorrect(zhVal, wrongRetypeTarget.zhText);
    const transCorrect = isTransCorrect(transVal, wrongRetypeTarget.translations);
    setCheckMark('wrong-retype-zh-check', zhVal, zhCorrect);
    setCheckMark('wrong-retype-trans-check', transVal, transCorrect);
    $('next-btn').disabled = !wrongRetypeSatisfied(zhVal, transVal, wrongRetypeTarget.zhText, wrongRetypeTarget.translations, requireZh, requireTrans);
  }
  $('wrong-retype-zh-input').addEventListener('input', updateWrongRetypeState);
  $('wrong-retype-trans-input').addEventListener('input', updateWrongRetypeState);

  // Enter in a gate input continues, as long as the gate button is enabled.
  [['new-word-zh-input', 'new-word-got-it-btn'], ['new-word-trans-input', 'new-word-got-it-btn'],
   ['wrong-retype-zh-input', 'next-btn'], ['wrong-retype-trans-input', 'next-btn']].forEach(([inputId, btnId]) => {
    $(inputId).addEventListener('keydown', e => {
      if (e.key !== 'Enter' || e.isComposing) return;
      e.preventDefault();
      if (!$(btnId).disabled) $(btnId).click();
    });
  });
  // Enter continues on every screen with a continue action, also when the
  // focus is on another button or on nothing (issue #535).
  const isShown = id => $(id) && !$(id).classList.contains('hidden');
  document.addEventListener('keydown', e => {
    if (e.key !== 'Enter' || e.isComposing || e.repeat || e.defaultPrevented) return;
    if (isShown('issue-modal') || isShown('filter-overlay') || isShown('more-sheet') || $('match-game-overlay')) return;
    const btnId = enterActionButtonId({
      celebration: isShown('celebration-screen'),
      result: isShown('result-area'),
      newComponent: isShown('new-component-area'),
      newWord: isShown('new-word-area'),
      error: isShown('error-state'),
    }, document.activeElement ? document.activeElement.tagName : '');
    if (!btnId || $(btnId).disabled) return;
    e.preventDefault();
    $(btnId).click();
  });
  $('new-component-got-it-btn').addEventListener('click', () => withBusyButton($('new-component-got-it-btn'), async () => {
    if (!currentCard) return;
    try {
      await apiFetch('/api/component/seen', {
        method: 'POST',
        body: JSON.stringify({ character: currentCard.prompt }),
      });
    } catch (err) {
      showToastError(err.message);
      return;
    }
    loadNextCard();
  }));

  document.querySelectorAll('.advance-btn').forEach(btn => {
    btn.addEventListener('click', () => withBusyButton(btn, async () => {
      const count = parseInt(btn.dataset.advance);
      // When "drill my hardest words" is ticked, the amount buttons flag that
      // many difficult words and start a focused drill instead of advancing.
      if ($('difficult-words-checkbox') && $('difficult-words-checkbox').checked) {
        let resp;
        try {
          resp = await apiFetch('/api/quiz/difficult', {
            method: 'POST',
            body: JSON.stringify({ count }),
          });
        } catch (err) {
          showToastError(err.message);
          return;
        }
        if (!resp || !resp.flagged) {
          alert(t('success.noDifficult'));
          return;
        }
        difficultDrill = true;
        localStorage.setItem('quizDifficultDrill', 'true');
        renderDifficultDrill();
        hide('success-state');
        loadNextCard();
        return;
      }
      // ponytail: auto-reset cap when fewer seen words than requested, so new words fill the gap
      const resetNewCap = $('reset-cap-checkbox').checked || (latestStats && latestStats.available_to_advance < count);
      try {
        await apiFetch('/api/quiz/advance', {
          method: 'POST',
          body: JSON.stringify({ count, reset_new_cap: resetNewCap }),
        });
      } catch (err) {
        showToastError(err.message);
        return;
      }
      hide('success-state');
      loadNextCard();
    }));
  });

  const difficultCheckbox = $('difficult-words-checkbox');
  if (difficultCheckbox) {
    difficultCheckbox.addEventListener('change', updateAdvanceButtonsForDifficult);
  }

  const drillPill = $('difficult-drill-pill');
  if (drillPill) {
    drillPill.addEventListener('click', async () => {
      await exitDifficultDrill(true);
      loadNextCard();
    });
  }

  // "Review today's mistakes" sheet on the all-done screen (issue #537).
  $('wrong-today-btn').addEventListener('click', openWrongToday);
  $('wrong-today-close').addEventListener('click', closeWrongToday);
  $('wrong-today-backdrop').addEventListener('click', closeWrongToday);
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && !$('wrong-today-overlay').classList.contains('hidden')) closeWrongToday();
  });
  $('wrong-today-retrain-btn').addEventListener('click', async () => {
    const btn = $('wrong-today-retrain-btn');
    btn.disabled = true;
    try {
      await apiFetch('/api/quiz/wrong-today/retrain', { method: 'POST' });
    } catch (err) {
      showToastError(err.message);
      return;
    } finally {
      btn.disabled = false;
    }
    closeWrongToday();
    hide('success-state');
    loadNextCard();
  });

  $('introduce-new-btn').addEventListener('click', () => withBusyButton($('introduce-new-btn'), async () => {
    try {
      await apiFetch('/api/quiz/advance', {
        method: 'POST',
        body: JSON.stringify({ count: 0, reset_new_cap: true }),
      });
    } catch (err) {
      showToastError(err.message);
      return;
    }
    hide('success-state');
    loadNextCard();
  }));

  // Re-render dynamic text when language changes
  document.addEventListener('langchange', () => {
    applyModeButtons();
    applyTierPills();
    updateAdvanceButtonsForDifficult();
    renderWrongTodayButton();
  });

  // Onboarding (shown when user has zero words): the setup wizard, fed with
  // the shared library's lists.
  async function obLoadTags() {
    const root = $('wz-root');
    try {
      const tags = await apiFetch('/api/import/source-tags');
      wizardStart(root, tags, detail => {
        selectedLangs = detail.langs;
        if (detail.audio !== undefined) autoPlayEnabled = detail.audio;
        if (detail.game !== undefined) _gamificationEnabled = detail.game;
        applyAutoPlayButton();
        hide('empty-state');
        loadNextCard();
        watchImports();
      });
      show('ob-quickstart');
    } catch (e) {
      root.innerHTML = `<div class="wz-card wz-pad wz-body"><p class="wz-error">${escHtml(e.message)}</p></div>`;
      show('ob-quickstart');
    }
  }

  document.addEventListener('ob:loadtags', obLoadTags, { once: false });

  $('error-retry-btn').addEventListener('click', () => loadNextCard());
  const errReport = $('error-report-btn');
  const navReport = $('nav-report');
  if (errReport && navReport) {
    // Offered only when issue reporting is enabled (the shell shows #nav-report).
    new MutationObserver(() => errReport.classList.toggle('hidden', navReport.classList.contains('hidden')))
      .observe(navReport, { attributes: true, attributeFilter: ['class'] });
    errReport.classList.toggle('hidden', navReport.classList.contains('hidden'));
    errReport.addEventListener('click', () => navReport.click());
  }

  loadTrainSettings().then(() => { applyAutoPlayButton(); loadNextCard(); });
  watchImports();
});
