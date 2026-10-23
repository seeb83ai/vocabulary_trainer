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

// isVoiceOnlyMode returns true for voice_to_transl and voice_to_zh when the
// user hasn't disabled voice, meaning the prompt text should be hidden.
function isVoiceOnlyMode(mode) {
  return (mode === 'voice_to_transl' || mode === 'voice_to_zh') && !voiceUnavailable;
}

// voiceFallbackMode returns the mode a voice card is asked in when voice is
// unavailable: Voice → Translation becomes Chinese → Translation, Voice →
// Chinese becomes Translation → Chinese (issue #571).
function voiceFallbackMode(mode, unavailable) {
  if (!unavailable) return mode;
  if (mode === 'voice_to_transl') return 'zh_to_transl';
  if (mode === 'voice_to_zh') return 'transl_to_zh';
  return mode;
}

// isZhPromptWithSound decides whether the Chinese prompt for this mode has
// audio available (play button + eligible for auto-play). zh_to_transl_no_sound
// is deliberately excluded — it's the whole point of that mode.
function isZhPromptWithSound(mode) {
  return mode === 'zh_to_transl' || mode === 'zh_pinyin_to_transl' || mode === 'voice_to_transl' || mode === 'voice_to_zh';
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
// on and the card is eligible (playAudio cuts off the previous clip).
function autoPlayCard(currentCard) {
  if (!autoPlayEnabled || !shouldAutoPlay(currentCard)) return;
  if (isAutoPlayBlockedByBlur(currentCard, noAutoVoiceOnBlur, blurPinyin)) return;
  questionAutoPlayed = true;
  if (currentCard.card_type === 'component') playComponentAudio(currentCard.prompt);
  else playAudio(currentCard.word_id, currentCard.zh_text || currentCard.prompt);
}

// shouldAutoPlayResult decides whether the result screen should read out the
// Chinese answer. Fires whenever auto-play is on and the answer wasn't
// already read out on the question screen (either because the mode never
// plays audio there, e.g. transl_to_zh, or because the question-screen play
// was skipped for some other reason, e.g. the blur guard in autoPlayCard) —
// except for card types/modes that must always stay silent (hmm cards have
// no audio; zh_to_transl_no_sound is deliberately silent) (issue #272).
// Sentence-blank cards ARE read out here: they stay silent on the question
// screen to avoid spoiling the blank, but once the answer is revealed on the
// result screen there's no reason to withhold audio (issue #446).
function shouldAutoPlayResult(currentCard, autoPlayEnabled, alreadyPlayed) {
  if (!autoPlayEnabled || !currentCard) return false;
  if (currentCard.card_type === 'hmm') return false;
  return !alreadyPlayed;
}

// resultAudioTarget picks what the result screen reads out: the full
// sentence for sentence-blank cards (issue #557), else the solved word.
function resultAudioTarget(currentCard, zhText) {
  if (currentCard.card_type === 'sentence' && currentCard.sentence_id) {
    return { id: currentCard.sentence_id, text: currentCard.sentence_text };
  }
  return { id: currentCard.word_id, text: zhText };
}

// autoPlayResultAudio plays the solved word's/component's Chinese audio (the
// full sentence for sentence-blank cards) on the result screen when eligible
// (see shouldAutoPlayResult); playAudio cuts off the previous clip.
function autoPlayResultAudio(currentCard, result) {
  if (!shouldAutoPlayResult(currentCard, autoPlayEnabled, questionAutoPlayed)) return;
  questionAutoPlayed = true; // avoid double-firing if called again for the same card
  const target = resultAudioTarget(currentCard, result.zh_text);
  if (currentCard.card_type === 'component') playComponentAudio(currentCard.prompt);
  else playAudio(target.id, target.text);
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
