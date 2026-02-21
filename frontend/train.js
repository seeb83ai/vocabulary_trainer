// Training page logic

let currentCard = null;
let isSubmitted = false;

async function loadStats() {
  try {
    const stats = await apiFetch('/api/quiz/stats');
    setText('stats-due', stats.due_today);
    setText('stats-total', stats.total);
  } catch (_) {}
}

async function loadNextCard() {
  isSubmitted = false;
  hide('result-area');
  hide('empty-state');
  hide('error-state');
  $('answer-input').value = '';

  try {
    currentCard = await apiFetch('/api/quiz/next');
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

  if (currentCard.pinyin) {
    setText('pinyin-hint', currentCard.pinyin);
    show('pinyin-hint');
  } else {
    hide('pinyin-hint');
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

    setText('correct-answers', result.correct_answers.join(' / '));

    // On wrong answers show what was typed vs the correct answer
    const breakdown = $('word-breakdown');
    if (!result.correct) {
      const pinyin = result.pinyin ? `<span class="text-gray-400 text-base ml-2">${escHtml(result.pinyin)}</span>` : '';
      breakdown.innerHTML = `
        <div class="mt-4 space-y-2 text-left">
          <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
            <div class="text-xs text-red-400 uppercase tracking-wide mb-1">Your answer</div>
            <div class="text-lg font-medium text-red-700">${escHtml(answer)}</div>
          </div>
          <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
            <div class="text-xs text-green-500 uppercase tracking-wide mb-1">Correct</div>
            <div class="text-xl font-bold text-gray-800">${escHtml(result.zh_text)}${pinyin}</div>
            <div class="text-gray-600 text-sm mt-0.5">${result.en_texts.map(escHtml).join(' · ')}</div>
          </div>
        </div>`;
      show('word-breakdown');
    } else {
      breakdown.innerHTML = '';
      hide('word-breakdown');
    }

    setText('next-due-info', `Next review in ${result.interval_days} day(s)`);
    setText('attempt-stats',
      `Correct: ${result.total_correct} / ${result.total_attempts}`);

    $('next-btn').focus();
    await loadStats();
  } catch (err) {
    isSubmitted = false;
    alert('Error: ' + err.message);
  }
}

document.addEventListener('DOMContentLoaded', () => {
  $('answer-form').addEventListener('submit', submitAnswer);
  $('next-btn').addEventListener('click', loadNextCard);
  loadNextCard();
});
