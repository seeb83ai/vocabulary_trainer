// Training page logic

// Language settings loaded from /api/settings on init
let userPrimaryLang = 'en';
let userSecondaryLang = '';
const _settingsPromise = fetch('/api/settings').then(r => r.ok ? r.json() : null).then(st => {
  if (st?.primary_lang) userPrimaryLang = st.primary_lang;
  userSecondaryLang = st?.secondary_lang ?? '';
}).catch(() => {});

const HMM_TYPE_COLORS = {
  actor:     'bg-purple-100 text-purple-700',
  location:  'bg-blue-100 text-blue-700',
  tone_room: 'bg-amber-100 text-amber-700',
  prop:      'bg-emerald-100 text-emerald-700',
};

let currentCard = null;
let isSubmitted = false;
let selectedMode = localStorage.getItem('quizMode') || 'random';
let selectedTags = JSON.parse(localStorage.getItem('quizTags') || '[]');
let selectedBucket = localStorage.getItem('quizBucket') || '';
let selectedLangs = JSON.parse(localStorage.getItem('quizLangs') || '["en"]');
let includeMnemonics = localStorage.getItem('quizMnemonics') !== 'false';
let includeComponents = localStorage.getItem('quizComponents') !== 'false';
let latestStats = null;
let skipNewWords = false;

