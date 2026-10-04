// train-result.js — result rendering / decomposition

const SPEAKER_PATH = 'M11 5 6 9H3v6h3l5 4zM15.5 8.5a5 5 0 0 1 0 7M18.5 5.5a9 9 0 0 1 0 13';
const STATUS_ICON_PATH = {
  correct: 'M5 12.5l4.5 4.5L19 7.5',
  wrong: 'M6 6l12 12M18 6 6 18',
  mixup: 'M4 7h14l-3-3M20 17H6l3 3',
  ambiguous: 'M12 8v5M12 16.5h.01',
  skipped: 'M5 12h10M11 6l6 6-6 6M19 6v12',
};
const STATUS_TITLE_KEY = {
  correct: 'result.correct',
  wrong: 'result.wrong',
  mixup: 'result.mixup',
  ambiguous: 'result.disambigAmbiguous',
  skipped: 'result.skipped',
};

function speakerButtonHTML(cls, small) {
  return `<button type="button" class="${cls} tr-speak${small ? ' tr-speak-sm' : ''}" title="${escHtml(t('card.readAloud'))}" aria-label="${escHtml(t('card.readAloud'))}"><svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="${SPEAKER_PATH}"/></svg></button>`;
}

// setResultHead renders the status circle and title of a result screen.
function setResultHead(kind) {
  const icon = $('result-icon');
  icon.textContent = t(STATUS_TITLE_KEY[kind]);
  icon.className = 'tr-result-title' + (kind === 'correct' ? '' : ` is-${kind}`);
  const circle = $('result-status-icon');
  circle.className = 'tr-status-icon' + (kind === 'correct' ? '' : ` is-${kind}`);
  circle.innerHTML = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="${STATUS_ICON_PATH[kind]}"/></svg>`;
}

// setCheckMark drives the round ✓/✗ marker next to a typing-gate input.
function setCheckMark(id, value, ok) {
  const el = $(id);
  if (!el) return;
  const typed = !!(value && value.trim());
  el.textContent = typed ? (ok ? '✓' : '✗') : '';
  el.className = 'tr-check' + (typed ? (ok ? ' is-ok' : ' is-bad') : '');
}

function renderFlagButton(btn, flagged) {
  const icon = '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 21V4h11l-2 4 2 4H5"/></svg>';
  btn.innerHTML = `${icon}<span>${escHtml(t(flagged ? 'result.flagged' : 'result.flagReview'))}</span>`;
  btn.classList.toggle('is-flagged', flagged);
}

// tierChipHTML renders the coloured tier chip (icon + label). The icon pops
// once when the tier just changed.
function tierChipHTML(tier, prevTier) {
  const entry = TIERS.find(e => e.label === tier);
  if (!entry) return '';
  const changed = !!prevTier && prevTier !== tier;
  return `<span class="tier-icon${changed ? ' tier-icon-changed' : ''}" title="${escHtml(tier)}">${entry.icon}</span>${escHtml(t(entry.i18nKey))}`;
}

function renderTierChip(el, tier, prevTier) {
  if (!el) return;
  const entry = TIERS.find(e => e.label === tier);
  if (!entry) { el.classList.add('hidden'); return; }
  el.innerHTML = tierChipHTML(tier, prevTier);
  el.className = `tier-chip tier-chip-${entry.label.toLowerCase()}` + (prevTier && prevTier !== tier ? ' is-changed' : '');
}

// moreInfoCountLabel summarises a folded "More info" box, e.g.
// "2 meanings · Measure word · 1 example".
function moreInfoCountLabel(info) {
  const parts = [];
  if (info.meanings.length) parts.push(info.meanings.length === 1 ? t('moreInfo.oneMeaning') : t('moreInfo.meanings', { n: info.meanings.length }));
  if (info.measureWords.length) parts.push(t('moreInfo.measureWord'));
  if (info.examples.length) parts.push(info.examples.length === 1 ? t('moreInfo.oneExample') : t('moreInfo.examples', { n: info.examples.length }));
  return parts.join(' · ');
}

function measureWordsHTML(list) {
  return `<div class="tr-mw-list">${list.map(m =>
    `<span><span class="tr-mw-zh font-hanzi">${escHtml(m.zh)}</span> ${escHtml(m.py)}</span>`).join('')}</div>`;
}

function moreInfoBlocksHTML(info) {
  let html = '';
  if (info.meanings.length) {
    html += `<div><div class="tr-eyebrow-sm">${escHtml(t('moreInfo.moreMeanings'))}</div><div class="tr-more-block-text">${info.meanings.map(escHtml).join(' · ')}</div></div>`;
  }
  if (info.measureWords.length) {
    html += `<div><div class="tr-eyebrow-sm">${escHtml(t('moreInfo.measureWord'))}</div>${measureWordsHTML(info.measureWords)}</div>`;
  }
  for (const ex of info.examples) {
    html += `<div><div class="tr-eyebrow-sm">${escHtml(t('moreInfo.example'))}</div><div class="tr-example-zh font-hanzi">${escHtml(ex.zh)}</div>${ex.tr ? `<div class="tr-example-tr">${escHtml(ex.tr)}</div>` : ''}</div>`;
  }
  return html;
}

// moreInfoBoxHTML renders the folded grey "More info" box for the collapsed
// translations of a word; `id` prefixes the toggle/body element IDs.
function moreInfoBoxHTML(collapsed, id) {
  const info = splitMoreInfo(collapsed);
  const count = moreInfoCountLabel(info);
  if (!count) return '';
  return `<div class="tr-more">
      <button type="button" id="${id}-toggle" class="tr-more-toggle" data-more-toggle="${id}-body" aria-expanded="false">
        <span>▸ ${escHtml(t('moreInfo.title'))}</span><span class="tr-more-count">${escHtml(count)}</span>
      </button>
      <div id="${id}-body" class="tr-more-body hidden">${moreInfoBlocksHTML(info)}</div>
    </div>`;
}

// wireDisclosures makes every [data-more-toggle] / [data-disclosure] button
// inside root fold and unfold its target, swapping ▸ / ▾.
function wireDisclosures(root) {
  root.querySelectorAll('[data-more-toggle], [data-disclosure]').forEach(btn => {
    btn.addEventListener('click', () => {
      const body = document.getElementById(btn.dataset.moreToggle || btn.dataset.disclosure);
      if (!body) return;
      const open = body.classList.contains('hidden');
      body.classList.toggle('hidden', !open);
      btn.setAttribute('aria-expanded', String(open));
      const label = btn.querySelector('span') || btn;
      label.textContent = label.textContent.replace(/^[▸▾]/, open ? '▾' : '▸');
    });
  });
}

// resultWordBlockHTML: big Hanzi, pinyin, meanings and a speaker button.
function resultWordBlockHTML(zh, pinyin, meanings) {
  return `<div class="tr-word-row result-word">
      <div class="tr-word-main">
        <div class="tr-word-head">
          <span class="tr-hanzi-lg font-hanzi">${escHtml(zh)}</span>
          ${pinyin ? `<span class="tr-pinyin-lg">${escHtml(pinyin)}</span>` : ''}
        </div>
        <div class="tr-meanings">${meanings.map(escHtml).join(' · ')}</div>
      </div>
      ${speakerButtonHTML('result-inline-play')}
    </div>`;
}

// nextDueText turns the answer's interval into the status-row text.
function nextDueText(result) {
  if (result.graduated) return t('result.graduated');
  if (result.learning_new_word) return t('result.dueToday');
  if (result.interval_days <= 0) return t('result.dueToday');
  if (result.interval_days === 1) return t('result.dueTomorrow');
  return t('result.nextReview', { n: result.interval_days });
}

// renderStatusRow fills the tier chip, learning-phase streak dots, the
// correct/attempts text and the next due date under every result.
function renderStatusRow(result) {
  // The tier chip always shows (it did before the redesign); only the
  // streak dots are a gamification element.
  if (result.tier) {
    renderTierChip($('bucket-info'), result.tier, result.prev_tier);
    show('bucket-info');
  } else {
    hide('bucket-info');
  }
  const dots = $('streak-dots');
  hide('streak-dots');
  if (result.learning_new_word && result.graduate_reps > 0) {
    const n = Math.max(0, Math.min(result.repetitions || 0, result.graduate_reps));
    if (_gamificationEnabled) {
      dots.innerHTML = Array.from({ length: result.graduate_reps }, (_, i) =>
        `<span class="tr-dot${i < n ? ' is-on' : ''}"></span>`).join('');
      show('streak-dots');
    }
    setText('attempt-stats', t('result.streakProgress', { n, total: result.graduate_reps }));
  } else if (result.graduated) {
    setText('attempt-stats', '');
  } else {
    const eff = result.total_correct + (result.streak_bonus || 0);
    setText('attempt-stats',
      t('result.correctStats', { eff, total: result.total_attempts }) +
      (result.streak_bonus > 0 ? ` (${t('result.streakBonus', { n: result.streak_bonus })})` : ''));
  }
  setText('next-due-info', nextDueText(result));
}

function renderWordAnswerResult(result, answer) {
  hide('card-area');
  show('result-area');
  hide('result-subtitle');
  hide('result-question');
  const breakdown = $('word-breakdown');
  const langs = orderLangsPrimaryFirst(selectedLangs, userPrimaryLang, userSecondaryLang);
  // Grouped by language (primary first, never interleaved); noise
  // annotations and anything beyond the max-translations-shown cap go into
  // the folded "More info" box (issue #431/#432/#433).
  const { shown: meanings, collapsed: moreTexts } =
    groupTranslationsByLang(result.translations, result.translations_extra, langs);
  const pinyin = result.pinyin || '';
  const playWord = () => playAudio(currentCard.word_id, result.zh_text);

  const renderCorrect = () => {
    setResultHead('correct');
    breakdown.innerHTML = resultWordBlockHTML(result.zh_text, pinyin, meanings) + moreInfoBoxHTML(moreTexts, 'result-more-info');
    wireDisclosures(breakdown);
    breakdown.querySelector('.result-inline-play')?.addEventListener('click', playWord);
    show('word-breakdown');
  };

  if (result.correct) {
    renderCorrect();
    hide('add-translation-row');
    hide('add-translation-lang-select');
    hide('accept-correct-btn');
    autoPlayResultAudio(currentCard, result);
    if (!result.learning_new_word && result.repetitions > 1) {
      $('streak-info').textContent = t('result.streak', { n: result.repetitions });
      show('streak-info');
    } else {
      hide('streak-info');
    }
  } else {
    const isEmpty = answer.trim() === '';
    const cw = result.confused_with;
    const yourAnswerPinyin = currentCard.mode === 'transl_to_zh' && result.user_answer_pinyin
      ? ` <span class="tr-pinyin-sm">${escHtml(result.user_answer_pinyin)}</span>`
      : '';
    const yourAnswerHtml = isEmpty ? '' : `
        <div class="tr-box tr-box-red tr-your-answer result-your-answer">
          <div class="tr-box-label">${escHtml(t('result.yourAnswer'))}</div>
          <div class="tr-struck">${escHtml(answer)}${yourAnswerPinyin}</div>
        </div>`;
    const { shown: cwMeanings, collapsed: cwMore } = cw
      ? groupTranslationsByLang(cw.confused_with_translations, cw.confused_with_translations_extra, langs)
      : { shown: [], collapsed: [] };
    const cwPinyin = cw && cw.confused_with_pinyin ? `<span class="tr-pinyin-sm">${escHtml(cw.confused_with_pinyin)}</span>` : '';
    const playConfused = () => playAudio(cw.confused_with_id, cw.confused_with_text);

    // Mix-up: "Asked" word, connector "you typed …, which is", then the
    // other word with the typed meaning highlighted.
    const mixupHtml = () => {
      // The asked word's folded extras (measure word first in the design,
      // but also capped-out meanings and examples — never dropped).
      const askedInfo = splitMoreInfo(moreTexts);
      const askedCount = moreInfoCountLabel(askedInfo);
      const askedMw = askedCount ? `
          <button type="button" class="tr-disclosure-link" data-disclosure="mixup-asked-more"><span>▸ ${escHtml(askedInfo.measureWords.length && !askedInfo.meanings.length && !askedInfo.examples.length ? t('moreInfo.measureWord') : t('moreInfo.title'))}</span></button>
          <div id="mixup-asked-more" class="hidden tr-more-body" style="padding:6px 0 0">${moreInfoBlocksHTML(askedInfo)}</div>` : '';
      const marked = currentCard.mode === 'transl_to_zh'
        ? cwMeanings.map(text => ({ text, typed: false }))
        : markTypedMeaning(cwMeanings, answer);
      const cwList = marked.map(m => m.typed
        ? `<span class="mixup-typed">${escHtml(m.text)}</span>`
        : `<span>${escHtml(m.text)}</span>`).join(' · ');
      const cwInfo = splitMoreInfo(cwMore);
      const cwCount = moreInfoCountLabel(cwInfo);
      const cwMoreHtml = cwCount ? `
          <button type="button" class="tr-disclosure-link" data-disclosure="mixup-other-more"><span>▸ ${escHtml(t('moreInfo.title'))}</span></button>
          <div id="mixup-other-more" class="hidden tr-more-body" style="padding:6px 0 0">${moreInfoBlocksHTML(cwInfo)}</div>` : '';
      return `
        <div id="result-mixup">
          <div class="tr-box tr-box-green mixup-asked">
            <div class="tr-box-label">${escHtml(t('result.asked'))}</div>
            <div class="tr-word-row">
              <div class="tr-word-main">
                <div class="tr-word-head"><span class="tr-hanzi-md font-hanzi">${escHtml(result.zh_text)}</span>${pinyin ? `<span class="tr-pinyin-sm">${escHtml(pinyin)}</span>` : ''}</div>
                <div class="tr-meanings">${meanings.map(escHtml).join(' · ')}</div>
              </div>
              ${speakerButtonHTML('result-inline-play', true)}
            </div>
            ${askedMw}
          </div>
          <div class="tr-mixup-connector"><span>${escHtml(t('result.youTyped'))} <b>“${escHtml(answer)}”</b>${escHtml(t('result.whichIs'))}</span></div>
          <div class="tr-box tr-box-amber mixup-other">
            <div class="tr-word-row">
              <div class="tr-word-main">
                <div class="tr-word-head"><span class="tr-hanzi-md font-hanzi">${escHtml(cw.confused_with_text)}</span>${cwPinyin}</div>
                <div class="tr-meanings">${cwList}</div>
              </div>
              ${speakerButtonHTML('btn-confused-play', true)}
            </div>
            ${cwMoreHtml}
          </div>
        </div>`;
    };

    // Renders the normal wrong-answer screen. Used directly for non-ambiguous
    // wrong answers, and as the fallback when the user continues past an
    // ambiguous result without resolving it (issue #194).
    const renderWrongResult = () => {
      hide('result-question');
      hide('result-subtitle');
      if (cw && !isEmpty) {
        setResultHead('mixup');
        breakdown.innerHTML = mixupHtml();
      } else {
        setResultHead('wrong');
        breakdown.innerHTML = resultWordBlockHTML(result.zh_text, pinyin, meanings) +
          moreInfoBoxHTML(moreTexts, 'result-more-info') + yourAnswerHtml;
      }
      wireDisclosures(breakdown);
      breakdown.querySelector('.btn-confused-play')?.addEventListener('click', playConfused);
      breakdown.querySelector('.result-inline-play')?.addEventListener('click', playWord);
      show('word-breakdown');

      if (!isEmpty && currentCard.mode !== 'transl_to_zh') {
        const addBtn = $('add-translation-btn');
        const langSelect = $('add-translation-lang-select');
        addBtn.textContent = t('result.addTranslation', { answer });
        addBtn.disabled = false;
        show('add-translation-row');

        const { langs: addLangs, defaultLang } = buildAddTranslationLangOptions(selectedLangs, userPrimaryLang);
        if (addLangs.length > 1) {
          langSelect.innerHTML = addLangs.map(l => `<option value="${l}">${l.toUpperCase()}</option>`).join('');
          langSelect.value = defaultLang;
          show('add-translation-lang-select');
        } else {
          hide('add-translation-lang-select');
        }

        addBtn.onclick = async () => {
          addBtn.disabled = true;
          try {
            const lang = addLangs.length > 1 ? langSelect.value : defaultLang;
            await apiFetch(`/api/words/${currentCard.word_id}/translations`, {
              method: 'POST',
              body: JSON.stringify({ text: answer, lang }),
            });
            await apiFetch('/api/quiz/accept-correct', {
              method: 'POST',
              body: JSON.stringify({
                word_id: currentCard.word_id,
                mode: currentCard.mode,
                langs: selectedLangs,
              }),
            });
            addBtn.textContent = t('result.added');
            loadNextCard(true);
          } catch (err) {
            addBtn.disabled = false;
            alert('Could not add translation: ' + err.message);
          }
        };
      } else {
        hide('add-translation-row');
        hide('add-translation-lang-select');
      }

      // Show "Accept as correct" button based on user's mode setting.
      // ponytail: transl_to_zh stores zh answers as en/de translations — wrong shape, hide both buttons.
      if (currentCard.mode !== 'transl_to_zh' && shouldShowAcceptTypo(answer, result, acceptCorrectMode, currentCard.mode)) {
        const acceptBtn = $('accept-correct-btn');
        acceptBtn.disabled = false;
        acceptBtn.textContent = t('result.acceptTypo');
        show('accept-correct-btn');
      } else {
        hide('accept-correct-btn');
      }

      loadDecomposition(result.zh_text, 'result-decompose', 'result-decompose-toggle');
      autoPlayResultAudio(currentCard, result);

      // Retype-on-wrong gate: block Next until the user retypes the correct
      // Chinese word and translation (same input-validation pattern as the
      // new-word introduction screen — see updateGotItState/isZhCorrect/isTransCorrect).
      // "Add as translation" (issue #372) and "Accept as correct (typo)"
      // (issue #389) both stay visible alongside the gate — clicking either
      // accepts the answer and advances immediately, intentionally bypassing
      // the retype requirement, since accepting the answer already confirms
      // it without needing a retype.
      if (wrongAnswerRetryMode !== 'off') {
        const { requireZh, requireTrans } = wrongRetypeFieldsForCard(wrongAnswerRetryMode, currentCard.mode);
        wrongRetypeTarget = { zhText: result.zh_text, translations: mergeTranslationMaps(result.translations, result.translations_extra), requireZh, requireTrans };
        $('wrong-retype-zh-input').value = '';
        $('wrong-retype-trans-input').value = '';
        setCheckMark('wrong-retype-zh-check', '', false);
        setCheckMark('wrong-retype-trans-check', '', false);
        requireZh ? show('wrong-retype-zh-group') : hide('wrong-retype-zh-group');
        requireTrans ? show('wrong-retype-trans-group') : hide('wrong-retype-trans-group');
        show('wrong-retype-area');
        $('next-btn').disabled = true;
        // Delayed so the just-unhidden input has completed layout before we
        // focus it. Guarded on an empty value so this can never steal focus
        // back from someone (or an E2E test) who already started typing in
        // the 50ms window — auto-focus is a convenience for the common case
        // of an untouched field, not a mandate to fight with faster input.
        setTimeout(() => {
          const el = $(requireZh ? 'wrong-retype-zh-input' : 'wrong-retype-trans-input');
          if (autofocusInput && !el.value) el.focus();
        }, 50);
      } else {
        wrongRetypeTarget = null;
        hide('wrong-retype-area');
        $('next-btn').disabled = false;
      }
    };

    if (result.ambiguous) {
      // Several words share the typed meaning — ask for the word being learned.
      setResultHead('ambiguous');
      const subtitle = $('result-subtitle');
      subtitle.textContent = t('result.ambiguousBody', { answer: currentCard.prompt });
      show('result-subtitle');

      $('result-question-label').textContent = getModeLabel(currentCard.mode);
      $('result-question-word').textContent = currentCard.prompt;
      if (currentCard.pinyin) {
        $('result-question-pinyin').textContent = currentCard.pinyin;
        show('result-question-pinyin');
      } else {
        hide('result-question-pinyin');
      }
      if (currentCard.mode === 'transl_to_zh') {
        // Show all translations across all languages except the one already shown as prompt.
        // Example-sentence noise is dropped, as on the question screen: the user
        // still has to type the zh answer here (issue #465).
        const { shown: others, collapsed: extraTexts } =
          groupTranslationsByLang(currentCard.translations, currentCard.translations_extra, langs, currentCard.prompt, true);
        const all = [...others, ...extraTexts];
        if (all.length > 0) {
          $('result-question-translations').textContent = t('card.also', { list: all.join(' · ') });
          show('result-question-translations');
        } else {
          hide('result-question-translations');
        }
      } else {
        hide('result-question-translations');
      }

      const typedBox = cw ? `
          <div class="tr-box tr-box-amber mixup-other">
            <div class="tr-word-head"><span class="tr-hanzi-md font-hanzi">${escHtml(cw.confused_with_text)}</span>${cwPinyin}<span class="tr-typed-tag">${escHtml(t('result.youTypedTag'))}</span>${speakerButtonHTML('btn-confused-play', true)}</div>
            <div class="tr-meanings">${cwMeanings.map(escHtml).join(' · ')}</div>
          </div>` : '';
      breakdown.innerHTML = `
        <div id="disambig-area">
          ${typedBox}
          <div class="tr-box tr-box-orange tr-disambig">
            <form id="disambig-form" class="tr-disambig-form">
              <input id="disambig-input" type="text" autocomplete="off" autocorrect="off" autocapitalize="off"
                class="tr-input font-hanzi" placeholder="${escHtml(t('card.placeholderZh'))}" />
              <button type="submit" class="ui-btn tr-btn-orange">${escHtml(t('card.submit'))}</button>
            </form>
            <div id="disambig-feedback" class="tr-disambig-feedback hidden"></div>
          </div>
          <div class="tr-center"><button id="disambig-give-up" type="button" class="tr-text-btn">${escHtml(t('result.showAnswer'))}</button></div>
        </div>`;
      const disambigArea = document.getElementById('disambig-area');
      // The question recap (prompt + masked pinyin + other meanings) sits
      // between the typed word and the answer form (issue #231).
      disambigArea.insertBefore($('result-question'), disambigArea.querySelector('.tr-disambig'));
      show('result-question');
      breakdown.querySelector('.btn-confused-play')?.addEventListener('click', playConfused);
      show('word-breakdown');
      hide('add-translation-row');
      hide('add-translation-lang-select');
      hide('accept-correct-btn');
      // Continuing without resolving falls back to the normal wrong-answer
      // screen instead of silently advancing (issue #194).
      const restoreResultQuestion = () => {
        const el = $('result-question');
        const anchor = $('result-subtitle');
        if (el.parentElement !== anchor.parentElement) anchor.after(el);
      };
      ambiguousUnresolved = () => { restoreResultQuestion(); renderWrongResult(); };
      $('disambig-give-up').addEventListener('click', () => {
        const fallback = ambiguousUnresolved;
        ambiguousUnresolved = null;
        if (fallback) fallback();
      });

      const disambigInput = document.getElementById('disambig-input');
      const disambigFeedback = document.getElementById('disambig-feedback');
      if (autofocusInput) disambigInput.focus();

      document.getElementById('disambig-form').addEventListener('submit', async (ev) => {
        ev.preventDefault();
        const typed = disambigInput.value.trim();
        if (!typed) return;
        if (typed.toLowerCase() === result.zh_text.toLowerCase()) {
          // Correct — upgrade to correct via AcceptCorrect
          try {
            await apiFetch('/api/quiz/accept-correct', {
              method: 'POST',
              body: JSON.stringify({ word_id: currentCard.word_id, mode: currentCard.mode, langs: selectedLangs }),
            });
            ambiguousUnresolved = null;
            restoreResultQuestion();
            hide('result-question');
            hide('result-subtitle');
            renderCorrect();
            loadDecomposition(result.zh_text, 'result-decompose', 'result-decompose-toggle');
            autoPlayResultAudio(currentCard, result);
          } catch (err) {
            disambigFeedback.textContent = 'Error: ' + err.message;
            disambigFeedback.classList.remove('hidden');
          }
        } else {
          // Wrong word typed — let the user retry; the disambiguation stays
          // unresolved until they either type the correct word or click Next,
          // at which point the normal wrong-answer screen is shown (issue #194).
          disambigInput.value = '';
          disambigFeedback.textContent = t('result.disambigNotQuite');
          disambigFeedback.classList.remove('hidden');
          if (autofocusInput) disambigInput.focus();
        }
      });
    } else {
      renderWrongResult();
    }
  }

  renderStatusRow(result);

  const reviewBtn = $('needs-review-btn');
  renderFlagButton(reviewBtn, false);
  reviewBtn.disabled = false;
  reviewBtn.onclick = async () => {
    reviewBtn.disabled = true;
    try {
      await apiFetch(`/api/words/${currentCard.word_id}/review`, { method: 'POST' });
      renderFlagButton(reviewBtn, true);
    } catch (err) {
      reviewBtn.disabled = false;
      alert('Could not flag word: ' + err.message);
    }
  };

  const editBtn = $('edit-card-btn');
  editBtn.onclick = () => window.open(`/vocab?edit=${currentCard.word_id}`, '_blank');
  show('review-edit-row');

  // Wrong-answer paths (including the ambiguous fallback) load the
  // character breakdown themselves once the result actually resolves —
  // it must stay hidden while an ambiguous result is unresolved so it
  // doesn't give away the answer (issue: "do not show character
  // breakdown on ambiguous screen").
  if (result.correct) {
    loadDecomposition(result.zh_text, 'result-decompose', 'result-decompose-toggle');
  }

  // HMM mnemonic scene: folded disclosure on correct; hidden on wrong
  renderResultMnemonic(result.correct && result.scene_text, !result.scene_text && !result.ambiguous,
    `/vocab?edit=${currentCard.word_id}`);

  if (!ambiguousUnresolved) $('next-btn').focus();
  loadStats();
}

// renderResultMnemonic shows the "▸ Show mnemonic" disclosure (sceneText) or
// the "+ Create mnemonic" link under a result.
function renderResultMnemonic(sceneText, offerCreate, createHref) {
  const hmmEl = $('result-hmm');
  if (sceneText) {
    hmmEl.innerHTML = `
      <button id="hmm-toggle-btn" type="button" class="tr-disclosure tr-disclosure-violet">▸ ${escHtml(t('hmm.showMnemonic'))}</button>
      <div id="hmm-toggle-content" class="hidden tr-mt-8"></div>
    `;
    show('result-hmm');
    $('hmm-toggle-btn').addEventListener('click', () => {
      const content = $('hmm-toggle-content');
      if (content.classList.contains('hidden')) {
        renderHMMSceneReadOnly('hmm-toggle-content', sceneText);
        content.classList.remove('hidden');
        $('hmm-toggle-btn').textContent = `▾ ${t('hmm.hideMnemonic')}`;
      } else {
        content.classList.add('hidden');
        $('hmm-toggle-btn').textContent = `▸ ${t('hmm.showMnemonic')}`;
      }
    });
  } else if (offerCreate) {
    hmmEl.innerHTML = `<a href="${escHtml(createHref)}" target="_blank" class="tr-disclosure tr-disclosure-violet">${escHtml(t('hmm.createMnemonic'))}</a>`;
    show('result-hmm');
  } else {
    hide('result-hmm');
  }
}

// Shows a full-screen interstitial celebrating a tier advance, gated by the
// celebrate_bucket_change user setting. Shared by all three card types
// (vocab word, HMM, component) — shown from maybeCelebrateThenShow right
// after an answer comes back, before the correct/wrong result screen.
function showCelebrationScreen({ prevTier, tier, zhText, pinyin, meanings }, onContinue) {
  hide('card-area');
  hide('result-area');
  show('celebration-screen');

  // Crossfade the old tier's icon into the new one in place: the old icon
  // fades, shrinks and turns away while the new one comes in from the
  // mirror of that; the tile's colours change over the same 1400 ms and a
  // ring pulses once at the end.
  const oldEntry = TIERS.find(e => e.label === prevTier);
  const newEntry = TIERS.find(e => e.label === tier);
  const oldEl = $('celebration-icon-old');
  const newEl = $('celebration-icon-new');
  const tile = $('celebration-tile');
  const ring = $('celebration-ring');
  oldEl.textContent = oldEntry ? oldEntry.icon : '';
  newEl.textContent = newEntry ? newEntry.icon : '';
  const tileStyle = e => e ? `background:${e.tile};border-color:${e.soft}` : '';
  tile.setAttribute('style', tileStyle(oldEntry));
  ring.classList.remove('is-on');
  ring.style.borderColor = newEntry ? newEntry.fg : 'transparent';
  oldEl.style.cssText = 'opacity:1;transform:scale(1)';
  newEl.style.cssText = 'opacity:0;transform:scale(.5) rotate(20deg)';
  // Apply the end state one frame later so the browser paints the initial
  // state first — otherwise the icons would swap instantly.
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      oldEl.style.cssText = 'opacity:0;transform:scale(.5) rotate(-20deg)';
      newEl.style.cssText = 'opacity:1;transform:scale(1)';
      tile.setAttribute('style', tileStyle(newEntry));
      void ring.offsetWidth;
      ring.classList.add('is-on');
    });
  });

  const wordEl = $('celebration-word');
  wordEl.innerHTML = zhText
    ? `<span class="tr-hanzi-md font-hanzi">${escHtml(zhText)}</span>${pinyin ? `<span class="tr-pinyin-sm">${escHtml(pinyin)}</span>` : ''}`
    : '';
  setText('celebration-meanings', (meanings || []).join(' · '));
  const chip = (e, cls) => e
    ? `<span class="tier-chip tier-chip-${e.label.toLowerCase()} ${cls}"><span class="tier-icon" aria-hidden="true">${e.icon}</span>${escHtml(t(e.i18nKey))}</span>`
    : '';
  $('celebration-transition').innerHTML =
    `${chip(oldEntry, '')}<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#9ca3af" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14M13 6l6 6-6 6"/></svg>${chip(newEntry, 'is-changed')}`;
  $('celebration-transition').setAttribute('aria-label', `${prevTier} → ${tier}`);
  $('celebration-continue-btn').onclick = () => {
    hide('celebration-screen');
    onContinue();
  };
  $('celebration-continue-btn').focus();
}

