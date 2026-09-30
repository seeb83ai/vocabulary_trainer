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
// A box that shows exactly the same text as the left word's own box is
// interchangeable with it: "swap" — correct, and the two boxes trade owners
// so the other word can still take the left word's own box (issue #473).
function matchGameOutcome(rightIdx, lIdx, rightText, leftShownText, leftTransls, matchedLeftIdxs) {
  if (rightIdx === lIdx) return 'correct';
  if (rightText === leftShownText) return 'swap';
  if (leftTransls.includes(rightText)) {
    return matchedLeftIdxs.has(rightIdx) ? 'correct' : 'blocked';
  }
  return 'wrong';
}

// A dictionary-derived translation string can bundle many senses plus German
// example sentences into one semicolon-separated blob (e.g. HanDeDict
// entries: "nah (Adj); in der Nähe (S); Bsp.: 附近 附近 -- ..."), which used to
// be dumped whole into a match-game box. Keep only the first meaning, or the
// first two when both are short enough to fit comfortably (issue #428).
// isNoise (from train-answer.js) drops "Bsp.:"/"CL:"/"ZEW:" annotation
// segments, which aren't real meanings.
const MATCH_GAME_SHORT_MEANING_MAX_CHARS = 20;

function shortenMatchGameTranslation(text) {
  const parts = text.split(';').map(s => s.trim()).filter(s => s && !isNoise(s));
  if (parts.length === 0) return text.trim();
  if (parts.length === 1) return parts[0];
  if (parts[0].length <= MATCH_GAME_SHORT_MEANING_MAX_CHARS &&
      parts[1].length <= MATCH_GAME_SHORT_MEANING_MAX_CHARS) {
    return `${parts[0]}; ${parts[1]}`;
  }
  return parts[0];
}

// Picks the translation text to show on a word's right-column box. Training
// already hides CL:/Bsp.:/ZEW: example/measure-word entries via isNoise
// (train-card.js) — the match game used to skip that filter entirely and
// could surface a raw example sentence as the box's only content (issue
// #429). Walks languages in the order they appear on the word and returns
// the first non-noise translation found (shortened per issue #428); falls
// back to fallbackText only when every translation across every language is
// noise.
function pickMatchGameTranslationText(translations, fallbackText) {
  for (const texts of Object.values(translations || {})) {
    const clean = (texts || []).filter(t => !isNoise(t));
    if (clean.length > 0) return shortenMatchGameTranslation(clean[0]);
  }
  return fallbackText;
}

