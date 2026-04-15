// Pinyin listening training state machine

let currentCard = null;
let isSubmitted = false;
let selectedTags = JSON.parse(localStorage.getItem('pinyinTags') || '[]');
let currentAudio = null;

function playPinyinAudio(filename) {
  if (currentAudio) {
    currentAudio.pause();
    currentAudio = null;
  }
  currentAudio = new Audio(`/api/pinyin-quiz/audio/${filename}`);
  currentAudio.play().catch(() => {});
}

async function loadStats() {
  try {
    const params = new URLSearchParams();
    if (selectedTags.length) params.set('tags', selectedTags.join(','));
    const stats = await apiFetch(`/api/pinyin-quiz/stats?${params}`);
    setText('stats-due', stats.due_today);
    setText('stats-total', stats.total);
    return stats;
  } catch (e) {
    return null;
  }
}

async function loadTags() {
  try {
    const tags = await apiFetch('/api/pinyin-quiz/tags');
    const container = $('tag-chips');
    if (!container || !tags.length) return;

    // Remove old tag buttons (keep the label)
    container.querySelectorAll('.tag-btn').forEach(b => b.remove());

    // "All" button
    const allBtn = document.createElement('button');
    allBtn.className = 'tag-btn px-2.5 py-0.5 rounded-full text-xs font-medium transition';
    allBtn.textContent = t('pinyin.all');
    allBtn.dataset.tag = '';
    container.appendChild(allBtn);

    for (const tag of tags) {
      const btn = document.createElement('button');
      btn.className = 'tag-btn px-2.5 py-0.5 rounded-full text-xs font-medium transition';
      btn.textContent = tag;
      btn.dataset.tag = tag;
      container.appendChild(btn);
    }
    applyTagPills();
  } catch (e) {}
}

function applyTagPills() {
  document.querySelectorAll('.tag-btn').forEach(btn => {
    const tag = btn.dataset.tag;
    const active = tag === '' ? selectedTags.length === 0 : selectedTags.includes(tag);
    btn.className = 'tag-btn px-2.5 py-0.5 rounded-full text-xs font-medium transition ' +
      (active ? 'bg-purple-600 text-white' : 'bg-gray-100 text-gray-600 hover:bg-gray-200');
  });
}

async function loadNextCard() {
  isSubmitted = false;
  hide('card-area');
  hide('result-area');
  hide('success-state');
  hide('empty-state');
  hide('error-state');

  const stats = await loadStats();

  const params = new URLSearchParams();
  if (selectedTags.length) params.set('tags', selectedTags.join(','));

  try {
    currentCard = await apiFetch(`/api/pinyin-quiz/next?${params}`);
  } catch (e) {
    if (e.message === 'no pinyin sounds available') {
      if (stats && stats.total === 0) {
        show('empty-state');
      } else {
        show('success-state');
      }
      return;
    }
    setText('error-msg', e.message);
    show('error-state');
    return;
  }

  showCard();
}

function showCard() {
  if (!currentCard) return;

  setText('mode-label', t(currentCard.mode === 'multiple_choice' ? 'pinyin.listenChoose' : 'pinyin.listenType'));

  // Setup play button
  $('play-btn').onclick = () => playPinyinAudio(currentCard.audio_file);

  // Auto-play
  playPinyinAudio(currentCard.audio_file);

  if (currentCard.mode === 'multiple_choice') {
    show('mc-options');
    hide('answer-form');
    renderMCOptions(currentCard.options);
  } else {
    hide('mc-options');
    show('answer-form');
    const input = $('answer-input');
    input.value = '';
    input.focus();
  }

  show('card-area');
}

function renderMCOptions(options) {
  const container = $('mc-options');
  container.innerHTML = '';
  for (const opt of options) {
    const btn = document.createElement('button');
    btn.className = 'mc-btn px-4 py-4 rounded-xl text-lg font-medium border-2 border-gray-200 hover:border-purple-400 hover:bg-purple-50 transition text-gray-800';
    btn.textContent = opt.label;
    btn.dataset.soundId = opt.sound_id;
    btn.addEventListener('click', () => {
      if (isSubmitted) return;
      submitAnswer(String(opt.sound_id));
    });
    container.appendChild(btn);
  }
}

async function submitAnswer(answer) {
  if (isSubmitted) return;
  isSubmitted = true;

  try {
    const resp = await apiFetch('/api/pinyin-quiz/answer', {
      method: 'POST',
      body: JSON.stringify({
        sound_id: currentCard.sound_id,
        answer: answer,
        mode: currentCard.mode,
      }),
    });
    showResult(resp);
  } catch (e) {
    setText('error-msg', e.message);
    show('error-state');
  }
}