// showSkipResult shows the answer of a card skipped for today, when the
// skip_reveal_answer setting is on (issue #536). resp is the skip response.
function showSkipResult(resp) {
  hide('card-area');
  show('result-area');
  hide('result-subtitle');
  setResultHead('skipped');

  const breakdown = $('word-breakdown');
  if (currentCard.card_type === 'hmm') {
    breakdown.innerHTML = `
      <div class="result-word">
        <div class="tr-word-head"><span class="tr-hanzi-lg">${escHtml(currentCard.prompt)}</span></div>
        <div class="tr-meanings">${escHtml(resp.correct_answer || '')}</div>
      </div>`;
  } else if (currentCard.card_type === 'component') {
    const defsHtml = Object.entries(resp.correct_answers || {}).map(([lang, def]) =>
      `<div class="tr-def"><span class="tr-def-lang">${escHtml(lang)}</span><span class="tr-def-text">${escHtml(def)}</span></div>`
    ).join('');
    breakdown.innerHTML = `
      <div class="result-word">
        <div class="tr-word-row">
          <div class="tr-word-main">
            <div class="tr-word-head"><span class="tr-hanzi-lg font-hanzi">${escHtml(currentCard.prompt)}</span>${currentCard.pinyin ? `<span class="tr-pinyin-lg">${escHtml(currentCard.pinyin)}</span>` : ''}</div>
          </div>
          ${speakerButtonHTML('result-inline-play')}
        </div>
        <div class="tr-defs" style="margin:10px 0 0">${defsHtml}</div>
      </div>`;
    breakdown.querySelector('.result-inline-play')?.addEventListener('click', () => playComponentAudio(currentCard.prompt));
  } else {
    const langs = orderLangsPrimaryFirst(selectedLangs, userPrimaryLang, userSecondaryLang);
    const { shown, collapsed } = groupTranslationsByLang(resp.translations, resp.translations_extra, langs);
    breakdown.innerHTML = resultWordBlockHTML(resp.zh_text, resp.pinyin || '', shown) +
      moreInfoBoxHTML(collapsed, 'result-more-info');
    wireDisclosures(breakdown);
    const wordId = currentCard.word_id;
    breakdown.querySelector('.result-inline-play')?.addEventListener('click', () => playAudio(wordId, resp.zh_text));
  }
  show('word-breakdown');

  hide('add-translation-row');
  hide('add-translation-lang-select');
  hide('accept-correct-btn');
  hide('result-hmm');
  hide('result-decompose');
  hide('result-decompose-content');
  hide('review-edit-row');
  hide('bucket-info');
  hide('streak-dots');
  hide('streak-info');
  setText('attempt-stats', '');
  setText('next-due-info', t('result.dueTomorrow'));
  $('next-btn').focus();
  loadStats();
}

