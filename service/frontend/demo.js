// Landing-page demo quiz. Stateless: cards come from /api/demo/cards and
// answers are checked server-side with the same matching logic as the real
// quiz. Kept in its own file so the strict CSP (script-src 'self') holds.
//
// The correct/wrong screen mirrors the real quiz's result screen
// (#result-icon / #word-breakdown in train.html, built by renderWordAnswerResult
// in train.js): a big ✓/✗ headline, a red "your answer" box on a miss, and a
// green word box with pinyin + accepted translations.
(function () {
  let cards = [];
  let idx = 0;
  let score = 0;
  let answered = false;

  const el = id => document.getElementById(id);
  const escHtml = s => String(s).replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));

  function renderCard() {
    el('demo-zh').textContent = cards[idx].zh;
    el('demo-pinyin').textContent = cards[idx].pinyin;
    el('demo-progress').textContent = `Card ${idx + 1} of ${cards.length}`;
    el('demo-input').value = '';
    el('demo-input').disabled = false;
    el('demo-submit').disabled = false;
    el('demo-question').classList.remove('hidden');
    el('demo-form').classList.remove('hidden');
    el('demo-feedback').classList.add('hidden');
    el('demo-next').classList.add('hidden');
    answered = false;
  }

  function showResult(correct, answer, translations) {
    el('demo-question').classList.add('hidden');
    el('demo-form').classList.add('hidden');

    const icon = el('demo-icon');
    const card = cards[idx];
    const pinyin = card.pinyin ? `<span class="text-gray-400 text-base ml-2">${escHtml(card.pinyin)}</span>` : '';
    const wordBox = `
      <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
        <div class="text-xs text-green-500 uppercase tracking-wide mb-1">Word</div>
        <div class="text-3xl font-bold text-gray-800">${escHtml(card.zh)}${pinyin}</div>
        <div class="text-gray-600 text-sm mt-0.5">${translations.map(escHtml).join(' · ')}</div>
      </div>`;

    if (correct) {
      icon.textContent = '✓ Correct!';
      icon.className = 'text-3xl font-bold text-green-600 mb-4';
      el('demo-breakdown').innerHTML = wordBox;
    } else {
      icon.textContent = '✗ Not quite';
      icon.className = 'text-3xl font-bold text-red-600 mb-4';
      const yourAnswerBox = `
        <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
          <div class="text-xs text-red-400 uppercase tracking-wide mb-1">Your answer</div>
          <div class="text-sm font-medium text-red-700">${escHtml(answer)}</div>
        </div>`;
      el('demo-breakdown').innerHTML = yourAnswerBox + wordBox;
    }

    el('demo-feedback').classList.remove('hidden');
    el('demo-next').textContent = idx === cards.length - 1 ? 'Finish' : 'Next word';
    el('demo-next').classList.remove('hidden');
    el('demo-next').focus();
  }

  async function submit(e) {
    e.preventDefault();
    if (answered) return;
    const answer = el('demo-input').value.trim();
    if (!answer) return;
    try {
      const res = await fetch('/api/demo/answer', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ card_id: cards[idx].id, answer }),
      });
      if (!res.ok) return;
      const data = await res.json();
      answered = true;
      if (data.correct) score++;
      showResult(data.correct, answer, data.translations || []);
    } catch {
      // Network error: leave the form usable so the visitor can retry.
    }
  }

  function next() {
    if (idx === cards.length - 1) {
      el('demo-card').classList.add('hidden');
      el('demo-score').textContent = `${score} of ${cards.length}`;
      el('demo-done').classList.remove('hidden');
      return;
    }
    idx++;
    renderCard();
    el('demo-input').focus();
  }

  document.addEventListener('DOMContentLoaded', async () => {
    try {
      const res = await fetch('/api/demo/cards');
      if (!res.ok) return;
      cards = (await res.json()).cards || [];
    } catch {
      return;
    }
    if (!cards.length) return;
    el('demo-form').addEventListener('submit', submit);
    el('demo-next').addEventListener('click', next);
    renderCard();
  });
})();
