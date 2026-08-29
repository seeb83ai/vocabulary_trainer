// train-matchgame.js — gamification match game

// ── Match Game ───────────────────────────────────────────────────────────────

async function _maybeShowMatchGame() {
  if (!_gamificationEnabled) return;
  if (Date.now() - _lastGameShownAt < _gamificationFrequencyMs) return;
  let data;
  try {
    data = await apiFetch('/api/quiz/match-game');
  } catch {
    return;
  }
  if (!data?.words || data.words.length < 2) return;
  _lastGameShownAt = Date.now();
  await showMatchGame(data.words);
}

// Decides the outcome of matching a left word (lIdx) to a right box whose true
// owner is rightIdx. A right box may legitimately be claimed by a non-owning
// word when they share a translation text — but only once its true owner is
// already matched elsewhere. Otherwise the claim is "blocked": accepting it
// would visually strand the true owner's only matching box (issue #215).
function matchGameOutcome(rightIdx, lIdx, rightText, leftTransls, matchedLeftIdxs) {
  if (rightIdx === lIdx) return 'correct';
  if (leftTransls.includes(rightText)) {
    return matchedLeftIdxs.has(rightIdx) ? 'correct' : 'blocked';
  }
  return 'wrong';
}

// showMatchGame accepts the flat words array returned by GET /api/quiz/match-game.
// Each word: { zh_word_id, zh_text, pinyin, translations }
// Left column shows Chinese words; right column shows one translation each, shuffled.
function showMatchGame(words) {
  return new Promise(resolve => {
    const overlay = document.createElement('div');
    overlay.id = 'match-game-overlay';
    overlay.className = 'fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50';

    // Build left items (Chinese) and right items (first EN translation), indexed by position.
    // kind/character distinguish a component tile from a word tile (issue #280)
    // so the match-answer POST updates the right progress table.
    const leftItems = words.map((w, i) => ({
      idx: i,
      kind: w.kind,
      zh_word_id: w.zh_word_id,
      character: w.character,
      text: w.zh_text,
      pinyin: w.pinyin,
    }));
    const matchAnswerBody = (item, correct) => item.kind === 'component'
      ? { kind: 'component', character: item.character, correct }
      : { zh_word_id: item.zh_word_id, correct };
    const rightItems = words.map((w, i) => ({
      idx: i,   // idx matches leftItems position — used to identify the correct pair
      text: Object.values(w.translations || {})[0]?.[0] || w.zh_text,
    }));
    const shuffledRight = [...rightItems].sort(() => Math.random() - 0.5);

    let selectedLeft = null;
    const matched = new Set();

    function renderBox(text, sub) {
      const div = document.createElement('div');
      div.className = 'border-2 border-gray-300 rounded-xl p-3 cursor-pointer select-none transition text-center min-h-[72px] flex flex-col items-center justify-center';
      const t = document.createElement('div');
      t.className = 'font-semibold text-gray-800';
      t.textContent = text;
      div.appendChild(t);
      if (sub) {
        const s = document.createElement('div');
        s.className = 'match-pinyin-sub text-xs text-gray-500 mt-1';
        s.textContent = sub;
        div.appendChild(s);
      }
      return div;
    }

    // reveals a left box's pinyin hint after a correct match, when the
    // match_game_pinyin_reveal setting is "after_correct" (issue #375).
    function revealPinyin(box, pinyin) {
      if (!pinyin || box.querySelector('.match-pinyin-sub')) return;
      const s = document.createElement('div');
      s.className = 'match-pinyin-sub text-xs text-gray-500 mt-1';
      s.textContent = pinyin;
      box.appendChild(s);
    }

    const modal = document.createElement('div');
    modal.className = 'bg-white rounded-2xl shadow-xl p-6 w-full max-w-lg mx-4';
    modal.innerHTML = '<h2 class="text-lg font-semibold text-gray-800 mb-4 text-center">Match the pairs</h2>';

    const grid = document.createElement('div');
    grid.className = 'grid grid-cols-2 gap-3 mb-4';

    const leftBoxes = leftItems.map(item =>
      renderBox(item.text, _matchGamePinyinReveal === 'always' ? item.pinyin : null));
    const rightBoxes = shuffledRight.map(item => renderBox(item.text));

    leftBoxes.forEach((box, lIdx) => {
      box.addEventListener('click', () => {
        if (matched.has(lIdx)) return;
        leftBoxes.forEach(b => b.classList.remove('border-blue-500', 'bg-blue-50'));
        selectedLeft = lIdx;
        box.classList.add('border-blue-500', 'bg-blue-50');
      });
    });

    rightBoxes.forEach((box, rIdx) => {
      box.addEventListener('click', async () => {
        if (selectedLeft === null) return;
        const lIdx = selectedLeft;
        if (matched.has(lIdx)) return;
        const rightIdx = shuffledRight[rIdx].idx; // which word this translation belongs to
        const rightText = shuffledRight[rIdx].text;
        const leftTransls = Object.values(words[lIdx].translations || {}).flat();
        const outcome = matchGameOutcome(rightIdx, lIdx, rightText, leftTransls, matched);

        if (outcome === 'correct') {
          // Correct match
          leftBoxes[lIdx].classList.remove('border-blue-500', 'bg-blue-50');
          leftBoxes[lIdx].classList.add('border-green-500', 'bg-green-50', 'cursor-default');
          box.classList.add('border-green-500', 'bg-green-50', 'cursor-default');
          matched.add(lIdx);
          selectedLeft = null;
          if (_matchGamePinyinReveal === 'after_correct') {
            revealPinyin(leftBoxes[lIdx], leftItems[lIdx].pinyin);
          }
          try {
            await apiFetch('/api/quiz/match-answer', {
              method: 'POST',
              body: JSON.stringify(matchAnswerBody(leftItems[lIdx], true)),
            });
          } catch { /* best effort */ }
          if (matched.size === words.length) {
            setTimeout(() => { overlay.remove(); resolve(); }, 600);
          }
        } else if (outcome === 'blocked') {
          // Right box is still needed as its true owner's only match — flash
          // yellow (not a mistake) and reset without recording an SM2 answer.
          leftBoxes[lIdx].classList.add('border-yellow-500', 'bg-yellow-50');
          box.classList.add('border-yellow-500', 'bg-yellow-50');
          setTimeout(() => {
            leftBoxes[lIdx].classList.remove('border-yellow-500', 'bg-yellow-50', 'border-blue-500', 'bg-blue-50');
            box.classList.remove('border-yellow-500', 'bg-yellow-50');
            selectedLeft = null;
          }, 800);
        } else {
          // Wrong match — flash red both boxes, then reset
          leftBoxes[lIdx].classList.add('border-red-500', 'bg-red-50');
          box.classList.add('border-red-500', 'bg-red-50');
          setTimeout(() => {
            leftBoxes[lIdx].classList.remove('border-red-500', 'bg-red-50', 'border-blue-500', 'bg-blue-50');
            box.classList.remove('border-red-500', 'bg-red-50');
            selectedLeft = null;
          }, 800);
          try {
            await apiFetch('/api/quiz/match-answer', {
              method: 'POST',
              body: JSON.stringify(matchAnswerBody(leftItems[lIdx], false)),
            });
            await apiFetch('/api/quiz/match-answer', {
              method: 'POST',
              body: JSON.stringify(matchAnswerBody(leftItems[rightIdx], false)),
            });
          } catch { /* best effort */ }
        }
      });
    });

    const leftCol = document.createElement('div');
    leftCol.className = 'space-y-3';
    leftBoxes.forEach(b => leftCol.appendChild(b));

    const rightCol = document.createElement('div');
    rightCol.className = 'space-y-3';
    rightBoxes.forEach(b => rightCol.appendChild(b));

    grid.appendChild(leftCol);
    grid.appendChild(rightCol);
    modal.appendChild(grid);

    const skipBtn = document.createElement('button');
    skipBtn.textContent = 'Skip game';
    skipBtn.className = 'w-full text-sm text-gray-400 hover:text-gray-600 mt-2';
    skipBtn.addEventListener('click', () => { overlay.remove(); resolve(); });
    modal.appendChild(skipBtn);

    overlay.appendChild(modal);
    document.body.appendChild(overlay);
  });
}