function showHMMResult(resp) {
  hide('card-area');
  show('result-area');
  hide('result-subtitle');
  setResultHead(resp.correct ? 'correct' : 'wrong');

  // Reuse word-breakdown for the answer display
  const badgeClass = HMM_TYPE_COLORS[currentCard.entity_type] || 'bg-gray-100 text-gray-700';
  const badgeHtml = `<span class="inline-block px-2 py-0.5 rounded-full text-xs font-bold uppercase tracking-wider ${escHtml(badgeClass)}">${escHtml(t('hmm.type.' + currentCard.entity_type))}</span>`;
  const yourAnswerHtml = (!resp.correct && resp.your_answer) ? `
    <div class="tr-box tr-box-red tr-your-answer result-your-answer">
      <div class="tr-box-label">${escHtml(t('result.yourAnswer'))}</div>
      <div class="tr-struck">${escHtml(resp.your_answer)}</div>
    </div>` : '';
  $('word-breakdown').innerHTML = `
    <div class="result-word">
      <div>${badgeHtml}</div>
      <div class="tr-word-head tr-mt-8"><span class="tr-hanzi-lg">${escHtml(currentCard.prompt)}</span></div>
      <div class="tr-meanings">${escHtml(resp.correct_answer)}</div>
    </div>
    ${yourAnswerHtml}`;
  show('word-breakdown');

  hide('add-translation-row');
  hide('add-translation-lang-select');
  hide('result-hmm');
  hide('result-decompose');
  hide('result-decompose-content');
  hide('review-edit-row');

  if (resp.learning) {
    setText('next-due-info', t('pinyin.learning', { n: 3 }));
  } else {
    setText('next-due-info', nextDueText({ interval_days: resp.interval_days }));
  }

  if (resp.tier) {
    renderTierChip($('bucket-info'), resp.tier, resp.prev_tier);
    show('bucket-info');
  } else {
    hide('bucket-info');
  }
  hide('streak-dots');

  const eff = resp.total_correct + (resp.streak_bonus || 0);
  setText('attempt-stats',
    t('result.correctStats', { eff, total: resp.total_attempts }) +
    (resp.streak_bonus > 0 ? ` (${t('result.streakBonus', { n: resp.streak_bonus })})` : ''));
  hide('streak-info');

  $('next-btn').focus();
  loadStats();
}

