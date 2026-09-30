// Pinyin listening training state machine

let currentCard = null;
let isSubmitted = false;
let selectedTags = JSON.parse(localStorage.getItem('pinyinTags') || '[]');
let currentAudio = null;
let answeredThisSession = 0;

const STATUS_ICON = {
  correct: 'M5 12.5l4.5 4.5L19 7.5',
  wrong: 'M6 6l12 12M18 6 6 18',
};

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
    renderPinyinBar(stats);
    return stats;
  } catch (e) {
    return null;
  }
}

// renderPinyinBar fills the sticky bar: answers this session against the
// sounds still due, and the due / total counts.
function renderPinyinBar(stats) {
  const p = sessionProgress(answeredThisSession, stats.due_today);
  setText('pinyin-progress-label', t('session.progress', { done: p.done, total: p.total }));
  setText('pinyin-total-label', t('pinyin.dueTotal', { due: stats.due_today, total: stats.total }));
  $('pinyin-progress-bar').style.width = p.pct + '%';
}

async function loadTags() {
  try {
    const tags = await apiFetch('/api/pinyin-quiz/tags');
    const container = $('tag-chips');
    if (!container || !tags.length) return;

    container.querySelectorAll('.tag-btn').forEach(b => b.remove());

    // "All" button
    const allBtn = document.createElement('button');
    allBtn.type = 'button';
    allBtn.className = 'tag-btn ui-chip py-chip';
    allBtn.textContent = t('pinyin.all');
    allBtn.dataset.tag = '';
    container.appendChild(allBtn);

    for (const tag of tags) {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'tag-btn ui-chip py-chip';
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
    btn.setAttribute('aria-pressed', String(active));
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
  const play = $('play-btn');
  play.classList.remove('is-playing');
  void play.offsetWidth;
  play.classList.add('is-playing');

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
    btn.type = 'button';
    btn.className = 'mc-btn py-option';
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

function setPinyinResultHead(correct) {
  const kind = correct ? 'correct' : 'wrong';
  const title = $('result-icon');
  title.textContent = t(correct ? 'pinyin.correct' : 'pinyin.wrong');
  title.className = 'tr-result-title' + (correct ? '' : ' is-wrong');
  const circle = $('result-status-icon');
  circle.className = 'tr-status-icon' + (correct ? '' : ' is-wrong');
  circle.innerHTML = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="${STATUS_ICON[kind]}"/></svg>`;
}

// In multiple-choice mode the answer is a sound id; show its label instead.
function answerLabel(answer) {
  if (currentCard.mode !== 'multiple_choice') return answer;
  const opt = (currentCard.options || []).find(o => String(o.sound_id) === String(answer));
  return opt ? opt.label : answer;
}

function showResult(resp) {
  hide('card-area');
  answeredThisSession++;

  setPinyinResultHead(resp.correct);

  setText('result-correct-answer', resp.correct_answer);
  $('result-play-btn').onclick = () => playPinyinAudio(currentCard.audio_file);

  // Tone variants — let the user listen to every tone of the syllable.
  const tvContainer = $('tone-variants');
  tvContainer.innerHTML = '';
  if (resp.tone_variants && resp.tone_variants.length > 1) {
    for (const v of resp.tone_variants) {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'py-variant' + (v.current ? ' is-current' : '');
      if (v.current) btn.setAttribute('aria-current', 'true');
      btn.innerHTML = `<span class="py-variant-label">${escHtml(v.label)}</span>` +
        (v.tone >= 1 && v.tone <= 5 ? `<span class="py-variant-tone">${escHtml(t('pinyin.tone' + v.tone))}</span>` : '');
      btn.addEventListener('click', () => playPinyinAudio(v.filename));
      tvContainer.appendChild(btn);
    }
    tvContainer.style.gridTemplateColumns = `repeat(${Math.min(resp.tone_variants.length, 5)}, minmax(0, 1fr))`;
    show('tone-variants-wrap');
  } else {
    hide('tone-variants-wrap');
  }

  // Your answer (wrong answers only)
  if (resp.your_answer && !resp.correct) {
    $('result-your-answer').innerHTML = `<span class="py-your-label">${escHtml(t('pinyin.yourAnswerLabel'))}</span><span class="py-your-value">${escHtml(answerLabel(resp.your_answer))}</span>`;
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
    setText('next-due-info', t('pinyin.nextReview', { n: resp.interval_days }));
  } else {
    setText('next-due-info', t('pinyin.dueSoon'));
  }

  // Tier chip; a changed tier gets a ring.
  const tier = TIERS.find(e => e.label === resp.tier);
  const bucket = $('bucket-info');
  if (tier) {
    const changed = !!resp.prev_tier && resp.prev_tier !== resp.tier;
    bucket.className = `tier-chip tier-chip-${tier.label.toLowerCase()}` + (changed ? ' is-changed' : '');
    bucket.innerHTML = `<span class="tier-icon">${tier.icon}</span>${escHtml(t(tier.i18nKey))}`;
  } else {
    bucket.className = 'hidden';
    bucket.innerHTML = '';
  }

  // Attempts
  const acc = resp.total_attempts > 0
    ? Math.round(100 * resp.total_correct / resp.total_attempts)
    : 0;
  setText('attempt-stats', `${resp.total_correct}/${resp.total_attempts} (${acc}%)`);

  show('result-area');
  $('next-btn').focus();
  loadStats();
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
