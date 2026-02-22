// Training page logic

let currentCard = null;
let isSubmitted = false;
let selectedMode = localStorage.getItem('quizMode') || 'random';

function applyModeButtons() {
  document.querySelectorAll('.mode-btn').forEach(btn => {
    const active = btn.dataset.mode === selectedMode;
    btn.className = active
      ? 'mode-btn px-3 py-1 rounded-full text-sm font-medium transition bg-blue-600 text-white'
      : 'mode-btn px-3 py-1 rounded-full text-sm font-medium transition bg-gray-100 text-gray-600 hover:bg-gray-200';
  });
}

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
  hide('add-translation-btn');
  hide('result-play-btn');
  $('answer-input').value = '';

  try {
    const url = selectedMode === 'random' ? '/api/quiz/next' : `/api/quiz/next?mode=${selectedMode}`;
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

    // On wrong answers show what was typed vs the correct answer (play button inside breakdown)
    const breakdown = $('word-breakdown');
    if (!result.correct) {
      hide('result-play-btn');
      const pinyin = result.pinyin ? `<span class="text-gray-400 text-base ml-2">${escHtml(result.pinyin)}</span>` : '';
      breakdown.innerHTML = `
        <div class="mt-4 space-y-2 text-left">
          <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
            <div class="text-xs text-red-400 uppercase tracking-wide mb-1">Your answer</div>
            <div class="text-lg font-medium text-red-700">${escHtml(answer)}</div>
          </div>
          <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
            <div class="text-xs text-green-500 uppercase tracking-wide mb-1">Correct</div>
            <div class="flex items-center gap-2">
              <div class="text-xl font-bold text-gray-800">${escHtml(result.zh_text)}${pinyin}</div>
              <button class="btn-breakdown-play text-xl text-gray-400 hover:text-blue-500 transition leading-none shrink-0" title="Read aloud">🔊</button>
            </div>
            <div class="text-gray-600 text-sm mt-0.5">${result.en_texts.map(escHtml).join(' · ')}</div>
          </div>
        </div>`;
      breakdown.querySelector('.btn-breakdown-play').addEventListener('click', () => playAudio(currentCard.word_id, result.zh_text));
      show('word-breakdown');

      // "Add as correct answer" button
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
      breakdown.innerHTML = '';
      hide('word-breakdown');
      hide('add-translation-btn');
      const playBtn = $('result-play-btn');
      playBtn.onclick = () => playAudio(currentCard.word_id, result.zh_text);
      show('result-play-btn');
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
  applyModeButtons();
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
  loadNextCard();
});
