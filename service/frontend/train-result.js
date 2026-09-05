// train-result.js — result rendering / decomposition

function renderWordAnswerResult(result, answer) {
  hide('card-area');
  show('result-area');

  const icon = $('result-icon');
  // The question-recap box is temporarily relocated into the ambiguous
  // disambiguation panel (issue #231); restore it to its normal spot
  // before any breakdown/panel it might be sitting inside gets replaced
  // or removed, so the still-referenced #result-question node isn't lost.
  const restoreResultQuestion = () => {
    const el = $('result-question');
    if (el.parentElement !== icon.parentElement) {
      icon.parentElement.insertBefore(el, icon);
    }
  };
  if (result.correct) {
    icon.textContent = t('result.correct');
    icon.className = 'text-3xl font-bold text-green-600 mb-4';
  } else {
    icon.textContent = t('result.wrong');
    icon.className = 'text-3xl font-bold text-red-600 mb-4';
  }

  // Build breakdown for both correct and wrong answers
  const breakdown = $('word-breakdown');
  const pinyin = result.pinyin ? `<span class="text-gray-400 text-base ml-2">${escHtml(result.pinyin)}</span>` : '';
  const allTransTexts = selectedLangs.flatMap(lang => (result.translations || {})[lang] || []);
  const cleanTransTexts = allTransTexts.filter(x => !isNoise(x));
  const noiseTransTexts = allTransTexts.filter(isNoise);
  const noiseHtml = noiseTransTexts.length > 0
    ? `<details class="mt-1"><summary class="text-xs text-gray-400 cursor-pointer select-none">More info</summary><div class="text-gray-400 text-xs mt-0.5">${noiseTransTexts.map(escHtml).join(' · ')}</div></details>`
    : '';
  // For wrong answers use compact equal-sized display so zh and translations are easy to read side-by-side.
  // For correct answers keep the large Chinese character as a visual reward.
  const makeCorrectBox = (compact) => `
    <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
      <div class="text-xs text-green-500 uppercase tracking-wide mb-1">${escHtml(t('result.wordLabel'))}</div>
      <div class="flex items-center gap-2">
        <div class="${compact ? 'text-xl' : 'text-3xl'} font-bold text-gray-800 min-w-0">${escHtml(result.zh_text)}${pinyin}</div>
        <button type="button" class="result-inline-play text-2xl text-gray-400 hover:text-blue-500 transition leading-none shrink-0" title="Read aloud">🔊</button>
      </div>
      <div class="text-gray-600 text-sm mt-0.5">${cleanTransTexts.map(escHtml).join(' · ')}</div>
      ${noiseHtml}
    </div>`;
  const correctBox = makeCorrectBox(false);

  if (!result.correct) {
    const isEmpty = answer.trim() === '';
    const cw = result.confused_with;
    const yourAnswerPinyin = currentCard.mode === 'transl_to_zh' && result.user_answer_pinyin
      ? `<span class="text-gray-400 text-xs ml-1">${escHtml(result.user_answer_pinyin)}</span>`
      : '';

    if (result.ambiguous) {
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
        const allTexts = Object.values(currentCard.translations || {}).flat();
        const others = allTexts.filter(txt => txt !== currentCard.prompt && !isNoise(txt));
        if (others.length > 0) {
          $('result-question-translations').innerHTML = others.map(escHtml).join(' · ');
          show('result-question-translations');
        } else {
          hide('result-question-translations');
        }
      } else {
        hide('result-question-translations');
      }
      show('result-question');
    }

    const yourAnswerHtml = isEmpty ? '' : `
        <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
          <div class="text-xs text-red-400 uppercase tracking-wide mb-1">${escHtml(t('result.yourAnswer'))}</div>
          <div class="text-sm font-medium text-red-700">${escHtml(answer)}${yourAnswerPinyin}</div>
        </div>`;
    const confusedHtml = cw ? `
        <div class="p-3 bg-yellow-50 border border-yellow-200 rounded-xl">
          <div class="text-xs text-yellow-600 uppercase tracking-wide mb-1">${escHtml(t('result.belongsTo'))}</div>
          <div class="flex items-center gap-2">
            <div class="text-base font-semibold text-gray-800 min-w-0 overflow-hidden">${escHtml(cw.confused_with_text)}${cw.confused_with_pinyin ? `<span class="text-gray-400 text-sm ml-1">${escHtml(cw.confused_with_pinyin)}</span>` : ''}</div>
            <button class="btn-confused-play text-xl text-gray-400 hover:text-blue-500 transition leading-none shrink-0" title="Read aloud">🔊</button>
          </div>
          <div class="text-gray-500 text-sm mt-0.5">${Object.values(cw.confused_with_translations || {}).flat().map(escHtml).join(' · ')}</div>
        </div>` : '';
    // Renders the normal wrong-answer screen. Used directly for non-ambiguous
    // wrong answers, and as the fallback when the user continues past an
    // ambiguous result without resolving it (issue #194).
    const renderWrongResult = () => {
      restoreResultQuestion();
      hide('result-question');
      icon.textContent = t('result.wrong');
      icon.className = 'text-3xl font-bold text-red-600 mb-4';
      breakdown.innerHTML = `
        <div class="mt-4 space-y-2 text-left">
          ${yourAnswerHtml}
          ${confusedHtml}
          ${makeCorrectBox(true)}
        </div>`;
      const confusedPlayBtn = breakdown.querySelector('.btn-confused-play');
      if (confusedPlayBtn) {
        confusedPlayBtn.addEventListener('click', () => playAudio(cw.confused_with_id, cw.confused_with_text));
      }
      const inlinePlay = breakdown.querySelector('.result-inline-play');
      if (inlinePlay) inlinePlay.addEventListener('click', () => playAudio(currentCard.word_id, result.zh_text));
      show('word-breakdown');

      if (!isEmpty && currentCard.mode !== 'transl_to_zh') {
        const addBtn = $('add-translation-btn');
        const langSelect = $('add-translation-lang-select');
        addBtn.textContent = t('result.addTranslation', { answer });
        addBtn.disabled = false;
        addBtn.className = 'border border-gray-300 hover:border-blue-400 text-gray-600 hover:text-blue-700 text-sm font-medium py-2 rounded-xl transition';
        show('add-translation-row');

        const { langs, defaultLang } = buildAddTranslationLangOptions(selectedLangs, userPrimaryLang);
        if (langs.length > 1) {
          langSelect.innerHTML = langs.map(l => `<option value="${l}">${l.toUpperCase()}</option>`).join('');
          langSelect.value = defaultLang;
          show('add-translation-lang-select');
        } else {
          hide('add-translation-lang-select');
        }

        addBtn.onclick = async () => {
          addBtn.disabled = true;
          try {
            const lang = langs.length > 1 ? langSelect.value : defaultLang;
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
        acceptBtn.textContent = 'Accept as correct (typo)';
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
        wrongRetypeTarget = { zhText: result.zh_text, translations: result.translations, requireZh, requireTrans };
        $('wrong-retype-zh-input').value = '';
        $('wrong-retype-trans-input').value = '';
        $('wrong-retype-zh-check').textContent = '';
        $('wrong-retype-trans-check').textContent = '';
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
          if (!el.value) el.focus();
        }, 50);
      } else {
        wrongRetypeTarget = null;
        hide('wrong-retype-area');
        $('next-btn').disabled = false;
      }
    };

    if (result.ambiguous) {
      // Several words share a translation — ask the user to type another word with the same meaning.
      icon.textContent = t('result.disambigAmbiguous');
      icon.className = 'text-3xl font-bold text-orange-500 mb-4';
      const disambigHtml = `
        <div id="disambig-area" class="mt-4 space-y-2 text-left">
          ${confusedHtml}
          <div class="p-3 bg-orange-50 border border-orange-200 rounded-xl overflow-hidden">
            <div class="text-xs text-orange-600 uppercase tracking-wide mb-2 break-words">${escHtml(t('result.disambigPrompt'))}</div>
            <form id="disambig-form" class="flex flex-col sm:flex-row gap-2">
              <input id="disambig-input" type="text" autocomplete="off" autocorrect="off" autocapitalize="off"
                class="w-full sm:flex-1 min-w-0 border border-orange-300 rounded-lg px-3 py-1.5 text-base focus:outline-none focus:ring-2 focus:ring-orange-300"
                placeholder="Type Chinese word…" />
              <button type="submit" class="w-full sm:w-auto sm:shrink-0 px-4 py-1.5 bg-orange-500 hover:bg-orange-600 text-white text-sm font-medium rounded-lg transition">Check</button>
            </form>
            <div id="disambig-feedback" class="mt-1 text-sm hidden"></div>
          </div>
        </div>`;
      breakdown.innerHTML = disambigHtml;
      const confusedPlayBtn = breakdown.querySelector('.btn-confused-play');
      if (confusedPlayBtn) {
        confusedPlayBtn.addEventListener('click', () => playAudio(cw.confused_with_id, cw.confused_with_text));
      }
      // Move the question-recap box (shown/populated above) between the
      // "belongs to" box and the disambiguation input (issue #231).
      const disambigArea = document.getElementById('disambig-area');
      const orangeBox = disambigArea.querySelector('.bg-orange-50');
      disambigArea.insertBefore($('result-question'), orangeBox);
      show('word-breakdown');
      hide('add-translation-row');
      hide('add-translation-lang-select');
      hide('accept-correct-btn');
      // Continuing without resolving falls back to the normal wrong-answer
      // screen instead of silently advancing (issue #194).
      ambiguousUnresolved = renderWrongResult;

      const disambigInput = document.getElementById('disambig-input');
      const disambigFeedback = document.getElementById('disambig-feedback');
      disambigInput.focus();

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
            icon.textContent = t('result.correct');
            icon.className = 'text-3xl font-bold text-green-600 mb-4';
            const disambigArea = document.getElementById('disambig-area');
            if (disambigArea) disambigArea.remove();
            breakdown.innerHTML = `<div class="mt-4 space-y-2 text-left">${correctBox}</div>`;
            const disambigInlinePlay = breakdown.querySelector('.result-inline-play');
            if (disambigInlinePlay) disambigInlinePlay.addEventListener('click', () => playAudio(currentCard.word_id, result.zh_text));
            show('word-breakdown');
            loadDecomposition(result.zh_text, 'result-decompose', 'result-decompose-toggle');
            autoPlayResultAudio(currentCard, result);
          } catch (err) {
            disambigFeedback.textContent = 'Error: ' + err.message;
            disambigFeedback.className = 'mt-1 text-sm text-red-600';
            disambigFeedback.classList.remove('hidden');
          }
        } else {
          // Wrong word typed — let the user retry; the disambiguation stays
          // unresolved until they either type the correct word or click Next,
          // at which point the normal wrong-answer screen is shown (issue #194).
          disambigInput.value = '';
          disambigFeedback.textContent = t('result.disambigNotQuite');
          disambigFeedback.className = 'mt-1 text-sm text-red-500';
          disambigFeedback.classList.remove('hidden');
          disambigInput.focus();
        }
      });
    } else {
      renderWrongResult();
    }
  } else {
    breakdown.innerHTML = `<div class="mt-4 space-y-2 text-left">${correctBox}</div>`;
    const inlinePlay = breakdown.querySelector('.result-inline-play');
    if (inlinePlay) inlinePlay.addEventListener('click', () => playAudio(currentCard.word_id, result.zh_text));
    show('word-breakdown');
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
  }

  if (result.tier) {
    renderTierIcon($('bucket-info'), result.tier, result.prev_tier);
    show('bucket-info');
  } else {
    hide('bucket-info');
  }

  if (result.graduated) {
    setText('next-due-info', t('result.graduated'));
  } else if (result.learning_new_word) {
    setText('next-due-info', t('result.learning', { n: result.graduate_reps }));
  } else {
    setText('next-due-info', t('result.nextReview', { n: result.interval_days }));
  }
  if (result.graduated) {
    setText('attempt-stats', ``);
  } else if (result.learning_new_word) {
    setText('attempt-stats', t('result.streakProgress', { n: result.repetitions, total: result.graduate_reps }));
  } else {
    const eff = result.total_correct + (result.streak_bonus || 0);
    setText('attempt-stats',
      t('result.correctStats', { eff, total: result.total_attempts }) +
      (result.streak_bonus > 0 ? ` (${t('result.streakBonus', { n: result.streak_bonus })})` : ''));
  }

  const reviewBtn = $('needs-review-btn');
  reviewBtn.textContent = t('result.flagReview');
  reviewBtn.disabled = false;
  reviewBtn.className = 'w-1/2 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
  reviewBtn.onclick = async () => {
    reviewBtn.disabled = true;
    try {
      await apiFetch(`/api/words/${currentCard.word_id}/review`, { method: 'POST' });
      reviewBtn.textContent = t('result.flagged');
      reviewBtn.className = 'w-1/2 border border-orange-200 text-orange-400 font-medium py-2 rounded-xl text-sm';
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

  // HMM mnemonic scene: show collapsed toggle on correct; hide completely on wrong
  const hmmEl = $('result-hmm');
  if (result.scene_text && result.correct) {
    hmmEl.innerHTML = `
      <button id="hmm-toggle-btn" type="button" class="text-sm text-purple-400 hover:text-purple-600 transition">&#9654; ${t('hmm.showMnemonic')}</button>
      <div id="hmm-toggle-content" class="hidden mt-2"></div>
    `;
    show('result-hmm');
    $('hmm-toggle-btn').addEventListener('click', () => {
      const content = $('hmm-toggle-content');
      if (content.classList.contains('hidden')) {
        renderHMMSceneReadOnly('hmm-toggle-content', result.scene_text);
        content.classList.remove('hidden');
        $('hmm-toggle-btn').innerHTML = `&#9660; ${t('hmm.hideMnemonic')}`;
      } else {
        content.classList.add('hidden');
        $('hmm-toggle-btn').innerHTML = `&#9654; ${t('hmm.showMnemonic')}`;
      }
    });
  } else if (!result.scene_text && !result.ambiguous) {
    hmmEl.innerHTML = `<a href="/vocab?edit=${currentCard.word_id}" target="_blank" class="text-sm text-purple-400 hover:text-purple-600 transition">+ ${t('hmm.createMnemonic')}</a>`;
    show('result-hmm');
  } else {
    hide('result-hmm');
  }

  if (!ambiguousUnresolved) $('next-btn').focus();
  loadStats();
}

// Shows a full-screen interstitial celebrating a tier advance, gated by the
// celebrate_bucket_change user setting. Shared by all three card types
// (vocab word, HMM, component) — shown from maybeCelebrateThenShow right
// after an answer comes back, before the correct/wrong result screen.
function showCelebrationScreen({ prevTier, tier }, onContinue) {
  hide('card-area');
  hide('result-area');
  show('celebration-screen');

  // Crossfade the old tier's icon into the new one in place, rather than
  // statically showing both — old icon fades/shrinks out while the new one
  // fades/grows in.
  const oldEntry = TIERS.find(e => e.label === prevTier);
  const newEntry = TIERS.find(e => e.label === tier);
  const oldEl = $('celebration-icon-old');
  const newEl = $('celebration-icon-new');
  oldEl.textContent = oldEntry ? oldEntry.icon : '';
  newEl.textContent = newEntry ? newEntry.icon : '';
  const base = 'absolute inset-0 flex items-center justify-center transition-all duration-[1400ms] ease-in-out';
  oldEl.className = `${base} opacity-100 scale-100`;
  newEl.className = `${base} opacity-0 scale-50`;
  // Apply the transitioned end-state one frame later so the browser paints
  // the initial state first — otherwise the change can collapse into the
  // same frame and the icon would just swap instantly instead of animating.
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      oldEl.className = `${base} opacity-0 scale-50`;
      newEl.className = `${base} opacity-100 scale-100`;
    });
  });

  setText('celebration-transition', `${prevTier} → ${tier}`);
  $('celebration-continue-btn').onclick = () => {
    hide('celebration-screen');
    onContinue();
  };
  $('celebration-continue-btn').focus();
}