function showComponentResult(resp) {
  hide('card-area');
  show('result-area');
  hide('result-subtitle');
  const cw = resp.confused_with;
  setResultHead(resp.correct ? 'correct' : (cw ? 'mixup' : 'wrong'));

  const yourAnswerHtml = (!resp.correct && $('answer-input').value.trim()) ? `
    <div class="tr-box tr-box-red tr-your-answer result-your-answer">
      <div class="tr-box-label">${escHtml(t('result.yourAnswer'))}</div>
      <div class="tr-struck">${escHtml($('answer-input').value)}</div>
    </div>` : '';

  const answers = resp.correct_answers || {};
  const defsHtml = Object.entries(answers).map(([lang, def]) =>
    `<div class="tr-def"><span class="tr-def-lang">${escHtml(lang)}</span><span class="tr-def-text">${escHtml(def)}</span></div>`
  ).join('');

  // "Belongs to" mismatch box (issue #280) — mirrors renderWordAnswerResult's
  // confusedHtml, adapted for a component result where the confused-with
  // entity may itself be a word or another component.
  const answer = $('answer-input').value;
  const cwHtml = (!resp.correct && cw) ? `
      <div class="tr-mixup-connector"><span>${escHtml(t('result.youTyped'))} <b>“${escHtml(answer)}”</b>${escHtml(t('result.whichIs'))}</span></div>
      <div class="tr-box tr-box-amber mixup-other">
        <div class="tr-word-row">
          <div class="tr-word-main">
            <div class="tr-word-head"><span class="tr-hanzi-md font-hanzi">${escHtml(cw.confused_with_text)}</span>${cw.confused_with_pinyin ? `<span class="tr-pinyin-sm">${escHtml(cw.confused_with_pinyin)}</span>` : ''}</div>
            <div class="tr-meanings">${markTypedMeaning(Object.values(cw.confused_with_translations || {}).flat().filter(x => !isNoise(x)), answer)
              .map(m => m.typed ? `<span class="mixup-typed">${escHtml(m.text)}</span>` : `<span>${escHtml(m.text)}</span>`).join(' · ')}</div>
          </div>
          ${speakerButtonHTML('btn-confused-play', true)}
        </div>
      </div>` : '';
  const askedLabel = escHtml(currentCard.is_also_word ? t('component.modeLabelAlsoWord') : t('component.modeLabel'));
  const componentBlock = `
      <div class="${cwHtml ? 'tr-box tr-box-green mixup-asked' : 'result-word'}">
        <div class="${cwHtml ? 'tr-box-label' : 'tr-eyebrow-sm'}">${cwHtml ? escHtml(t('result.asked')) : askedLabel}</div>
        <div class="tr-word-row">
          <div class="tr-word-main">
            <div class="tr-word-head"><span class="${cwHtml ? 'tr-hanzi-md' : 'tr-hanzi-lg'} font-hanzi">${escHtml(currentCard.prompt)}</span>${currentCard.pinyin ? `<span class="tr-pinyin-lg">${escHtml(currentCard.pinyin)}</span>` : ''}</div>
          </div>
          ${speakerButtonHTML('component-inline-play', !!cwHtml)}
        </div>
        <div class="tr-defs" style="margin:10px 0 0">${defsHtml}</div>
      </div>`;

  $('word-breakdown').innerHTML = cwHtml
    ? `<div id="result-mixup">${componentBlock}${cwHtml}</div>`
    : `${componentBlock}${yourAnswerHtml}`;
  show('word-breakdown');
  const confusedPlayBtn = $('word-breakdown').querySelector('.btn-confused-play');
  if (confusedPlayBtn) {
    confusedPlayBtn.addEventListener('click', () => {
      if (cw.confused_with_kind === 'component') {
        playComponentAudio(cw.confused_with_component);
      } else {
        playAudio(cw.confused_with_id, cw.confused_with_text);
      }
    });
  }
  const inlinePlay = $('word-breakdown').querySelector('.component-inline-play');
  if (inlinePlay) inlinePlay.addEventListener('click', () => playComponentAudio(currentCard.prompt));
  autoPlayResultAudio(currentCard, resp);

  hide('add-translation-row');
  hide('add-translation-lang-select');
  loadDecomposition(currentCard.prompt, 'result-decompose', 'result-decompose-toggle');
  if (resp.tier) {
    renderTierChip($('bucket-info'), resp.tier, resp.prev_tier);
    show('bucket-info');
  } else {
    hide('bucket-info');
  }
  hide('streak-info');
  hide('streak-dots');

  if (!resp.correct) {
    const normCorrects = splitComponentDefs(resp.correct_answers);
    if (shouldShowAcceptBtn(answer, normCorrects, acceptCorrectMode)) {
      const acceptBtn = $('accept-correct-btn');
      acceptBtn.disabled = false;
      acceptBtn.textContent = t('result.acceptTypo');
      show('accept-correct-btn');
    } else {
      hide('accept-correct-btn');
    }
  } else {
    hide('accept-correct-btn');
  }

  renderResultMnemonic(resp.correct && resp.scene_text, !resp.scene_text,
    `/vocab?editComp=${encodeURIComponent(currentCard.prompt)}`);

  const reviewBtn = $('needs-review-btn');
  renderFlagButton(reviewBtn, false);
  reviewBtn.disabled = false;
  reviewBtn.onclick = async () => {
    reviewBtn.disabled = true;
    try {
      await apiFetch(`/api/components/${encodeURIComponent(currentCard.prompt)}/review`, { method: 'POST' });
      renderFlagButton(reviewBtn, true);
    } catch (err) {
      reviewBtn.disabled = false;
      alert('Could not flag component: ' + err.message);
    }
  };

  const editBtn = $('edit-card-btn');
  editBtn.onclick = () => window.open(`/vocab?editComp=${encodeURIComponent(currentCard.prompt)}`, '_blank');
  show('review-edit-row');

  setText('next-due-info', nextDueText({ interval_days: resp.interval_days }));
  const eff = resp.total_correct;
  setText('attempt-stats', t('result.correctStats', { eff, total: resp.total_attempts }));

  $('next-btn').focus();
  loadStats();
}

