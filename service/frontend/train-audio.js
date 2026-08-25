// train-audio.js — autoplay / audio helpers

// shouldAutoPlay decides whether a newly shown card should trigger auto-play
// audio. Never fires for transl_to_zh (would reveal the answer), hmm cards
// (no audio exists), or zh_to_transl_no_sound (deliberately excluded below).
function shouldAutoPlay(currentCard) {
  if (!currentCard) return false;
  if (currentCard.mode === 'new_word') return true;
  if (currentCard.card_type === 'component') return true;
  if (currentCard.card_type === 'hmm') return false;
  if (currentCard.card_type === 'sentence') return false;
  return isZhPromptWithSound(currentCard.mode);
}

// isVoiceOnlyMode returns true for voice_to_transl when the user hasn't
// disabled it, meaning the Chinese prompt text should be hidden.
function isVoiceOnlyMode(mode) {
  return mode === 'voice_to_transl' && !voiceUnavailable;
}

// isZhPromptWithSound decides whether the Chinese prompt for this mode has
// audio available (play button + eligible for auto-play). zh_to_transl_no_sound
// is deliberately excluded — it's the whole point of that mode.
function isZhPromptWithSound(mode) {
  return mode === 'zh_to_transl' || mode === 'zh_pinyin_to_transl' || mode === 'voice_to_transl';
}

// isAutoPlayBlockedByBlur decides whether the noAutoVoiceOnBlur guard should
// suppress auto-play for this card. Intro screens (new_word, new-component)
// render their pinyin via plain setText() calls that never route through
// applyPinyinBlur() — nothing is ever actually blurred there — so the guard,
// whose whole purpose is to avoid spoiling a blurred answer, doesn't apply.
function isAutoPlayBlockedByBlur(currentCard, noAutoVoiceOnBlur, blurPinyin) {
  const isIntroScreen = currentCard.mode === 'new_word' ||
    (currentCard.card_type === 'component' && currentCard.is_new);
  if (isIntroScreen || isVoiceOnlyMode(currentCard.mode)) return false;
  return noAutoVoiceOnBlur && (blurPinyin || !currentCard.pinyin);
}

// autoPlayCard plays audio for the current card when the auto-play toggle is
// on and the card is eligible, cutting off any still-playing previous clip.
function autoPlayCard(currentCard) {
  if (!autoPlayEnabled || !shouldAutoPlay(currentCard)) return;
  if (isAutoPlayBlockedByBlur(currentCard, noAutoVoiceOnBlur, blurPinyin)) return;
  if (currentAutoPlayAudio) {
    currentAutoPlayAudio.pause();
    currentAutoPlayAudio = null;
  }
  questionAutoPlayed = true;
  currentAutoPlayAudio = currentCard.card_type === 'component'
    ? playComponentAudio(currentCard.prompt)
    : playAudio(currentCard.word_id, currentCard.prompt);
}

// shouldAutoPlayResult decides whether the result screen should read out the
// Chinese answer. Fires whenever auto-play is on and the answer wasn't
// already read out on the question screen (either because the mode never
// plays audio there, e.g. transl_to_zh, or because the question-screen play
// was skipped for some other reason, e.g. the blur guard in autoPlayCard) —
// except for card types/modes that must always stay silent (hmm cards have
// no audio; zh_to_transl_no_sound is deliberately silent) (issue #272).
function shouldAutoPlayResult(currentCard, autoPlayEnabled, alreadyPlayed) {
  if (!autoPlayEnabled || !currentCard) return false;
  if (currentCard.card_type === 'hmm') return false;
  if (currentCard.card_type === 'sentence') return false;
  return !alreadyPlayed;
}

// autoPlayResultAudio plays the solved word's/component's Chinese audio on
// the result screen when eligible (see shouldAutoPlayResult), cutting off
// any still-playing previous clip.
function autoPlayResultAudio(currentCard, result) {
  if (!shouldAutoPlayResult(currentCard, autoPlayEnabled, questionAutoPlayed)) return;
  if (currentAutoPlayAudio) {
    currentAutoPlayAudio.pause();
    currentAutoPlayAudio = null;
  }
  questionAutoPlayed = true; // avoid double-firing if called again for the same card
  currentAutoPlayAudio = currentCard.card_type === 'component'
    ? playComponentAudio(currentCard.prompt)
    : playAudio(currentCard.word_id, result.zh_text);
}

// applyPinyinBlur blurs the pinyin hint (if the user enabled the setting)
// until the user taps/clicks it to reveal it, re-blurring on the next card.
function applyPinyinBlur() {
  const el = $('pinyin-hint');
  if (!el) return;
  if (blurPinyin) {
    el.classList.add('blur-sm', 'cursor-pointer');
    el.onclick = () => el.classList.remove('blur-sm');
  } else {
    el.classList.remove('blur-sm', 'cursor-pointer');
    el.onclick = null;
  }
}