function applyModeButtons() {
  document.querySelectorAll('.mode-btn').forEach(btn => {
    const active = btn.dataset.mode === selectedMode;
    btn.className = active
      ? 'mode-btn px-3 py-1 rounded-full text-sm font-medium transition bg-blue-600 text-white'
      : 'mode-btn px-3 py-1 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  });
  // Mobile: update the single visible label
  const mobileLabel = document.getElementById('mode-mobile-label');
  if (mobileLabel) mobileLabel.textContent = t('mode.' + selectedMode);
  // Overlay: apply same active/inactive styling
  document.querySelectorAll('.overlay-mode-btn').forEach(btn => {
    const active = btn.dataset.mode === selectedMode;
    btn.className = active
      ? 'overlay-mode-btn px-4 py-2 rounded-full text-sm font-medium transition bg-blue-600 text-white'
      : 'overlay-mode-btn px-4 py-2 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  });
}

function applyTierPills() {
  document.querySelectorAll('.tier-pill, .overlay-tier-btn').forEach(btn => {
    const active = btn.dataset.bucket === selectedBucket;
    const isMini = btn.classList.contains('tier-pill');
    btn.className = (isMini
      ? `tier-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition `
      : `overlay-tier-btn px-3 py-1.5 rounded-full text-sm font-medium transition `) +
      (active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200');
  });
  // Mobile: update the single level chip next to the mode chip
  const levelLabel = document.getElementById('level-mobile-label');
  if (levelLabel) {
    const tier = TIERS.find(t => t.key === selectedBucket);
    if (tier) {
      levelLabel.textContent = t('tier.' + tier.label.toLowerCase());
      levelLabel.className = 'px-3 py-1 rounded-full text-sm font-medium bg-blue-600 text-white';
    } else {
      levelLabel.textContent = t('tier.all');
      levelLabel.className = 'px-3 py-1 rounded-full text-sm font-medium bg-gray-100 text-gray-600';
    }
  }
}

let obTagsLoaded = false;
function showEmptyState() {
  show('empty-state');
  if (!obTagsLoaded) {
    obTagsLoaded = true;
    // obLoadTags is defined inside DOMContentLoaded; call lazily via event
    document.dispatchEvent(new CustomEvent('ob:loadtags'));
  }
}

function applyMnemonicPill() {
  const active = includeMnemonics;
  const cls = active
    ? 'px-2.5 py-0.5 rounded-full text-xs font-medium transition bg-blue-600 text-white'
    : 'px-2.5 py-0.5 rounded-full text-xs font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  const overlayCls = active
    ? 'px-4 py-2 rounded-full text-sm font-medium transition bg-blue-600 text-white'
    : 'px-4 py-2 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  const label = t('filter.mnemonicsOn');
  const pill = $('mnemonics-pill');
  if (pill) { pill.className = cls; pill.textContent = label; }
  const overlayPill = $('overlay-mnemonics-pill');
  if (overlayPill) { overlayPill.className = overlayCls; overlayPill.textContent = label; }
}

function applyComponentPill() {
  const active = includeComponents;
  const cls = active
    ? 'px-2.5 py-0.5 rounded-full text-xs font-medium transition bg-blue-600 text-white'
    : 'px-2.5 py-0.5 rounded-full text-xs font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  const overlayCls = active
    ? 'px-4 py-2 rounded-full text-sm font-medium transition bg-blue-600 text-white'
    : 'px-4 py-2 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  const label = t('filter.componentsOn');
  const pill = $('components-pill');
  if (pill) { pill.className = cls; pill.textContent = label; }
  const overlayPill = $('overlay-components-pill');
  if (overlayPill) { overlayPill.className = overlayCls; overlayPill.textContent = label; }
}

async function loadStats() {
  try {
    const params = new URLSearchParams();
    if (selectedTags.length) params.set('tags', selectedTags.join(','));
    if (selectedBucket) params.set('bucket', selectedBucket);
    if (!includeMnemonics) params.set('mnemonics', 'false');
    if (includeComponents) params.set('trainComponents', '1');
    const qs = params.toString();
    const statsUrl = qs ? `/api/quiz/stats?${qs}` : '/api/quiz/stats';
    const stats = await apiFetch(statsUrl);
    latestStats = stats;
    setText('stats-due', stats.due_today + (stats.hmm_due_today || 0) + (stats.components_due_today || 0));
    setText('stats-total', stats.total);
    setText('stats-new', `${stats.new_today} / ${stats.max_new_per_day}`);
  } catch (_) {}
}

async function loadNextCard() {
  isSubmitted = false;
  hide('card-area');
  hide('result-area');
  hide('empty-state');
  hide('success-state');
  hide('error-state');
  hide('add-translation-btn');
  hide('result-play-btn');
  hide('new-word-area');
  hide('new-component-area');
  hide('result-decompose');
  hide('result-decompose-content');
  hide('bucket-info');
  hide('streak-info');
  $('answer-input').value = '';
  const reviewBtn = $('needs-review-btn');
  reviewBtn.textContent = t('result.flagReview');
  reviewBtn.disabled = false;
  reviewBtn.className = 'w-1/2 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
  reviewBtn.onclick = null;

  // Fetch fresh stats first. The backend's GetNextCard may return non-due
  // (future) cards via its fallback queries even when due_today = 0, so we
  // cannot rely solely on a 404 "no words available" to trigger the success
  // screen — we must check due_today proactively.
  await loadStats();

  if (latestStats) {
    if (latestStats.total === 0) {
      showEmptyState();
      return;
    }
    if (latestStats.due_today === 0 && (latestStats.hmm_due_today || 0) === 0 && (latestStats.components_due_today || 0) === 0 && (!latestStats.new_available || skipNewWords)) {
      skipNewWords = false;
      setText('success-stats', t('stats.attemptsAndMistakes', { attempts: latestStats.today_attempts, mistakes: latestStats.today_mistakes }));
      const allAdvanceDisabled = latestStats.available_to_advance < 10;
      document.querySelectorAll('.advance-btn').forEach(btn => {
        btn.disabled = latestStats.available_to_advance < parseInt(btn.dataset.advance);
      });
      const hasUnseen = (latestStats.new_available || 0) > 0;
      if (allAdvanceDisabled && hasUnseen) {
        show('introduce-new-btn');
      } else {
        hide('introduce-new-btn');
      }
      show('success-state');
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
    const qs = params.toString();
    const url = qs ? `/api/quiz/next?${qs}` : '/api/quiz/next';
    currentCard = await apiFetch(url);
  } catch (e) {
    hide('card-area');
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
          btn.disabled = stats.available_to_advance < parseInt(btn.dataset.advance);
        });
        show('success-state');
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
    show('new-word-area');
    setText('new-word-zh', currentCard.prompt);
    setText('new-word-pinyin', currentCard.pinyin || '');
    const transLines = [];
    for (const texts of Object.values(currentCard.translations || {})) {
      if (texts && texts.length) transLines.push(texts.map(escHtml).join(' · '));
    }
    $('new-word-en').innerHTML = transLines.join('<br>') || '—';
    $('new-word-play-btn').onclick = () => playAudio(currentCard.word_id, currentCard.prompt);
    if (!currentCard.pinyin) hide('new-word-pinyin');
    loadNewWordBreakdown(currentCard.prompt);
    await loadStats();
    return;
  }

  // New component introduction
  if (currentCard.card_type === 'component' && currentCard.is_new) {
    hide('card-area');
    hide('new-word-area');
    show('new-component-area');
    setText('new-component-char', currentCard.prompt);
    const compPinyin = currentCard.pinyin || null;
    setText('new-component-pinyin', compPinyin || '');
    compPinyin ? show('new-component-pinyin-row') : hide('new-component-pinyin-row');
    const defs = currentCard.definitions || {};
    $('new-component-defs').innerHTML = Object.entries(defs).map(([lang, def]) =>
      `<div class="flex items-baseline gap-2 p-3 bg-purple-50 border border-purple-100 rounded-xl">
         <span class="text-xs font-semibold text-purple-500 uppercase w-6 shrink-0">${escHtml(lang)}</span>
         <span class="text-xl font-bold text-gray-800">${escHtml(def)}</span>
       </div>`
    ).join('');
    await loadStats();
    return;
  }

  hide('new-component-area');
  showCard();
  await loadStats();
}

function showCard() {
  show('card-area');

  if (currentCard.card_type === 'component') {
    setText('mode-label', t('component.modeLabel'));
    setText('prompt-word', currentCard.prompt);
    hide('play-btn');
    if (currentCard.pinyin) {
      setText('pinyin-hint', currentCard.pinyin);
      show('pinyin-hint');
    } else {
      hide('pinyin-hint');
    }
    hide('translations-hint');
    hide('hmm-type-badge');
    hide('hmm-actor-hint');
  } else if (currentCard.card_type === 'hmm') {
    setText('mode-label', t('hmm.modeLabel'));
    setText('prompt-word', currentCard.prompt);
    hide('play-btn');
    hide('pinyin-hint');
    hide('translations-hint');

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
  } else {
    hide('hmm-type-badge');
    hide('hmm-actor-hint');

    setText('mode-label', getModeLabel(currentCard.mode));
    setText('prompt-word', currentCard.prompt);

    // Show play button only when the prompt is Chinese
    const isZhPrompt = currentCard.mode === 'zh_to_transl' || currentCard.mode === 'zh_pinyin_to_transl';
    const playBtn = $('play-btn');
    if (isZhPrompt) {
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
      // Show all translations across all languages except the one already shown as prompt.
      const allTexts = Object.values(currentCard.translations || {}).flat();
      const others = allTexts.filter(t => t !== currentCard.prompt);
      if (others.length > 0) {
        $('translations-hint').innerHTML = others.map(escHtml).join(' · ');
        show('translations-hint');
      } else {
        hide('translations-hint');
      }
    } else {
      hide('translations-hint');
    }
  }

  $('answer-input').focus();
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
      showComponentResult(result);
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
      showHMMResult(result);
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

    hide('card-area');
    show('result-area');

    const icon = $('result-icon');
    if (result.correct) {
      icon.textContent = t('result.correct');
      icon.className = 'text-3xl font-bold text-green-600 mb-4';
    } else {
      icon.textContent = t('result.wrong');
      icon.className = 'text-3xl font-bold text-red-600 mb-4';
    }

    // Build breakdown for both correct and wrong answers
    const breakdown = $('word-breakdown');
    const pinyin = result.pinyin ? `<span class="text-gray-400 text-base ml-2">${escHtml(result.pinyin)}</span>` : '';
    const allTransTexts = selectedLangs.flatMap(lang => (result.translations || {})[lang] || []);
    const correctBox = `
      <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
        <div class="text-xs text-green-500 uppercase tracking-wide mb-1">${escHtml(t('result.correctLabel'))}</div>
        <div class="flex items-center gap-2">
          <div class="text-xl font-bold text-gray-800">${escHtml(result.zh_text)}${pinyin}</div>
          <button class="btn-breakdown-play text-xl text-gray-400 hover:text-blue-500 transition leading-none shrink-0" title="Read aloud">🔊</button>
        </div>
        <div class="text-gray-600 text-sm mt-0.5">${allTransTexts.map(escHtml).join(' · ')}</div>
      </div>`;

    if (!result.correct) {
      const isEmpty = answer.trim() === '';
      const cw = result.confused_with;
      const yourAnswerHtml = isEmpty ? '' : `
          <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
            <div class="text-xs text-red-400 uppercase tracking-wide mb-1">${escHtml(t('result.yourAnswer'))}</div>
            <div class="text-lg font-medium text-red-700">${escHtml(answer)}</div>
          </div>`;
      const confusedHtml = cw ? `
          <div class="p-3 bg-yellow-50 border border-yellow-200 rounded-xl">
            <div class="text-xs text-yellow-600 uppercase tracking-wide mb-1">${escHtml(t('result.belongsTo'))}</div>
            <div class="text-base font-semibold text-gray-800">${escHtml(cw.confused_with_text)}${cw.confused_with_pinyin ? `<span class="text-gray-400 text-sm ml-1">${escHtml(cw.confused_with_pinyin)}</span>` : ''}</div>
            <div class="text-gray-500 text-sm mt-0.5">${Object.values(cw.confused_with_translations || {}).flat().map(escHtml).join(' · ')}</div>
          </div>` : '';
      breakdown.innerHTML = `
        <div class="mt-4 space-y-2 text-left">
          ${yourAnswerHtml}
          ${confusedHtml}
          ${correctBox}
        </div>`;
      breakdown.querySelector('.btn-breakdown-play').addEventListener('click', () => playAudio(currentCard.word_id, result.zh_text));
      show('word-breakdown');

      if (!isEmpty) {
        const addBtn = $('add-translation-btn');
        addBtn.textContent = t('result.addTranslation', { answer });
        addBtn.disabled = false;
        addBtn.className = 'mt-3 mb-3 w-full border border-gray-300 hover:border-blue-400 text-gray-600 hover:text-blue-700 text-sm font-medium py-2 rounded-xl transition';
        show('add-translation-btn');

        addBtn.onclick = async () => {
          addBtn.disabled = true;
          try {
            await apiFetch(`/api/words/${currentCard.word_id}/translations`, {
              method: 'POST',
              body: JSON.stringify({ text: answer, lang: selectedLangs[0] || userPrimaryLang }),
            });
            addBtn.textContent = t('result.added');
            addBtn.className = 'mt-3 w-full border border-green-300 text-green-600 text-sm font-medium py-2 rounded-xl';
          } catch (err) {
            addBtn.disabled = false;
            alert('Could not add translation: ' + err.message);
          }
        };
      } else {
        hide('add-translation-btn');
      }
    } else {
      breakdown.innerHTML = `<div class="mt-4 space-y-2 text-left">${correctBox}</div>`;
      breakdown.querySelector('.btn-breakdown-play').addEventListener('click', () => playAudio(currentCard.word_id, result.zh_text));
      show('word-breakdown');
      hide('add-translation-btn');

      if (result.tier) {
        const bucketEl = $('bucket-info');
        bucketEl.textContent = result.prev_tier
          ? `${result.prev_tier} → ${result.tier}`
          : result.tier;
        show('bucket-info');
      } else {
        hide('bucket-info');
      }

      if (!result.learning_new_word && result.repetitions > 1) {
        $('streak-info').textContent = t('result.streak', { n: result.repetitions });
        show('streak-info');
      } else {
        hide('streak-info');
      }
    }

    if (result.graduated) {
      setText('next-due-info', t('result.graduated'));
    } else if (result.learning_new_word) {
      setText('next-due-info', t('result.learning', { n: result.graduate_reps }));
    } else {
      setText('next-due-info', t('result.nextReview', { n: result.interval_days }));
    }
    if (result.graduated) {
      setText('attempt-stats', ``);
    } else if (result.learning_new_word) {
      setText('attempt-stats', t('result.streakProgress', { n: result.repetitions, total: result.graduate_reps }));
    } else {
      const eff = result.total_correct + (result.streak_bonus || 0);
      setText('attempt-stats',
        t('result.correctStats', { eff, total: result.total_attempts }) +
        (result.streak_bonus > 0 ? ` (${t('result.streakBonus', { n: result.streak_bonus })})` : ''));
    }

    const reviewBtn = $('needs-review-btn');
    reviewBtn.textContent = t('result.flagReview');
    reviewBtn.disabled = false;
    reviewBtn.className = 'w-1/2 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
    reviewBtn.onclick = async () => {
      reviewBtn.disabled = true;
      try {
        await apiFetch(`/api/words/${currentCard.word_id}/review`, { method: 'POST' });
        reviewBtn.textContent = t('result.flagged');
        reviewBtn.className = 'w-1/2 border border-orange-200 text-orange-400 font-medium py-2 rounded-xl text-sm';
      } catch (err) {
        reviewBtn.disabled = false;
        alert('Could not flag word: ' + err.message);
      }
    };

    const editBtn = $('edit-card-btn');
    editBtn.onclick = () => window.open(`/vocab?edit=${currentCard.word_id}`, '_blank');
    show('review-edit-row');

    loadDecomposition(result.zh_text, 'result-decompose', 'result-decompose-toggle');

    // HMM mnemonic scene display
    const hmmEl = $('result-hmm');
    if (result.scene_text) {
      if (!result.correct) {
        // Wrong answer: auto-show scene
        renderHMMSceneReadOnly('result-hmm', result.scene_text);
        show('result-hmm');
      } else {
        // Correct answer: collapsed toggle
        hmmEl.innerHTML = `
          <button id="hmm-toggle-btn" type="button" class="text-sm text-purple-400 hover:text-purple-600 transition">&#9654; ${t('hmm.showMnemonic')}</button>
          <div id="hmm-toggle-content" class="hidden mt-2"></div>
        `;
        show('result-hmm');
        $('hmm-toggle-btn').addEventListener('click', () => {
          const content = $('hmm-toggle-content');
          if (content.classList.contains('hidden')) {
            renderHMMSceneReadOnly('hmm-toggle-content', result.scene_text);
            content.classList.remove('hidden');
            $('hmm-toggle-btn').innerHTML = `&#9660; ${t('hmm.hideMnemonic')}`;
          } else {
            content.classList.add('hidden');
            $('hmm-toggle-btn').innerHTML = `&#9654; ${t('hmm.showMnemonic')}`;
          }
        });
      }
    } else {
      hmmEl.innerHTML = `<a href="/vocab?edit=${currentCard.word_id}" target="_blank" class="text-sm text-purple-400 hover:text-purple-600 transition">+ ${t('hmm.createMnemonic')}</a>`;
      show('result-hmm');
    }

    $('next-btn').focus();
    await loadStats();
  } catch (err) {
    isSubmitted = false;
    alert('Error: ' + err.message);
  }
}

function showHMMResult(resp) {
  hide('card-area');
  show('result-area');

  const icon = $('result-icon');
  if (resp.correct) {
    icon.textContent = t('result.correct');
    icon.className = 'text-3xl font-bold text-green-600 mb-4';
  } else {
    icon.textContent = t('result.wrong');
    icon.className = 'text-3xl font-bold text-red-600 mb-4';
  }

  // Reuse word-breakdown for the answer display
  const badgeClass = HMM_TYPE_COLORS[currentCard.entity_type] || 'bg-gray-100 text-gray-700';
  const badgeHtml = `<span class="inline-block px-2 py-0.5 rounded-full text-xs font-bold uppercase tracking-wider ${escHtml(badgeClass)}">${escHtml(t('hmm.type.' + currentCard.entity_type))}</span>`;
  const yourAnswerHtml = (!resp.correct && resp.your_answer) ? `
    <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
      <div class="text-xs text-red-400 uppercase tracking-wide mb-1">${escHtml(t('result.yourAnswer'))}</div>
      <div class="text-lg font-medium text-red-700">${escHtml(resp.your_answer)}</div>
    </div>` : '';
  $('word-breakdown').innerHTML = `
    <div class="mt-4 space-y-2 text-left">
      ${yourAnswerHtml}
      <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
        <div class="text-xs text-green-500 uppercase tracking-wide mb-1">${badgeHtml} ${escHtml(currentCard.prompt)}</div>
        <div class="text-xl font-bold text-gray-800">${escHtml(resp.correct_answer)}</div>
      </div>
    </div>`;
  show('word-breakdown');

  hide('add-translation-btn');
  hide('result-hmm');
  hide('result-decompose');
  hide('result-decompose-content');
  hide('review-edit-row');

  if (resp.learning) {
    setText('next-due-info', t('pinyin.learning', { n: 3 }));
  } else {
    setText('next-due-info', t('result.nextReview', { n: resp.interval_days }));
  }

  if (resp.tier) {
    $('bucket-info').textContent = resp.prev_tier ? `${resp.prev_tier} → ${resp.tier}` : resp.tier;
    show('bucket-info');
  } else {
    hide('bucket-info');
  }

  const eff = resp.total_correct + (resp.streak_bonus || 0);
  setText('attempt-stats',
    t('result.correctStats', { eff, total: resp.total_attempts }) +
    (resp.streak_bonus > 0 ? ` (${t('result.streakBonus', { n: resp.streak_bonus })})` : ''));
  hide('streak-info');

  $('next-btn').focus();
  loadStats();
}

function showComponentResult(resp) {
  hide('card-area');
  show('result-area');

  const icon = $('result-icon');
  if (resp.correct) {
    icon.textContent = t('result.correct');
    icon.className = 'text-3xl font-bold text-green-600 mb-4';
  } else {
    icon.textContent = t('result.wrong');
    icon.className = 'text-3xl font-bold text-red-600 mb-4';
  }

  const yourAnswerHtml = (!resp.correct && $('answer-input').value.trim()) ? `
    <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
      <div class="text-xs text-red-400 uppercase tracking-wide mb-1">${escHtml(t('result.yourAnswer'))}</div>
      <div class="text-lg font-medium text-red-700">${escHtml($('answer-input').value)}</div>
    </div>` : '';

  const answers = resp.correct_answers || {};
  const defsHtml = Object.entries(answers).map(([lang, def]) =>
    `<div class="flex items-baseline gap-2">
       <span class="text-xs font-semibold text-green-600 uppercase w-6 shrink-0">${escHtml(lang)}</span>
       <span class="text-xl font-bold text-gray-800">${escHtml(def)}</span>
     </div>`
  ).join('');

  $('word-breakdown').innerHTML = `
    <div class="mt-4 space-y-2 text-left">
      ${yourAnswerHtml}
      <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
        <div class="text-xs text-green-500 uppercase tracking-wide mb-1">${escHtml(t('component.character'))}: ${escHtml(currentCard.prompt)}</div>
        ${defsHtml}
      </div>
    </div>`;
  show('word-breakdown');

  hide('add-translation-btn');
  hide('result-decompose');
  hide('result-decompose-content');
  hide('bucket-info');
  hide('streak-info');

  const hmmEl = $('result-hmm');
  if (resp.scene_text) {
    if (!resp.correct) {
      renderHMMSceneReadOnly('result-hmm', resp.scene_text);
      show('result-hmm');
    } else {
      hmmEl.innerHTML = `
        <button id="hmm-toggle-btn" type="button" class="text-sm text-purple-400 hover:text-purple-600 transition">&#9654; ${t('hmm.showMnemonic')}</button>
        <div id="hmm-toggle-content" class="hidden mt-2"></div>
      `;
      show('result-hmm');
      $('hmm-toggle-btn').addEventListener('click', () => {
        const content = $('hmm-toggle-content');
        if (content.classList.contains('hidden')) {
          renderHMMSceneReadOnly('hmm-toggle-content', resp.scene_text);
          content.classList.remove('hidden');
          $('hmm-toggle-btn').innerHTML = `&#9660; ${t('hmm.hideMnemonic')}`;
        } else {
          content.classList.add('hidden');
          $('hmm-toggle-btn').innerHTML = `&#9654; ${t('hmm.showMnemonic')}`;
        }
      });
    }
  } else {
    hmmEl.innerHTML = `<a href="/vocab?editComp=${encodeURIComponent(currentCard.prompt)}" target="_blank" class="text-sm text-purple-400 hover:text-purple-600 transition">+ ${t('hmm.createMnemonic')}</a>`;
    show('result-hmm');
  }

  const reviewBtn = $('needs-review-btn');
  reviewBtn.textContent = t('result.flagReview');
  reviewBtn.disabled = false;
  reviewBtn.className = 'w-1/2 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
  reviewBtn.onclick = async () => {
    reviewBtn.disabled = true;
    try {
      await apiFetch(`/api/components/${encodeURIComponent(currentCard.prompt)}/review`, { method: 'POST' });
      reviewBtn.textContent = t('result.flagged');
      reviewBtn.className = 'w-1/2 border border-orange-200 text-orange-400 font-medium py-2 rounded-xl text-sm';
    } catch (err) {
      reviewBtn.disabled = false;
      alert('Could not flag component: ' + err.message);
    }
  };

  const editBtn = $('edit-card-btn');
  editBtn.onclick = () => window.open(`/vocab?editComp=${encodeURIComponent(currentCard.prompt)}`, '_blank');
  show('review-edit-row');

  setText('next-due-info', t('result.nextReview', { n: resp.interval_days }));
  const eff = resp.total_correct;
  setText('attempt-stats', t('result.correctStats', { eff, total: resp.total_attempts }));

  $('next-btn').focus();
  loadStats();
}

function renderCharDecomposition(charData) {
  let html = `<div class="p-3 bg-gray-50 border border-gray-200 rounded-xl mb-2">`;
  html += `<div class="flex items-baseline gap-2 mb-1">`;
  html += `<span class="text-2xl font-bold">${escHtml(charData.character)}</span>`;
  if (charData.radical) {
    html += `<span class="text-sm text-gray-400">${escHtml(t('decompose.radical', { r: charData.radical }))}</span>`;
  }
  if (charData.definition) {
    html += `<span class="text-sm text-gray-500">${escHtml(charData.definition)}</span>`;
  }
  html += `</div>`;

  if (charData.etymology && charData.etymology.hint) {
    html += `<div class="text-xs text-gray-400 italic mb-2">${escHtml(charData.etymology.hint)}</div>`;
  }

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

async function loadDecomposition(zhText, containerId, toggleId) {
  try {
    const data = await apiFetch(`/api/hanzi/decompose?chars=${encodeURIComponent(zhText)}`);
    if (!data || data.length === 0) return;

    show(containerId);
    const toggle = $(toggleId);
    const content = $(containerId + '-content');

    content.innerHTML = data.map(renderCharDecomposition).join('');

    toggle.innerHTML = `&#9654; ${escHtml(t('result.charBreakdown'))}`;
    toggle.onclick = () => {
      if (content.classList.contains('hidden')) {
        content.classList.remove('hidden');
        toggle.innerHTML = `&#9660; ${escHtml(t('result.charBreakdown'))}`;
      } else {
        content.classList.add('hidden');
        toggle.innerHTML = `&#9654; ${escHtml(t('result.charBreakdown'))}`;
      }
    };
  } catch (_) {}
}

async function loadNewWordBreakdown(zhText) {
  const container = $('new-word-breakdown');
  container.innerHTML = '';
  hide('new-word-breakdown');
  try {
    const langs = [userPrimaryLang, userSecondaryLang].filter(Boolean);
    const langsParam = langs.join(',');
    const data = await apiFetch(`/api/hanzi/decompose?chars=${encodeURIComponent(zhText)}&mark_new=true&langs=${encodeURIComponent(langsParam)}`);
    if (!data || data.length === 0) return;
    // Collect all semantic components that have a definition in at least one requested lang.
    const comps = [];
    for (const charData of data) {
      for (const comp of (charData.components || [])) {
        if (comp.is_semantic === false) continue;
        const defs = comp.definitions || {};
        const hasDef = langs.some(l => defs[l.toLowerCase()]) || comp.definition;
        if (!hasDef) continue;
        comps.push(comp);
      }
    }
    if (comps.length === 0) return;
    let html = `<div class="mt-5 text-left border-t border-gray-100 pt-4">`;
    html += `<div class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-3">${escHtml(t('newWord.components'))}</div>`;
    html += `<div class="space-y-2">`;
    for (const comp of comps) {
      const isNew = comp.is_new_component === true;
      const defs = comp.definitions || {};
      const defParts = langs.map(l => defs[l.toLowerCase()]).filter(Boolean);
      const defText = defParts.length > 0 ? defParts.join(' · ') : (comp.definition || '');
      html += `<div class="flex items-center gap-3">`;
      html += `<span class="text-2xl font-bold text-gray-800 w-8 shrink-0">${escHtml(comp.character)}</span>`;
      html += `<span class="text-sm text-gray-600 flex-1">${escHtml(defText)}</span>`;
      if (isNew) {
        html += `<span class="text-xs font-semibold text-purple-600 bg-purple-50 border border-purple-200 px-2 py-0.5 rounded-full shrink-0">${escHtml(t('newWord.componentNew'))}</span>`;
      }
      html += `</div>`;
    }
    html += `</div></div>`;
    container.innerHTML = html;
    show('new-word-breakdown');
  } catch (_) {}
}

function applyLangChips(allLangs) {
  const desktopContainer = $('lang-chips-desktop');
  desktopContainer.querySelectorAll('.lang-pill').forEach(p => p.remove());
  const overlayContainer = $('overlay-lang-chips');
  overlayContainer.innerHTML = '';

  $('overlay-langs-section').classList.toggle('hidden', allLangs.length < 2);

  for (const lang of allLangs) {
    const active = selectedLangs.includes(lang);

    // Desktop chip — insert before the separator (third child: label, sep, mnemonics-pill)
    const sep = desktopContainer.querySelector('span.text-gray-300');
    const pill = document.createElement('button');
    pill.className = `lang-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    pill.textContent = lang.toUpperCase();
    pill.addEventListener('click', () => toggleLang(lang, allLangs));
    desktopContainer.insertBefore(pill, sep);

    // Overlay chip
    const overlayPill = document.createElement('button');
    overlayPill.className = `overlay-lang-btn px-3 py-1.5 rounded-full text-sm font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    overlayPill.textContent = lang.toUpperCase();
    overlayPill.addEventListener('click', () => toggleLang(lang, allLangs));
    overlayContainer.appendChild(overlayPill);
  }
}

function toggleLang(lang, allLangs) {
  if (selectedLangs.includes(lang)) {
    // Don't allow deselecting the last lang
    if (selectedLangs.length <= 1) return;
    selectedLangs = selectedLangs.filter(l => l !== lang);
  } else {
    selectedLangs.push(lang);
  }
  localStorage.setItem('quizLangs', JSON.stringify(selectedLangs));
  applyLangChips(allLangs);
  loadNextCard();
}

async function loadLangs() {
  let availableLangs = [];
  try {
    availableLangs = await apiFetch('/api/quiz/langs');
  } catch (_) {}
  await _settingsPromise;
  // Only show langs the user has configured, in primary-first order
  const userLangs = [userPrimaryLang, userSecondaryLang].filter(l => l && availableLangs.includes(l));
  const allLangs = userLangs.length > 0 ? userLangs : availableLangs;
  // Prune stale selections
  selectedLangs = selectedLangs.filter(l => allLangs.includes(l));
  if (selectedLangs.length === 0) {
    selectedLangs = allLangs.length > 0 ? [allLangs[0]] : [userPrimaryLang];
  }
  localStorage.setItem('quizLangs', JSON.stringify(selectedLangs));
  applyLangChips(allLangs);
}

async function loadTrainTags() {
  let allTags = [];
  try {
    allTags = await apiFetch('/api/tags');
  } catch (_) {}

  // Remove stale tags from selection
  selectedTags = selectedTags.filter(t => allTags.includes(t));
  localStorage.setItem('quizTags', JSON.stringify(selectedTags));

  // Desktop: render tag pills into the tag bar
  const tagBar = $('tag-filter-bar');
  const desktopContainer = $('tag-chips-desktop');
  desktopContainer.querySelectorAll('.tag-pill').forEach(p => p.remove());
  if (allTags.length === 0) {
    tagBar.classList.add('hidden');
    $('overlay-tags-section').classList.add('hidden');
    return;
  }
  tagBar.classList.remove('hidden');
  for (const tag of allTags) {
    const pill = document.createElement('button');
    const active = selectedTags.includes(tag);
    pill.className = `tag-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    pill.textContent = tag;
    pill.addEventListener('click', () => {
      if (selectedTags.includes(tag)) {
        selectedTags = selectedTags.filter(t => t !== tag);
      } else {
        selectedTags.push(tag);
      }
      localStorage.setItem('quizTags', JSON.stringify(selectedTags));
      loadTrainTags();
      loadNextCard();
    });
    desktopContainer.appendChild(pill);
  }

  // Overlay: render all tag chips with toggle behaviour
  const overlayTagChips = $('overlay-tag-chips');
  overlayTagChips.innerHTML = '';
  for (const tag of allTags) {
    const pill = document.createElement('button');
    const active = selectedTags.includes(tag);
    pill.className = `overlay-tag-btn px-3 py-1.5 rounded-full text-sm font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    pill.textContent = tag;
    pill.addEventListener('click', () => {
      if (selectedTags.includes(tag)) {
        selectedTags = selectedTags.filter(t => t !== tag);
      } else {
        selectedTags.push(tag);
      }
      localStorage.setItem('quizTags', JSON.stringify(selectedTags));
      loadTrainTags();
    });
    overlayTagChips.appendChild(pill);
  }
  $('overlay-tags-section').classList.remove('hidden');
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
    applyMnemonicPill();
    loadNextCard();
  }
  const mnemonicsPill = $('mnemonics-pill');
  if (mnemonicsPill) mnemonicsPill.addEventListener('click', toggleMnemonics);
  const overlayMnemonicsPill = $('overlay-mnemonics-pill');
  if (overlayMnemonicsPill) overlayMnemonicsPill.addEventListener('click', toggleMnemonics);

  function toggleComponents() {
    includeComponents = !includeComponents;
    localStorage.setItem('quizComponents', includeComponents ? 'true' : 'false');
    applyComponentPill();
    loadNextCard();
  }
  const componentsPill = $('components-pill');
  if (componentsPill) componentsPill.addEventListener('click', toggleComponents);
  const overlayComponentsPill = $('overlay-components-pill');
  if (overlayComponentsPill) overlayComponentsPill.addEventListener('click', toggleComponents);

  document.querySelectorAll('.tier-pill').forEach(btn => {
    btn.addEventListener('click', () => {
      selectedBucket = btn.dataset.bucket;
      localStorage.setItem('quizBucket', selectedBucket);
      applyTierPills();
      loadNextCard();
    });
  });
  document.querySelectorAll('.overlay-tier-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      selectedBucket = btn.dataset.bucket;
      localStorage.setItem('quizBucket', selectedBucket);
      applyTierPills();
    });
  });
  document.querySelectorAll('.mode-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      selectedMode = btn.dataset.mode;
      localStorage.setItem('quizMode', selectedMode);
      applyModeButtons();
      loadNextCard();
    });
  });
  $('answer-form').addEventListener('submit', submitAnswer);
  $('next-btn').addEventListener('click', loadNextCard);

  // Mobile filter overlay
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
      applyModeButtons();
    });
  });


  // Skip current card for today (advance due_date by 1 day).
  $('skip-today-btn').addEventListener('click', async () => {
    if (isSubmitted || !currentCard) return;
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
      const resetNewCap = $('reset-cap-checkbox').checked;
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
  });

  // Onboarding import (shown when user has zero words)
  let obAllTags = [];
  let obSelectedTag = '';
  let obFilterLangs = new Set();
  let obFilterMode = 'any';
  let obApplyTags = [];

  function obTagMatchesFilter(tag) {
    if (obFilterLangs.size === 0) return true;
    const langs = tag.available_langs || [];
    if (obFilterMode === 'all') {
      for (const lang of obFilterLangs) {
        if (!langs.includes(lang)) return false;
      }
      return true;
    }
    for (const lang of obFilterLangs) {
      if (langs.includes(lang)) return true;
    }
    return false;
  }

  function obRenderTagPills() {
    const list = $('ob-tag-list');
    list.innerHTML = '';
    const visible = obAllTags.filter(obTagMatchesFilter);
    if (visible.length === 0) {
      list.innerHTML = `<span class="text-sm text-gray-400">${escHtml(obAllTags.length === 0 ? t('vocab.importNoTags') : t('vocab.importNoTagsMatch'))}</span>`;
      if (obSelectedTag) { obSelectedTag = ''; $('ob-next-btn').disabled = true; hide('ob-preview'); }
      return;
    }
    let selectedStillVisible = false;
    for (const tag of visible) {
      const pill = document.createElement('button');
      pill.type = 'button';
      const isSelected = tag.name === obSelectedTag;
      if (isSelected) selectedStillVisible = true;
      pill.className = 'px-3 py-1 rounded-full text-sm font-medium border transition ' +
        (isSelected ? 'bg-blue-600 text-white border-blue-600' : 'border-gray-300 text-gray-600 hover:bg-blue-50 hover:border-blue-400 hover:text-blue-600');
      pill.textContent = tag.name;
      if (tag.description) pill.title = tag.description;
      pill.addEventListener('click', () => obSelectTag(tag));
      list.appendChild(pill);
    }
    if (!selectedStillVisible && obSelectedTag) { obSelectedTag = ''; $('ob-next-btn').disabled = true; hide('ob-preview'); }
  }

  async function obSelectTag(tag) {
    obSelectedTag = tag.name;
    $('ob-next-btn').disabled = true;
    hide('ob-preview');
    obRenderTagPills();
    const descEl = $('ob-preview-desc');
    const statsEl = $('ob-preview-stats');
    const tableWrap = $('ob-preview-table-wrap');
    const tbody = $('ob-preview-tbody');
    statsEl.textContent = t('vocab.importLoading');
    descEl.classList.add('hidden');
    tableWrap.classList.add('hidden');
    tbody.innerHTML = '';
    show('ob-preview');
    try {
      const data = await apiFetch('/api/import/preview?tag=' + encodeURIComponent(tag.name));
      if (tag.description) { descEl.textContent = tag.description; descEl.classList.remove('hidden'); }
      if (data.total === 0) { statsEl.textContent = t('vocab.importPreviewEmpty'); $('ob-next-btn').disabled = true; return; }
      const parts = [`${data.total} ${t('vocab.importPreviewWords')}`];
      for (const [lang, count] of Object.entries(data.available_langs || {}).sort()) {
        if (count > 0) parts.push(`${count} ${lang.toUpperCase()}`);
      }
      statsEl.textContent = parts.join(' · ');
      const hasDe = (data.examples || []).some(e => (e.translations || {})['de']?.length > 0);
      for (const ex of (data.examples || [])) {
        const tr = document.createElement('tr');
        tr.className = 'border-b border-gray-100 last:border-0';
        const exTransl = ex.translations || {};
        const en = (exTransl['en'] || []).map(escHtml).join(', ') || '<span class="text-gray-300">—</span>';
        const de = (exTransl['de'] || []).map(escHtml).join(', ') || '<span class="text-gray-300">—</span>';
        tr.innerHTML = `<td class="py-1 px-2 font-medium">${escHtml(ex.zh_text)}</td><td class="py-1 px-2 text-gray-500">${escHtml(ex.pinyin)}</td><td class="py-1 px-2 text-gray-700">${en}</td><td class="py-1 px-2 text-gray-500">${hasDe ? de : ''}</td>`;
        tbody.appendChild(tr);
      }
      tableWrap.classList.remove('hidden');
      $('ob-next-btn').disabled = false;
    } catch (e) {
      statsEl.textContent = e.message;
      $('ob-next-btn').disabled = true;
    }
  }

  function obShowStep(n) {
    [1, 2, 3].forEach(i => {
      const el = $('ob-step' + i);
      if (el) el.classList.toggle('hidden', i !== n);
    });
  }

  async function obLoadTags() {
    const list = $('ob-tag-list');
    try {
      obAllTags = await apiFetch('/api/import/source-tags');
      obRenderTagPills();
    } catch (e) {
      list.innerHTML = `<span class="text-sm text-red-500">${escHtml(e.message)}</span>`;
    }
  }

  async function obExecuteImport() {
    const btn = $('ob-submit-btn');
    const statusEl = $('ob-status');
    btn.disabled = true;
    btn.textContent = t('vocab.importing');
    statusEl.className = 'mt-3 text-sm text-gray-500';
    statusEl.textContent = '';
    show('ob-status');
    try {
      const result = await apiFetch('/api/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tag: obSelectedTag,
          import_en: $('ob-import-en').checked,
          import_de: $('ob-import-de').checked,
          apply_tags: [...obApplyTags],
        }),
      });
      const startCount = parseInt($('ob-start-count').value) || result.imported;
      await apiFetch('/api/quiz/acknowledge-random', {
        method: 'POST',
        body: JSON.stringify({ count: startCount }),
      }).catch(() => {});
      const skippedNote = result.skipped > 0 ? `, ${t('vocab.importSkipped')} ${result.skipped} ${t('vocab.importAlreadyOwned')}` : '';
      statusEl.className = 'mt-3 text-sm text-green-600';
      statusEl.textContent = `${t('vocab.importDone')} ${result.imported} ${t('vocab.importWords2')}${skippedNote}.`;
      setTimeout(() => {
        hide('empty-state');
        loadNextCard();
      }, 1200);
    } catch (e) {
      statusEl.className = 'mt-3 text-sm text-red-500';
      statusEl.textContent = e.message;
      btn.disabled = false;
      btn.textContent = t('vocab.import');
    }
  }

  function obRenderApplyTags() {
    const container = $('ob-apply-tags');
    container.innerHTML = '';
    for (const tag of obApplyTags) {
      const pill = document.createElement('span');
      pill.className = 'inline-flex items-center bg-gray-200 text-gray-700 text-sm px-2 py-0.5 rounded-full';
      pill.innerHTML = `${escHtml(tag)} <button type="button" class="ml-1 text-gray-400 hover:text-red-500 leading-none">&times;</button>`;
      pill.querySelector('button').addEventListener('click', () => {
        obApplyTags = obApplyTags.filter(t => t !== tag);
        obRenderApplyTags();
      });
      container.appendChild(pill);
    }
  }

  function obAddTag(tag) {
    tag = tag.trim();
    if (!tag || obApplyTags.includes(tag)) return;
    obApplyTags.push(tag);
    obRenderApplyTags();
    $('ob-tag-input').value = '';
    $('ob-tag-autocomplete').classList.add('hidden');
  }

  function obShowTagAutocomplete(query) {
    const dropdown = $('ob-tag-autocomplete');
    const q = query.toLowerCase();
    const tagNames = obAllTags.map(t => t.name);
    const matches = tagNames.filter(n => n.toLowerCase().includes(q) && !obApplyTags.includes(n));
    if (query && !tagNames.includes(query) && !obApplyTags.includes(query)) matches.push(query);
    if (!matches.length) { dropdown.classList.add('hidden'); return; }
    dropdown.innerHTML = '';
    dropdown.classList.remove('hidden');
    for (const m of matches) {
      const item = document.createElement('div');
      item.className = 'px-3 py-1.5 text-sm hover:bg-blue-50 cursor-pointer';
      item.textContent = m === query && !tagNames.includes(query) ? t('vocab.createTag', { tag: m }) : m;
      item.addEventListener('mousedown', e => { e.preventDefault(); obAddTag(m); });
      dropdown.appendChild(item);
    }
  }

  $('ob-tag-input').addEventListener('input', e => obShowTagAutocomplete(e.target.value));
  $('ob-tag-input').addEventListener('keydown', e => {
    if (e.key === 'Enter') { e.preventDefault(); obAddTag(e.target.value); }
    if (e.key === 'Escape') $('ob-tag-autocomplete').classList.add('hidden');
  });
  $('ob-tag-input').addEventListener('blur', () => setTimeout(() => $('ob-tag-autocomplete').classList.add('hidden'), 150));

  // Wire up onboarding filter buttons
  ['en', 'de'].forEach(lang => {
    $('ob-filter-' + lang).addEventListener('click', () => {
      const btn = $('ob-filter-' + lang);
      if (obFilterLangs.has(lang)) {
        obFilterLangs.delete(lang);
        btn.classList.remove('bg-blue-600', 'text-white', 'border-blue-600');
        btn.classList.add('border-gray-300', 'text-gray-500');
      } else {
        obFilterLangs.add(lang);
        btn.classList.add('bg-blue-600', 'text-white', 'border-blue-600');
        btn.classList.remove('border-gray-300', 'text-gray-500');
      }
      obRenderTagPills();
    });
  });
  document.querySelectorAll('input[name="ob-filter-mode"]').forEach(radio => {
    radio.addEventListener('change', () => { obFilterMode = radio.value; obRenderTagPills(); });
  });
  $('ob-next-btn').addEventListener('click', () => obShowStep(2));
  $('ob-back1-btn').addEventListener('click', () => obShowStep(1));
  $('ob-next2-btn').addEventListener('click', () => obShowStep(3));
  $('ob-back2-btn').addEventListener('click', () => obShowStep(2));
  $('ob-submit-btn').addEventListener('click', obExecuteImport);

  document.addEventListener('ob:loadtags', obLoadTags, { once: false });

  loadNextCard();
});