function renderCharDecomposition(charData) {
  let html = `<div class="p-3 bg-gray-50 border border-gray-200 rounded-xl mb-2">`;
  html += `<div class="flex items-baseline gap-2 mb-1">`;
  html += `<span class="text-2xl font-bold">${escHtml(charData.character)}</span>`;
  if (charData.radical) {
    html += `<span class="text-sm text-gray-400">${escHtml(t('decompose.radical', { r: charData.radical }))}</span>`;
  }
  if (charData.definition) {
    html += `<span class="text-sm text-gray-500">${escHtml(charData.definition)}</span>`;
  }
  html += `</div>`;

  if (charData.etymology && charData.etymology.hint) {
    html += `<div class="text-xs text-gray-400 italic mb-2">${escHtml(charData.etymology.hint)}</div>`;
  }

  if (charData.components && charData.components.length > 0) {
    html += `<div class="flex flex-wrap gap-2 mt-1">`;
    for (const comp of charData.components) {
      const isPhonetic = comp.is_semantic === false;
      const dimClass = isPhonetic ? ' opacity-40' : '';
      const title = isPhonetic ? ' title="Phonetic component (sound hint only)"' : '';
      html += `<div class="px-2 py-1 bg-white border border-gray-200 rounded-lg text-center min-w-[3rem]${dimClass}"${title}>`;
      html += `<div class="text-lg font-medium">${escHtml(comp.character)}</div>`;
      if (comp.pinyin && comp.pinyin.length > 0) {
        html += `<div class="text-xs text-gray-400">${escHtml(comp.pinyin.join(' / '))}</div>`;
      }
      if (comp.definition) {
        html += `<div class="text-xs text-gray-400 leading-tight">${escHtml(comp.definition)}</div>`;
      }
      html += `</div>`;
    }
    html += `</div>`;
  }

  html += `</div>`;
  return html;
}