function showResult(resp) {
  hide('card-area');

  // Icon
  if (resp.correct) {
    setText('result-icon', t('pinyin.correct'));
    $('result-icon').className = 'text-3xl font-bold mb-4 text-green-600';
  } else {
    setText('result-icon', t('pinyin.wrong'));
    $('result-icon').className = 'text-3xl font-bold mb-4 text-red-500';
  }

  // Correct answer
  setText('result-correct-answer', resp.correct_answer);
  $('result-play-btn').onclick = () => playPinyinAudio(currentCard.audio_file);

  // Tone variants — let user listen to all tones
  const tvContainer = $('tone-variants');
  tvContainer.innerHTML = '';
  if (resp.tone_variants && resp.tone_variants.length > 1) {
    for (const v of resp.tone_variants) {
      const btn = document.createElement('button');
      btn.className = 'px-3 py-1.5 rounded-lg text-sm font-medium transition ' +
        (v.current
          ? 'bg-purple-100 text-purple-700 border-2 border-purple-400'
          : 'bg-gray-100 text-gray-600 border-2 border-transparent hover:bg-gray-200');
      btn.textContent = v.label;
      btn.addEventListener('click', () => playPinyinAudio(v.filename));
      tvContainer.appendChild(btn);
    }
    show('tone-variants');
  } else {
    hide('tone-variants');
  }

  // Your answer (type mode only)
  if (resp.your_answer) {
    $('result-your-answer').innerHTML = `${t('pinyin.yourAnswer')}: <strong>${escHtml(resp.your_answer)}</strong>`;
    show('result-your-answer');
  } else {
    hide('result-your-answer');
  }

  // Confusion info
  if (resp.confused_with) {
    const count = resp.confused_with.count;
    const key = count > 1 ? 'pinyin.confusedWith' : 'pinyin.confusedWithOnce';
    setText('result-confusion', t(key, { label: resp.confused_with.confused_with_label, n: count }));
    show('result-confusion');
  } else {
    hide('result-confusion');
  }

  // Progress info
  if (resp.learning) {
    setText('next-due-info', t('pinyin.learning', { n: resp.graduate_reps }));
  } else if (resp.interval_days > 0) {
    const days = resp.interval_days;
    setText('next-due-info', t('pinyin.nextReview', { n: days }));
  } else {
    setText('next-due-info', t('pinyin.dueSoon'));
  }

  // Tier transition
  if (resp.prev_tier && resp.tier) {
    setText('bucket-info', `${resp.prev_tier} → ${resp.tier}`);
    show('bucket-info');
  } else if (resp.tier) {
    setText('bucket-info', resp.tier);
    show('bucket-info');
  } else {
    hide('bucket-info');
  }

  // Attempts
  const acc = resp.total_attempts > 0
    ? Math.round(100 * resp.total_correct / resp.total_attempts)
    : 0;
  setText('attempt-stats', `${resp.total_correct}/${resp.total_attempts} (${acc}%)`);

  // Highlight MC buttons if still visible
  if (currentCard.mode === 'multiple_choice') {
    document.querySelectorAll('.mc-btn').forEach(btn => {
      const sid = parseInt(btn.dataset.soundId);
      if (sid === currentCard.sound_id) {
        btn.className = 'mc-btn px-4 py-4 rounded-xl text-lg font-medium border-2 border-green-500 bg-green-50 text-green-700';
      } else if (!resp.correct && String(sid) === resp.your_answer) {
        btn.className = 'mc-btn px-4 py-4 rounded-xl text-lg font-medium border-2 border-red-400 bg-red-50 text-red-600';
      }
    });
  }

  show('result-area');
  $('next-btn').focus();
}

function formatDuration(ms) {
  const mins = Math.round(ms / 60000);
  if (mins < 60) return `${mins} min`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours} hour${hours > 1 ? 's' : ''}`;
  const days = Math.round(hours / 24);
  return `${days} day${days > 1 ? 's' : ''}`;
}

// Initialization
document.addEventListener('DOMContentLoaded', () => {
  loadTags();
  loadNextCard();

  // Tag button clicks
  $('tag-chips').addEventListener('click', (e) => {
    const btn = e.target.closest('.tag-btn');
    if (!btn) return;
    const tag = btn.dataset.tag;
    if (tag === '') {
      selectedTags = [];
    } else {
      const idx = selectedTags.indexOf(tag);
      if (idx >= 0) {
        selectedTags.splice(idx, 1);
      } else {
        selectedTags.push(tag);
      }
    }
    localStorage.setItem('pinyinTags', JSON.stringify(selectedTags));
    applyTagPills();
    loadNextCard();
  });

  // Answer form submit (type mode)
  $('answer-form').addEventListener('submit', (e) => {
    e.preventDefault();
    const answer = $('answer-input').value.trim();
    if (!answer) return;
    submitAnswer(answer);
  });

  // Next button
  $('next-btn').addEventListener('click', () => loadNextCard());

  // Keyboard shortcut: Enter on result screen goes to next
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !$('result-area').classList.contains('hidden') && document.activeElement !== $('answer-input')) {
      e.preventDefault();
      loadNextCard();
    }
  });
});
