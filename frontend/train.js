// Training page logic

let currentCard = null;
let isSubmitted = false;
let selectedMode = localStorage.getItem('quizMode') || 'random';
let selectedTags = JSON.parse(localStorage.getItem('quizTags') || '[]');

function applyModeButtons() {
  document.querySelectorAll('.mode-btn').forEach(btn => {
    const active = btn.dataset.mode === selectedMode;
    btn.className = active
      ? 'mode-btn px-3 py-1 rounded-full text-sm font-medium transition bg-blue-600 text-white'
      : 'mode-btn px-3 py-1 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  });
  // Mobile: update the single visible label from the active desktop button's text
  const activeBtn = document.querySelector(`.mode-btn[data-mode="${selectedMode}"]`);
  const mobileLabel = document.getElementById('mode-mobile-label');
  if (mobileLabel && activeBtn) mobileLabel.textContent = activeBtn.textContent;
  // Overlay: apply same active/inactive styling
  document.querySelectorAll('.overlay-mode-btn').forEach(btn => {
    const active = btn.dataset.mode === selectedMode;
    btn.className = active
      ? 'overlay-mode-btn px-4 py-2 rounded-full text-sm font-medium transition bg-blue-600 text-white'
      : 'overlay-mode-btn px-4 py-2 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  });
}

async function loadStats() {
  try {
    const statsUrl = selectedTags.length ? `/api/quiz/stats?tags=${selectedTags.join(',')}` : '/api/quiz/stats';
    const stats = await apiFetch(statsUrl);
    setText('stats-due', stats.due_today);
    setText('stats-total', stats.total);
    setText('stats-new', `${stats.new_today} / ${stats.max_new_per_day}`);
  } catch (_) {}
}

async function loadNextCard() {
  isSubmitted = false;
  hide('result-area');
  hide('empty-state');
  hide('error-state');
  hide('add-translation-btn');
  hide('result-play-btn');
  $('answer-input').value = '';
  const reviewBtn = $('needs-review-btn');
  reviewBtn.textContent = 'Flag for Review';
  reviewBtn.disabled = false;
  reviewBtn.className = 'w-full mb-3 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
  reviewBtn.onclick = null;

  try {
    const params = new URLSearchParams();
    if (selectedMode !== 'random') params.set('mode', selectedMode);
    if (selectedTags.length) params.set('tags', selectedTags.join(','));
    const qs = params.toString();
    const url = qs ? `/api/quiz/next?${qs}` : '/api/quiz/next';
    currentCard = await apiFetch(url);
  } catch (e) {
    if (e.message === 'no words available') {
      hide('card-area');
      show('empty-state');
    } else {
      hide('card-area');
      show('error-state');
      setText('error-msg', e.message);
    }
    return;
  }

  show('card-area');
  setText('mode-label', MODE_LABELS[currentCard.mode] || currentCard.mode);
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

  $('answer-input').focus();
  await loadStats();
}

async function submitAnswer(e) {
  e.preventDefault();
  if (isSubmitted || !currentCard) return;
  isSubmitted = true;

  const answer = $('answer-input').value;

  try {
    const result = await apiFetch('/api/quiz/answer', {
      method: 'POST',
      body: JSON.stringify({
        word_id: currentCard.word_id,
        mode: currentCard.mode,
        answer: answer,
      }),
    });

    hide('card-area');
    show('result-area');

    const icon = $('result-icon');
    if (result.correct) {
      icon.textContent = '✓ Correct!';
      icon.className = 'text-3xl font-bold text-green-600 mb-4';
    } else {
      icon.textContent = '✗ Wrong';
      icon.className = 'text-3xl font-bold text-red-600 mb-4';
    }

    // Build breakdown for both correct and wrong answers
    const breakdown = $('word-breakdown');
    const pinyin = result.pinyin ? `<span class="text-gray-400 text-base ml-2">${escHtml(result.pinyin)}</span>` : '';
    const correctBox = `
      <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
        <div class="text-xs text-green-500 uppercase tracking-wide mb-1">Correct</div>
        <div class="flex items-center gap-2">
          <div class="text-xl font-bold text-gray-800">${escHtml(result.zh_text)}${pinyin}</div>
          <button class="btn-breakdown-play text-xl text-gray-400 hover:text-blue-500 transition leading-none shrink-0" title="Read aloud">🔊</button>
        </div>
        <div class="text-gray-600 text-sm mt-0.5">${result.en_texts.map(escHtml).join(' · ')}</div>
      </div>`;

    if (!result.correct) {
      const isEmpty = answer.trim() === '';
      const cw = result.confused_with;
      const yourAnswerHtml = isEmpty ? '' : `
          <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
            <div class="text-xs text-red-400 uppercase tracking-wide mb-1">Your answer</div>
            <div class="text-lg font-medium text-red-700">${escHtml(answer)}</div>
          </div>`;
      const confusedHtml = cw ? `
          <div class="p-3 bg-yellow-50 border border-yellow-200 rounded-xl">
            <div class="text-xs text-yellow-600 uppercase tracking-wide mb-1">Your answer belongs to</div>
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
        addBtn.textContent = `Add "${answer}" as correct answer`;
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
            addBtn.textContent = '✓ Added';
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
    }

    setText('next-due-info', `Next review in ${result.interval_days} day(s)`);
    setText('attempt-stats',
      `Correct: ${result.total_correct} / ${result.total_attempts}`);

    const reviewBtn = $('needs-review-btn');
    reviewBtn.textContent = 'Flag for Review';
    reviewBtn.disabled = false;
    reviewBtn.className = 'w-full mb-3 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
    reviewBtn.onclick = async () => {
      reviewBtn.disabled = true;
      try {
        await apiFetch(`/api/words/${currentCard.word_id}/review`, { method: 'POST' });
        reviewBtn.textContent = '✓ Flagged';
        reviewBtn.className = 'w-full mb-3 border border-orange-200 text-orange-400 font-medium py-2 rounded-xl text-sm';
      } catch (err) {
        reviewBtn.disabled = false;
        alert('Could not flag word: ' + err.message);
      }
    };

    $('next-btn').focus();
    await loadStats();
  } catch (err) {
    isSubmitted = false;
    alert('Error: ' + err.message);
  }
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

  // Mobile summary: show selected tags, or "All" if none selected
  const mobileSummary = $('tag-mobile-summary');
  mobileSummary.innerHTML = '';
  if (selectedTags.length === 0) {
    const all = document.createElement('span');
    all.className = 'px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-500 shrink-0';
    all.textContent = 'All';
    mobileSummary.appendChild(all);
  } else {
    for (const tag of selectedTags) {
      const chip = document.createElement('span');
      chip.className = 'px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-600 text-white shrink-0';
      chip.textContent = tag;
      mobileSummary.appendChild(chip);
    }
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
      loadNextCard();
    });
    overlayTagChips.appendChild(pill);
  }
  $('overlay-tags-section').classList.remove('hidden');
}

document.addEventListener('DOMContentLoaded', () => {
  applyModeButtons();
  loadTrainTags();
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
    $('filter-overlay').classList.add('hidden');
    document.body.style.overflow = '';
  }
  $('open-filter-overlay').addEventListener('click', openFilterOverlay);
  $('open-filter-overlay-tags').addEventListener('click', openFilterOverlay);
  $('filter-overlay-close').addEventListener('click', closeFilterOverlay);
  $('filter-overlay-backdrop').addEventListener('click', closeFilterOverlay);
  document.querySelectorAll('.overlay-mode-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      selectedMode = btn.dataset.mode;
      localStorage.setItem('quizMode', selectedMode);
      applyModeButtons();
      closeFilterOverlay();
      loadNextCard();
    });
  });

  loadNextCard();
});