async function loadDecomposition(zhText, containerId, toggleId) {
  try {
    const data = await apiFetch(`/api/hanzi/decompose?chars=${encodeURIComponent(zhText)}`);
    if (!data || data.length === 0) return;

    show(containerId);
    const toggle = $(toggleId);
    const content = $(containerId + '-content');

    content.innerHTML = data.map(renderCharDecomposition).join('');

    toggle.textContent = `▸ ${t('result.charBreakdown')}`;
    toggle.onclick = () => {
      const open = content.classList.contains('hidden');
      content.classList.toggle('hidden', !open);
      toggle.textContent = `${open ? '▾' : '▸'} ${t('result.charBreakdown')}`;
    };
  } catch (_) {}
}

async function loadNewWordBreakdown(zhText) {
  const container = $('new-word-breakdown');
  container.innerHTML = '';
  hide('new-word-breakdown');
  try {
    const langs = [userPrimaryLang, userSecondaryLang].filter(Boolean);
    const langsParam = langs.join(',');
    const data = await apiFetch(`/api/hanzi/decompose?chars=${encodeURIComponent(zhText)}&mark_new=true&langs=${encodeURIComponent(langsParam)}`);
    if (!data || data.length === 0) return;
    // Collect all semantic components that have a definition in at least one requested lang.
    const comps = [];
    for (const charData of data) {
      for (const comp of (charData.components || [])) {
        if (comp.is_semantic === false) continue;
        const defs = comp.definitions || {};
        const hasDef = langs.some(l => defs[l.toLowerCase()]) || comp.definition;
        if (!hasDef) continue;
        comps.push(comp);
      }
    }
    if (comps.length === 0) return;
    let html = `<div class="mt-5 text-left border-t border-gray-100 pt-4">`;
    html += `<div class="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-3">${escHtml(t('newWord.components'))}</div>`;
    html += `<div class="space-y-2">`;
    for (const comp of comps) {
      const isNew = comp.is_new_component === true;
      const defs = comp.definitions || {};
      const defParts = langs.map(l => defs[l.toLowerCase()]).filter(Boolean);
      const defText = defParts.length > 0 ? defParts.join(' · ') : (comp.definition || '');
      html += `<div class="flex items-center gap-3">`;
      html += `<span class="text-2xl font-bold text-gray-800 w-8 shrink-0">${escHtml(comp.character)}</span>`;
      html += `<span class="text-sm text-gray-600 flex-1">${escHtml(defText)}</span>`;
      if (isNew) {
        html += `<span class="text-xs font-semibold text-purple-600 bg-purple-50 border border-purple-200 px-2 py-0.5 rounded-full shrink-0">${escHtml(t('newWord.componentNew'))}</span>`;
      }
      html += `</div>`;
    }
    html += `</div></div>`;
    container.innerHTML = html;
    show('new-word-breakdown');
  } catch (_) {}
}