// showMatchGame accepts the flat words array returned by GET /api/quiz/match-game.
// Each word: { zh_word_id, zh_text, pinyin, translations }
// It renders inline in the Train card column (not as a modal): left column
// Chinese tiles, right column one translation each, shuffled. Either column
// can be tapped first. Resolves when the round is finished or skipped.
function showMatchGame(words) {
  return new Promise(resolve => {
    const host = document.getElementById('train-container') || document.body;
    const hiddenSiblings = [];
    for (const el of Array.from(host.children)) {
      if (!el.classList.contains('hidden')) {
        el.classList.add('hidden');
        hiddenSiblings.push(el);
      }
    }
    const overlay = document.createElement('div');
    overlay.id = 'match-game-overlay';
    overlay.className = 'mg-card';

    // kind/character distinguish a component tile from a word tile (issue #280)
    // so the match-answer POST updates the right progress table.
    const leftItems = words.map((w, i) => ({
      idx: i,
      kind: w.kind,
      zh_word_id: w.zh_word_id,
      character: w.character,
      text: w.zh_text,
      pinyin: w.pinyin,
      hidePinyin: !!w.hide_pinyin,
    }));
    const matchAnswerBody = (item, correct) => item.kind === 'component'
      ? { kind: 'component', character: item.character, correct }
      : { zh_word_id: item.zh_word_id, correct };
    const rightItems = words.map((w, i) => ({
      idx: i,   // idx matches leftItems position — used to identify the correct pair
      text: pickMatchGameTranslationText(w.translations, w.zh_text),
    }));
    const shuffledRight = [...rightItems].sort(() => Math.random() - 0.5);

    let selected = null; // { side: 'l' | 'r', i }
    let busy = false;
    let mistakes = 0;
    const matched = new Set();

    function finish() {
      overlay.remove();
      hiddenSiblings.forEach(el => el.classList.remove('hidden'));
      resolve();
    }

    function renderBox(text, sub, zh) {
      const div = document.createElement('div');
      div.className = 'mg-tile' + (zh ? ' mg-zh' : '');
      div.setAttribute('role', 'button');
      div.tabIndex = 0;
      const main = document.createElement('div');
      main.className = 'mg-main' + (zh ? ' font-hanzi' : '');
      main.textContent = text;
      div.appendChild(main);
      if (sub) {
        const s = document.createElement('div');
        s.className = 'match-pinyin-sub';
        s.textContent = sub;
        div.appendChild(s);
      }
      div.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); div.click(); } });
      return div;
    }

    // reveals a left box's pinyin hint once that pair has been attempted,
    // per the match_game_pinyin_reveal setting (issue #375): "always" reveals
    // it after any attempt (correct, wrong, or blocked); "after_correct"
    // only once the attempt was correct; "off" never reveals it.
    function revealPinyin(box, pinyin) {
      if (!pinyin || box.querySelector('.match-pinyin-sub')) return;
      const s = document.createElement('div');
      s.className = 'match-pinyin-sub';
      s.textContent = pinyin;
      box.appendChild(s);
    }

    function maybeRevealPinyin(box, item, outcome) {
      if (_matchGamePinyinReveal === 'off') return;
      if (_matchGamePinyinReveal === 'after_correct' && outcome !== 'correct') return;
      revealPinyin(box, item.pinyin);
    }

    overlay.innerHTML = `
      <div class="mg-head"><span class="mg-eyebrow">${escHtml(t('match.eyebrow'))}</span><span class="mg-progress tr-muted-sm"></span></div>
      <h2 class="mg-title">${escHtml(t('match.title'))}</h2>
      <p class="tr-sub" style="margin:0 0 16px">${escHtml(t('match.sub'))}</p>
      <div class="mg-segs" style="grid-template-columns:repeat(${words.length},1fr)">${words.map(() => '<span class="mg-seg"></span>').join('')}</div>`;

    const grid = document.createElement('div');
    grid.className = 'mg-grid grid';

    // A tile shows its pinyin up front unless the server flagged it
    // hide_pinyin (issue #349: word tier at/above the configured threshold).
    // Component tiles are never flagged and always show pinyin from the
    // start. A flagged word tile stays hidden until its pair is attempted,
    // then reveals per match_game_pinyin_reveal (issue #375).
    const leftBoxes = leftItems.map(item =>
      renderBox(item.text, item.hidePinyin ? null : item.pinyin, true));
    const rightBoxes = shuffledRight.map(item => renderBox(item.text, null, false));

    const foot = document.createElement('div');
    foot.className = 'mg-foot';
    foot.innerHTML = `<span class="mg-mistakes"></span>`;
    const skipBtn = document.createElement('button');
    skipBtn.type = 'button';
    skipBtn.className = 'tr-text-btn';
    skipBtn.textContent = t('match.skip');
    skipBtn.addEventListener('click', finish);
    foot.appendChild(skipBtn);

    function updateProgress() {
      overlay.querySelector('.mg-progress').textContent = t('match.progress', { n: matched.size, total: words.length });
      overlay.querySelectorAll('.mg-seg').forEach((seg, i) => seg.classList.toggle('is-on', i < matched.size));
      const m = foot.querySelector('.mg-mistakes');
      m.textContent = mistakes ? t('match.mistakes', { n: mistakes }) : t('match.noMistakes');
      m.classList.toggle('has-mistakes', mistakes > 0);
    }

    function clearSelection() {
      [...leftBoxes, ...rightBoxes].forEach(b => b.classList.remove('is-selected'));
      selected = null;
    }

    function showDone() {
      overlay.innerHTML = `
        <div class="tr-center">
          <div class="tr-tile tr-tile-violet font-hanzi" aria-hidden="true">对</div>
          <h2 class="tr-h1">${escHtml(t('match.doneTitle'))}</h2>
          <p class="tr-sub">${escHtml(t('match.doneStats', { pairs: words.length, mistakes }))}</p>
          ${_matchGameSm2Update !== 'never' ? `<p class="tr-muted-sm" style="margin:10px auto 0;max-width:36ch">${escHtml(t('match.doneNote'))}</p>` : ''}
          <button id="match-continue-btn" type="button" class="ui-btn ui-btn-primary tr-mt-24">${escHtml(t('match.continue'))}</button>
        </div>`;
      const btn = overlay.querySelector('#match-continue-btn');
      btn.addEventListener('click', finish);
      btn.focus();
    }

    async function attempt(lIdx, rIdx) {
      const box = rightBoxes[rIdx];
      if (matched.has(lIdx)) return;
      let rightIdx = shuffledRight[rIdx].idx; // which word this translation belongs to
      const rightText = shuffledRight[rIdx].text;
      // Filtered/shortened the same way as the displayed rightText (skip
      // noise entries, issue #429; collapse to the first short meaning(s),
      // issue #428) so shared-translation detection (matchGameOutcome's
      // "blocked" case) keeps comparing like with like.
      const leftTransls = Object.values(words[lIdx].translations || {}).flat()
        .filter(t => !isNoise(t))
        .map(shortenMatchGameTranslation);
      let outcome = matchGameOutcome(rightIdx, lIdx, rightText, rightItems[lIdx].text, leftTransls, matched);
      if (outcome === 'swap') {
        const ownBox = shuffledRight.find(item => item.idx === lIdx);
        ownBox.idx = rightIdx;
        shuffledRight[rIdx].idx = lIdx;
        rightIdx = lIdx;
        outcome = 'correct';
      }
      maybeRevealPinyin(leftBoxes[lIdx], leftItems[lIdx], outcome);
      clearSelection();

      if (outcome === 'correct') {
        leftBoxes[lIdx].classList.add('is-matched');
        box.classList.add('is-matched');
        matched.add(lIdx);
        updateProgress();
        try {
          await apiFetch('/api/quiz/match-answer', {
            method: 'POST',
            body: JSON.stringify(matchAnswerBody(leftItems[lIdx], true)),
          });
        } catch { /* best effort */ }
        if (matched.size === words.length) setTimeout(showDone, 500);
      } else if (outcome === 'blocked') {
        // Right box is still needed as its true owner's only match — flash
        // amber (not a mistake) and reset without recording an SM2 answer.
        busy = true;
        leftBoxes[lIdx].classList.add('is-blocked');
        box.classList.add('is-blocked');
        setTimeout(() => {
          leftBoxes[lIdx].classList.remove('is-blocked');
          box.classList.remove('is-blocked');
          busy = false;
        }, 750);
      } else {
        // Wrong match — flash both tiles red, then reset
        busy = true;
        mistakes++;
        updateProgress();
        leftBoxes[lIdx].classList.add('is-wrong');
        box.classList.add('is-wrong');
        setTimeout(() => {
          leftBoxes[lIdx].classList.remove('is-wrong');
          box.classList.remove('is-wrong');
          busy = false;
        }, 750);
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
    }

    function pick(side, i) {
      if (busy) return;
      if (side === 'l' && matched.has(i)) return;
      if (side === 'r' && rightBoxes[i].classList.contains('is-matched')) return;
      if (selected && selected.side !== side) {
        const lIdx = side === 'l' ? i : selected.i;
        const rIdx = side === 'r' ? i : selected.i;
        attempt(lIdx, rIdx);
        return;
      }
      clearSelection();
      selected = { side, i };
      (side === 'l' ? leftBoxes : rightBoxes)[i].classList.add('is-selected');
    }

    leftBoxes.forEach((box, i) => box.addEventListener('click', () => pick('l', i)));
    rightBoxes.forEach((box, i) => box.addEventListener('click', () => pick('r', i)));

    const leftCol = document.createElement('div');
    leftCol.className = 'mg-col';
    leftBoxes.forEach(b => leftCol.appendChild(b));
    const rightCol = document.createElement('div');
    rightCol.className = 'mg-col';
    rightBoxes.forEach(b => rightCol.appendChild(b));
    grid.appendChild(leftCol);
    grid.appendChild(rightCol);
    overlay.appendChild(grid);
    overlay.appendChild(foot);
    updateProgress();

    host.prepend(overlay);
    overlay.scrollIntoView({ behavior: 'auto', block: 'start' });
  });
}
