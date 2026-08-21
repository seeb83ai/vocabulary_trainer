// Landing-page demo quiz. Stateless: cards come from /api/demo/cards and
// answers are checked server-side with the same matching logic as the real
// quiz. Kept in its own file so the strict CSP (script-src 'self') holds.
(function () {
  let cards = [];
  let idx = 0;
  let score = 0;
  let answered = false;

  const el = id => document.getElementById(id);

  function renderCard() {
    el('demo-zh').textContent = cards[idx].zh;
    el('demo-pinyin').textContent = cards[idx].pinyin;
    el('demo-progress').textContent = `Card ${idx + 1} of ${cards.length}`;
    el('demo-input').value = '';
    el('demo-input').disabled = false;
    el('demo-submit').disabled = false;
    el('demo-feedback').classList.add('hidden');
    el('demo-next').classList.add('hidden');
    answered = false;
  }

  function showFeedback(correct, translations) {
    const box = el('demo-feedback');
    box.classList.remove('hidden', 'bg-green-50', 'text-green-800', 'bg-red-50', 'text-red-800');
    if (correct) {
      box.classList.add('bg-green-50', 'text-green-800');
      box.textContent = '✓ Correct!';
    } else {
      box.classList.add('bg-red-50', 'text-red-800');
      box.textContent = '✗ Not quite — accepted: ' + translations.join(', ');
    }
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
      el('demo-input').disabled = true;
      el('demo-submit').disabled = true;
      showFeedback(data.correct, data.translations || []);
      el('demo-next').textContent = idx === cards.length - 1 ? 'Finish' : 'Next word';
      el('demo-next').classList.remove('hidden');
      el('demo-next').focus();
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
