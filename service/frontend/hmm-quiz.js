// HMM mnemonic quiz state machine

let currentCard = null;
let isSubmitted = false;

const TYPE_COLORS = {
  actor:     { badge: 'bg-purple-100 text-purple-700', ring: 'focus:ring-purple-500' },
  location:  { badge: 'bg-blue-100 text-blue-700',    ring: 'focus:ring-blue-500' },
  tone_room: { badge: 'bg-amber-100 text-amber-700',  ring: 'focus:ring-amber-500' },
  prop:      { badge: 'bg-emerald-100 text-emerald-700', ring: 'focus:ring-emerald-500' },
};

const TYPE_LABELS = {
  actor:     'Actor',
  location:  'Location',
  tone_room: 'Tone Room',
  prop:      'Prop',
};

function formatPrompt(card) {
  if (card.entity_type === 'tone_room') {
    return 'Tone ' + card.entity_key;
  }
  if (card.entity_key === 'null') {
    return card.entity_type === 'actor' ? '(no initial)' : '(no final)';
  }
  return card.entity_key;
}

function typeBadgeHTML(entityType) {
  const colors = TYPE_COLORS[entityType] || { badge: 'bg-gray-100 text-gray-700' };
  const label = TYPE_LABELS[entityType] || entityType;
  return { classes: colors.badge, label };
}

async function loadStats() {
  try {
    const stats = await apiFetch('/api/hmm-quiz/stats');
    setText('stats-due', stats.due_today);
    setText('stats-total', stats.total);
    return stats;
  } catch (e) {
    return null;
  }
}

async function loadNextCard() {
  isSubmitted = false;
  hide('card-area');
  hide('result-area');
  hide('success-state');
  hide('empty-state');
  hide('error-state');

  const stats = await loadStats();

  try {
    currentCard = await apiFetch('/api/hmm-quiz/next');
  } catch (e) {
    if (e.message === 'no HMM entries available') {
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

  const { classes, label } = typeBadgeHTML(currentCard.entity_type);
  const badge = $('type-badge');
  badge.className = 'inline-block px-3 py-1 rounded-full text-xs font-bold uppercase tracking-wider mb-6 ' + classes;
  badge.textContent = label;

  setText('prompt-text', formatPrompt(currentCard));

  // Show actor hint if present
  const hintEl = $('actor-hint');
  if (currentCard.entity_type === 'actor' && (currentCard.category || currentCard.hint)) {
    const parts = [];
    if (currentCard.category) parts.push(currentCard.category);
    if (currentCard.hint) parts.push(currentCard.hint);
    hintEl.textContent = parts.join(' · ');
    show('actor-hint');
  } else {
    hide('actor-hint');
  }

  const input = $('answer-input');
  input.value = '';

  show('card-area');
  input.focus();
}

async function submitAnswer(answer) {
  if (isSubmitted) return;
  isSubmitted = true;

  try {
    const resp = await apiFetch('/api/hmm-quiz/answer', {
      method: 'POST',
      body: JSON.stringify({
        entity_type: currentCard.entity_type,
        entity_key: currentCard.entity_key,
        answer: answer,
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

  if (resp.correct) {
    setText('result-icon', t('pinyin.correct'));
    $('result-icon').className = 'text-3xl font-bold mb-4 text-green-600';
  } else {
    setText('result-icon', t('pinyin.wrong'));
    $('result-icon').className = 'text-3xl font-bold mb-4 text-red-500';
  }

  // Entity context
  const { classes, label } = typeBadgeHTML(currentCard.entity_type);
  const resultBadge = $('result-type-badge');
  resultBadge.className = 'inline-block px-3 py-1 rounded-full text-xs font-bold uppercase tracking-wider mb-2 ' + classes;
  resultBadge.textContent = label;
  setText('result-prompt', formatPrompt(currentCard));

  setText('result-correct-answer', resp.correct_answer);

  if (resp.your_answer) {
    $('result-your-answer').innerHTML = `${t('pinyin.yourAnswer', { answer: escHtml(resp.your_answer) })}`;
    show('result-your-answer');
  } else {
    hide('result-your-answer');
  }

  // Progress info
  if (resp.learning) {
    setText('next-due-info', t('pinyin.learning', { n: 3 }));
  } else if (resp.interval_days > 0) {
    setText('next-due-info', t('pinyin.nextReview', { n: resp.interval_days }));
  } else {
    setText('next-due-info', t('pinyin.dueSoon'));
  }

  // Tier
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

  show('result-area');
  $('next-btn').focus();
}

// Initialization
document.addEventListener('DOMContentLoaded', () => {
  loadNextCard();

  $('answer-form').addEventListener('submit', (e) => {
    e.preventDefault();
    const answer = $('answer-input').value.trim();
    if (!answer) return;
    submitAnswer(answer);
  });

  $('next-btn').addEventListener('click', () => loadNextCard());

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !$('result-area').classList.contains('hidden') && document.activeElement !== $('answer-input')) {
      e.preventDefault();
      loadNextCard();
    }
  });
});
