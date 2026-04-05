// Training page logic

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

async function loadStats() {
  try {
    const params = new URLSearchParams();
    if (selectedTags.length) params.set('tags', selectedTags.join(','));
    if (selectedBucket) params.set('bucket', selectedBucket);
    const qs = params.toString();
    const statsUrl = qs ? `/api/quiz/stats?${qs}` : '/api/quiz/stats';
    const stats = await apiFetch(statsUrl);
    latestStats = stats;
    setText('stats-due', stats.due_today + (stats.hmm_due_today || 0));
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
  hide('result-decompose');
  hide('result-decompose-content');
  hide('bucket-info');
  hide('streak-info');
  $('answer-input').value = '';
  const reviewBtn = $('needs-review-btn');
  reviewBtn.textContent = t('result.flagReview');
  reviewBtn.disabled = false;
  reviewBtn.className = 'w-full mb-3 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
  reviewBtn.onclick = null;

  // Fetch fresh stats first. The backend's GetNextCard may return non-due
  // (future) cards via its fallback queries even when due_today = 0, so we
  // cannot rely solely on a 404 "no words available" to trigger the success
  // screen — we must check due_today proactively.
  await loadStats();

  if (latestStats) {
    if (latestStats.total === 0) {
      show('empty-state');
      return;
    }
    if (latestStats.due_today === 0 && (latestStats.hmm_due_today || 0) === 0 && (!latestStats.new_available || skipNewWords)) {
      skipNewWords = false;
      setText('success-stats', t('stats.attemptsAndMistakes', { attempts: latestStats.today_attempts, mistakes: latestStats.today_mistakes }));
      document.querySelectorAll('.advance-btn').forEach(btn => {
        btn.disabled = latestStats.available_to_advance < parseInt(btn.dataset.advance);
      });
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
        show('empty-state');
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
    show('new-word-area');
    setText('new-word-zh', currentCard.prompt);
    setText('new-word-pinyin', currentCard.pinyin || '');
    const transLines = [];
    if (currentCard.en_texts && currentCard.en_texts.length) transLines.push(currentCard.en_texts.map(escHtml).join(' · '));
    if (currentCard.de_texts && currentCard.de_texts.length) transLines.push(currentCard.de_texts.map(escHtml).join(' · '));
    $('new-word-en').innerHTML = transLines.join('<br>') || '—';
    $('new-word-play-btn').onclick = () => playAudio(currentCard.word_id, currentCard.prompt);
    if (!currentCard.pinyin) hide('new-word-pinyin');
    await loadStats();
    return;
  }

  showCard();
  await loadStats();
}

function showCard() {
  show('card-area');

  if (currentCard.card_type === 'hmm') {
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
    const isZhPrompt = currentCard.mode === 'zh_to_en' || currentCard.mode === 'zh_pinyin_to_en';
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

    if (currentCard.mode === 'en_to_zh' && currentCard.en_texts && currentCard.en_texts.length > 1) {
      const others = currentCard.en_texts.filter(t => t !== currentCard.prompt);
      $('translations-hint').innerHTML = others.map(escHtml).join(' · ');
      show('translations-hint');
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
    const allTransTexts = [
      ...(result.en_texts || []),
      ...(result.de_texts || []),
    ];
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
            <div class="text-gray-500 text-sm mt-0.5">${(cw.confused_with_en_texts || []).map(escHtml).join(' · ')}</div>
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
        addBtn.className = 'mt-3 w-full border border-gray-300 hover:border-blue-400 text-gray-600 hover:text-blue-700 text-sm font-medium py-2 rounded-xl transition';
        show('add-translation-btn');

        addBtn.onclick = async () => {
          addBtn.disabled = true;
          try {
            await apiFetch(`/api/words/${currentCard.word_id}/translations`, {
              method: 'POST',
              body: JSON.stringify({ en_text: answer }),
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
    reviewBtn.className = 'w-full mb-3 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
    reviewBtn.onclick = async () => {
      reviewBtn.disabled = true;
      try {
        await apiFetch(`/api/words/${currentCard.word_id}/review`, { method: 'POST' });
        reviewBtn.textContent = t('result.flagged');
        reviewBtn.className = 'w-full mb-3 border border-orange-200 text-orange-400 font-medium py-2 rounded-xl text-sm';
      } catch (err) {
        reviewBtn.disabled = false;
        alert('Could not flag word: ' + err.message);
      }
    };

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
  hide('needs-review-btn');

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
      html += `<div class="px-2 py-1 bg-white border border-gray-200 rounded-lg text-center min-w-[3rem]">`;
      html += `<div class="text-lg font-medium">${escHtml(comp.character)}</div>`;
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

function applyLangChips(allLangs) {
  const bar = $('lang-filter-bar');
  const desktopContainer = $('lang-chips-desktop');
  desktopContainer.querySelectorAll('.lang-pill').forEach(p => p.remove());
  const overlayContainer = $('overlay-lang-chips');
  overlayContainer.innerHTML = '';

  if (allLangs.length < 2) {
    bar.classList.add('hidden');
    $('overlay-langs-section').classList.add('hidden');
    return;
  }

  bar.classList.remove('hidden');
  $('overlay-langs-section').classList.remove('hidden');

  for (const lang of allLangs) {
    const active = selectedLangs.includes(lang);

    // Desktop chip
    const pill = document.createElement('button');
    pill.className = `lang-pill px-2.5 py-0.5 rounded-full text-xs font-medium transition ${active ? 'bg-blue-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200'}`;
    pill.textContent = lang.toUpperCase();
    pill.addEventListener('click', () => toggleLang(lang, allLangs));
    desktopContainer.appendChild(pill);

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
  let allLangs = [];
  try {
    allLangs = await apiFetch('/api/quiz/langs');
  } catch (_) {}
  // Prune stale selections
  selectedLangs = selectedLangs.filter(l => allLangs.includes(l));
  if (selectedLangs.length === 0) {
    selectedLangs = allLangs.length > 0 ? [allLangs[0]] : ['en'];
  }
  localStorage.setItem('quizLangs', JSON.stringify(selectedLangs));
  applyLangChips(allLangs);
}

async function loadTrainTags() {
  let allTags = [];
  try {
    allTags = await apiFetch('/api/tags');
  } catch (_) {}
  const bar = $('tag-filter-bar');
  const desktopContainer = $('tag-chips-desktop');
  desktopContainer.querySelectorAll('.tag-pill').forEach(p => p.remove());
  if (allTags.length === 0) {
    bar.classList.add('hidden');
    $('overlay-tags-section').classList.add('hidden');
    return;
  }
  // Tag bar is desktop-only; on mobile the overlay handles tag selection.
  bar.classList.remove('hidden');
  // Remove stale tags from selection
  selectedTags = selectedTags.filter(t => allTags.includes(t));
  localStorage.setItem('quizTags', JSON.stringify(selectedTags));

  // Desktop: render all tag chips
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
  loadLangs();
  loadTrainTags();

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

  // Re-render dynamic text when language changes
  document.addEventListener('langchange', () => {
    applyModeButtons();
    applyTierPills();
  });

  loadNextCard();
});
