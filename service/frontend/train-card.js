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
let voiceUnavailable = false; // session-only flag, set when user skips a voice card and opts out
let _gamificationEnabled = false;
let _gamificationFrequencyMs = 5 * 60 * 1000;
let _lastGameShownAt = 0;
let _matchGamePinyinReveal = 'always';
let _matchGameSm2Update = 'always';
let _matchGameShowSummary = true;
const _settingsPromise = fetch('/api/settings').then(r => r.ok ? r.json() : null).then(st => {
  if (st?.primary_lang) userPrimaryLang = st.primary_lang;
  userSecondaryLang = st?.secondary_lang ?? '';
  acceptCorrectMode = st?.accept_correct_mode ?? 'typo';
  skipNewWordsVisible = st?.skip_new_words_visible !== false;
  blurPinyin = !!st?.blur_pinyin;
  noAutoVoiceOnBlur = !!st?.no_auto_voice_on_blur;
  celebrateBucketChange = !!st?.celebrate_bucket_change;
  _gamificationEnabled = !!st?.gamification_enabled;
  autoPlayEnabled = !!st?.autoplay_always;
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
let selectedTags = JSON.parse(localStorage.getItem('quizTags') || '[]');
let selectedBucket = localStorage.getItem('quizBucket') || '';
let selectedLangs = JSON.parse(localStorage.getItem('quizLangs') || '["en"]');
let includeMnemonics = localStorage.getItem('quizMnemonics') !== 'false';
let includeComponents = localStorage.getItem('quizComponents') !== 'false';
let latestStats = null;

// Auto-play toggle: in-memory only, never persisted — always resets to off
// on page load/reload so it doesn't surprise the user across sessions.
let autoPlayEnabled = false;
let currentAutoPlayAudio = null;
// Tracks whether audio actually started playing for the current card via
// autoPlayCard, so the result screen knows whether to play it there instead
// (issue #272). Reset whenever loadNextCard replaces currentCard.
let questionAutoPlayed = false;

let skipNewWords = false;
let requireNewWordZh = true;
let requireNewWordTrans = true;
let wrongAnswerRetryMode = 'off';
// Holds { zhText, translations } for the currently rendered wrong-answer
// result while the retype-on-wrong gate is active; read by the top-level
// wrong-retype input listeners set up in DOMContentLoaded.
let wrongRetypeTarget = null;
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

async function loadNextCard(trackCurrent = false) {
  _noCardsPaused = true;  // pause until we confirm a card is ready
  await _flushTime();
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
  hide('result-decompose');
  hide('result-decompose-content');
  hide('bucket-info');
  hide('streak-info');
  hide('streak-dots');
  hide('result-subtitle');
  hide('wrong-retype-area');
  wrongRetypeTarget = null;
  $('next-btn').disabled = false;
  $('answer-input').value = '';
  const reviewBtn = $('needs-review-btn');
  renderFlagButton(reviewBtn, false);
  reviewBtn.disabled = false;
  reviewBtn.onclick = null;

  // Fetch fresh stats first. The backend's GetNextCard may return non-due
  // (future) cards via its fallback queries even when due_today = 0, so we
  // cannot rely solely on a 404 "no words available" to trigger the success
  // screen — we must check due_today proactively.
  await loadStats();

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
    currentCard = await apiFetch(url);
    questionAutoPlayed = false;
    // The served card was pulled in from beyond today's due-date bound solely
    // to avoid repeating a just-answered word — it isn't reflected in
    // latestStats.due_today, so bump the displayed count by 1 to match.
    if (currentCard?.session_extension && latestStats) {
      setText('stats-due', dueDisplayCount(latestStats, true));
    }
  } catch (e) {
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
      const stats = latestStats || await apiFetch(statsUrl).catch(() => null);
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
        if (!el.value) el.focus({ preventScroll: true });
      }, 50);
    } else {
      $('new-word-inputs').classList.add('hidden');
      $('new-word-got-it-btn').disabled = false;
    }
    loadNewWordBreakdown(currentCard.prompt);
    await loadStats();
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

function placeholderKeyForCard(cardType, mode) {
  if (cardType === 'sentence') return 'card.placeholderSentence';
  if (!cardType && mode === 'transl_to_zh') return 'card.placeholderZh';
  return 'card.placeholder';
}