function showHMMResult(resp) {
  hide('card-area');
  show('result-area');

  const icon = $('result-icon');
  if (resp.correct) {
    icon.textContent = t('result.correct');
    icon.className = 'text-3xl font-bold text-green-600 mb-4';
  } else {
    icon.textContent = t('result.wrong');
    icon.className = 'text-3xl font-bold text-red-600 mb-4';
  }

  // Reuse word-breakdown for the answer display
  const badgeClass = HMM_TYPE_COLORS[currentCard.entity_type] || 'bg-gray-100 text-gray-700';
  const badgeHtml = `<span class="inline-block px-2 py-0.5 rounded-full text-xs font-bold uppercase tracking-wider ${escHtml(badgeClass)}">${escHtml(t('hmm.type.' + currentCard.entity_type))}</span>`;
  const yourAnswerHtml = (!resp.correct && resp.your_answer) ? `
    <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
      <div class="text-xs text-red-400 uppercase tracking-wide mb-1">${escHtml(t('result.yourAnswer'))}</div>
      <div class="text-lg font-medium text-red-700">${escHtml(resp.your_answer)}</div>
    </div>` : '';
  $('word-breakdown').innerHTML = `
    <div class="mt-4 space-y-2 text-left">
      ${yourAnswerHtml}
      <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
        <div class="text-xs text-green-500 uppercase tracking-wide mb-1">${badgeHtml}</div>
        <div class="text-3xl font-bold text-gray-800 mb-1">${escHtml(currentCard.prompt)}</div>
        <div class="text-xl font-bold text-gray-800">${escHtml(resp.correct_answer)}</div>
      </div>
    </div>`;
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
    setText('next-due-info', t('result.nextReview', { n: resp.interval_days }));
  }

  if (resp.tier) {
    renderTierIcon($('bucket-info'), resp.tier, resp.prev_tier);
    show('bucket-info');
  } else {
    hide('bucket-info');
  }

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

  const icon = $('result-icon');
  if (resp.correct) {
    icon.textContent = t('result.correct');
    icon.className = 'text-3xl font-bold text-green-600 mb-4';
  } else {
    icon.textContent = t('result.wrong');
    icon.className = 'text-3xl font-bold text-red-600 mb-4';
  }

  const yourAnswerHtml = (!resp.correct && $('answer-input').value.trim()) ? `
    <div class="p-3 bg-red-50 border border-red-200 rounded-xl">
      <div class="text-xs text-red-400 uppercase tracking-wide mb-1">${escHtml(t('result.yourAnswer'))}</div>
      <div class="text-lg font-medium text-red-700">${escHtml($('answer-input').value)}</div>
    </div>` : '';

  const answers = resp.correct_answers || {};
  const defsHtml = Object.entries(answers).map(([lang, def]) =>
    `<div class="flex items-baseline gap-2">
       <span class="text-xs font-semibold text-green-600 uppercase w-6 shrink-0">${escHtml(lang)}</span>
       <span class="text-xl font-bold text-gray-800">${escHtml(def)}</span>
     </div>`
  ).join('');
  const compResultPinyin = currentCard.pinyin
    ? `<span class="text-gray-400 text-base ml-2">${escHtml(currentCard.pinyin)}</span>`
    : '';

  // "Belongs to" mismatch box (issue #280) — mirrors renderWordAnswerResult's
  // confusedHtml, adapted for a component result where the confused-with
  // entity may itself be a word or another component.
  const cw = resp.confused_with;
  const confusedHtml = (!resp.correct && cw) ? `
      <div class="p-3 bg-yellow-50 border border-yellow-200 rounded-xl">
        <div class="text-xs text-yellow-600 uppercase tracking-wide mb-1">${escHtml(t('result.belongsTo'))}</div>
        <div class="flex items-center gap-2">
          <div class="text-base font-semibold text-gray-800 min-w-0 overflow-hidden">${escHtml(cw.confused_with_text)}${cw.confused_with_pinyin ? `<span class="text-gray-400 text-sm ml-1">${escHtml(cw.confused_with_pinyin)}</span>` : ''}</div>
          <button class="btn-confused-play text-xl text-gray-400 hover:text-blue-500 transition leading-none shrink-0" title="Read aloud">🔊</button>
        </div>
        <div class="text-gray-500 text-sm mt-0.5">${Object.values(cw.confused_with_translations || {}).flat().map(escHtml).join(' · ')}</div>
      </div>` : '';

  $('word-breakdown').innerHTML = `
    <div class="mt-4 space-y-2 text-left">
      ${yourAnswerHtml}
      ${confusedHtml}
      <div class="p-3 bg-green-50 border border-green-200 rounded-xl">
        <div class="text-xs text-green-500 uppercase tracking-wide mb-1">${escHtml(currentCard.is_also_word ? t('component.modeLabelAlsoWord') : t('component.modeLabel'))}</div>
        <div class="flex items-center gap-2 mb-1">
          <div class="text-3xl font-bold text-gray-800">${escHtml(currentCard.prompt)}${compResultPinyin}</div>
          <button type="button" class="component-inline-play text-2xl text-gray-400 hover:text-blue-500 transition leading-none shrink-0" title="Read aloud">🔊</button>
        </div>
        ${defsHtml}
      </div>
    </div>`;
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
    renderTierIcon($('bucket-info'), resp.tier, resp.prev_tier);
    show('bucket-info');
  } else {
    hide('bucket-info');
  }
  hide('streak-info');

  if (!resp.correct) {
    const answer = $('answer-input').value;
    const normCorrects = splitComponentDefs(resp.correct_answers);
    if (shouldShowAcceptBtn(answer, normCorrects, acceptCorrectMode)) {
      const acceptBtn = $('accept-correct-btn');
      acceptBtn.disabled = false;
      acceptBtn.textContent = 'Accept as correct (typo)';
      show('accept-correct-btn');
    } else {
      hide('accept-correct-btn');
    }
  } else {
    hide('accept-correct-btn');
  }

  const hmmEl = $('result-hmm');
  if (resp.scene_text && resp.correct) {
    hmmEl.innerHTML = `
      <button id="hmm-toggle-btn" type="button" class="text-sm text-purple-400 hover:text-purple-600 transition">&#9654; ${t('hmm.showMnemonic')}</button>
      <div id="hmm-toggle-content" class="hidden mt-2"></div>
    `;
    show('result-hmm');
    $('hmm-toggle-btn').addEventListener('click', () => {
      const content = $('hmm-toggle-content');
      if (content.classList.contains('hidden')) {
        renderHMMSceneReadOnly('hmm-toggle-content', resp.scene_text);
        content.classList.remove('hidden');
        $('hmm-toggle-btn').innerHTML = `&#9660; ${t('hmm.hideMnemonic')}`;
      } else {
        content.classList.add('hidden');
        $('hmm-toggle-btn').innerHTML = `&#9654; ${t('hmm.showMnemonic')}`;
      }
    });
  } else if (!resp.scene_text) {
    hmmEl.innerHTML = `<a href="/vocab?editComp=${encodeURIComponent(currentCard.prompt)}" target="_blank" class="text-sm text-purple-400 hover:text-purple-600 transition">+ ${t('hmm.createMnemonic')}</a>`;
    show('result-hmm');
  } else {
    hide('result-hmm');
  }

  const reviewBtn = $('needs-review-btn');
  reviewBtn.textContent = t('result.flagReview');
  reviewBtn.disabled = false;
  reviewBtn.className = 'w-1/2 border border-orange-300 hover:border-orange-400 text-orange-600 hover:text-orange-700 font-medium py-2 rounded-xl text-sm transition';
  reviewBtn.onclick = async () => {
    reviewBtn.disabled = true;
    try {
      await apiFetch(`/api/components/${encodeURIComponent(currentCard.prompt)}/review`, { method: 'POST' });
      reviewBtn.textContent = t('result.flagged');
      reviewBtn.className = 'w-1/2 border border-orange-200 text-orange-400 font-medium py-2 rounded-xl text-sm';
    } catch (err) {
      reviewBtn.disabled = false;
      alert('Could not flag component: ' + err.message);
    }
  };

  const editBtn = $('edit-card-btn');
  editBtn.onclick = () => window.open(`/vocab?editComp=${encodeURIComponent(currentCard.prompt)}`, '_blank');
  show('review-edit-row');

  setText('next-due-info', t('result.nextReview', { n: resp.interval_days }));
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

    toggle.innerHTML = `&#9654; ${escHtml(t('result.charBreakdown'))}`;
    toggle.onclick = () => {
      if (content.classList.contains('hidden')) {
        content.classList.remove('hidden');
        toggle.innerHTML = `&#9660; ${escHtml(t('result.charBreakdown'))}`;
      } else {
        content.classList.add('hidden');
        toggle.innerHTML = `&#9654; ${escHtml(t('result.charBreakdown'))}`;
      }
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