function showCard() {
  show('card-area');
  $('answer-input').placeholder = t(placeholderKeyForCard(currentCard.card_type, currentCard.mode));
  // Tier chip (word cards only) and prompt size: Hanzi prompts are larger,
  // sentence prompts smaller.
  const tierEl = $('card-tier');
  if (currentCard.tier) {
    renderTierChip(tierEl, currentCard.tier);
    show('card-tier');
  } else {
    hide('card-tier');
  }
  const promptEl = $('prompt-word');
  const zhPrompt = currentCard.card_type === 'component' ||
    (!currentCard.card_type && currentCard.mode !== 'transl_to_zh');
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

    // voice_to_transl: hide the Chinese text — the audio IS the prompt.
    // If the user has voice_unavailable set, fall back to showing the text.
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
      playBtn.onclick = () => playAudio(currentCard.word_id, currentCard.prompt);
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
  $('answer-input').focus({ preventScroll: true });
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

    const result = await apiFetch('/api/quiz/answer', {
      method: 'POST',
      body: JSON.stringify({
        word_id: currentCard.word_id,
        mode: currentCard.mode,
        answer: answer,
        langs: selectedLangs,
      }),
    });
    maybeCelebrateThenShow(result, (r) => renderWordAnswerResult(r, answer));
  } catch (err) {
    isSubmitted = false;
    alert('Error: ' + err.message);
  }
}

// Shows the celebration interstitial first when the answer was both correct
// and advanced the word's tier (gated by the celebrate_bucket_change
// setting) — so it appears BEFORE the correct/wrong result screen, not
// after it. Otherwise renders the result screen immediately.
function maybeCelebrateThenShow(result, showFn) {
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
  $('next-btn').addEventListener('click', async () => {
    // Continuing past an unresolved ambiguous result reveals the normal
    // wrong-answer screen first; a second click then actually advances.
    if (ambiguousUnresolved) {
      const showFallback = ambiguousUnresolved;
      ambiguousUnresolved = null;
      showFallback();
      return;
    }
    await _maybeShowMatchGame();
    loadNextCard(true);
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
          }),
        });
      }
      loadNextCard(true);
    } catch (err) {
      btn.disabled = false;
      alert('Could not accept as correct: ' + err.message);
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
  $('skip-today-btn').addEventListener('click', async () => {
    if (isSubmitted || !currentCard) return;

    // If skipping a voice card, ask whether to disable voice for this session.
    if (currentCard.mode === 'voice_to_transl' && !voiceUnavailable) {
      if (confirm('Voice not available? Switch to Chinese → Translation for the rest of this session?')) {
        voiceUnavailable = true;
        selectedMode = 'zh_to_transl';
        applyModeButtons();
        localStorage.setItem('quizMode', selectedMode);
      }
    }

    let url, body;
    if (currentCard.card_type === 'hmm') {
      url = '/api/hmm-quiz/skip';
      body = { entity_type: currentCard.entity_type, entity_key: currentCard.entity_key, days: 1 };
    } else if (currentCard.card_type === 'component') {
      url = '/api/component/skip';
      body = { character: currentCard.prompt, days: 1 };
    } else {
      url = '/api/quiz/skip';
      body = { word_id: currentCard.word_id, days: 1 };
    }
    try {
      await apiFetch(url, { method: 'POST', body: JSON.stringify(body) });
    } catch (err) {
      alert('Error: ' + err.message);
      return;
    }
    loadNextCard();
  });

  // Progressive mode: new word buttons
  $('new-word-skip-btn').addEventListener('click', async () => {
    if (!currentCard) return;
    try {
      await apiFetch('/api/quiz/skip', {
        method: 'POST',
        body: JSON.stringify({ word_id: currentCard.word_id }),
      });
    } catch (err) {
      alert('Error: ' + err.message);
      return;
    }
    loadNextCard();
  });
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

  $('new-word-got-it-btn').addEventListener('click', async () => {
    if (!currentCard) return;
    try {
      await apiFetch('/api/quiz/acknowledge', {
        method: 'POST',
        body: JSON.stringify({ word_id: currentCard.word_id }),
      });
    } catch (err) {
      alert('Error: ' + err.message);
      return;
    }
    loadNextCard();
  });
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
  $('new-component-got-it-btn').addEventListener('click', async () => {
    if (!currentCard) return;
    try {
      await apiFetch('/api/component/seen', {
        method: 'POST',
        body: JSON.stringify({ character: currentCard.prompt }),
      });
    } catch (err) {
      alert('Error: ' + err.message);
      return;
    }
    loadNextCard();
  });

  document.querySelectorAll('.advance-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
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
          alert('Error: ' + err.message);
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
        alert('Error: ' + err.message);
        return;
      }
      hide('success-state');
      loadNextCard();
    });
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

  $('introduce-new-btn').addEventListener('click', async () => {
    try {
      await apiFetch('/api/quiz/advance', {
        method: 'POST',
        body: JSON.stringify({ count: 0, reset_new_cap: true }),
      });
    } catch (err) {
      alert('Error: ' + err.message);
      return;
    }
    hide('success-state');
    loadNextCard();
  });

  // Re-render dynamic text when language changes
  document.addEventListener('langchange', () => {
    applyModeButtons();
    applyTierPills();
    updateAdvanceButtonsForDifficult();
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
});
